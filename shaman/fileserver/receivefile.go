package fileserver

/* ***** BEGIN GPL LICENSE BLOCK *****
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License
 * as published by the Free Software Foundation; either version 2
 * of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software Foundation,
 * Inc., 59 Temple Place - Suite 330, Boston, MA  02111-1307, USA.
 *
 * ***** END GPL LICENCE BLOCK *****
 *
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
 */

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/sirupsen/logrus"
	"github.com/armadillica/flamenco-manager/shaman/auth"
	"github.com/armadillica/flamenco-manager/shaman/filestore"
	"github.com/armadillica/flamenco-manager/shaman/hasher"
	"github.com/armadillica/flamenco-manager/shaman/httpserver"
)

// receiveFile streams a file from a HTTP request to disk.
func (fs *FileServer) receiveFile(ctx context.Context, w http.ResponseWriter, r *http.Request, checksum string, filesize int64) {
	logger := packageLogger.WithFields(auth.RequestLogFields(r))

	bodyReader, err := httpserver.DecompressedReader(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer bodyReader.Close()

	originalFilename := r.Header.Get("X-Shaman-Original-Filename")
	if originalFilename == "" {
		originalFilename = "-not specified-"
	}
	logger = logger.WithField("originalFilename", originalFilename)

	localPath, status := fs.fileStore.ResolveFile(checksum, filesize, filestore.ResolveEverything)
	logger = logger.WithField("path", localPath)
	if status == filestore.StatusStored {
		logger.Info("uploaded file already exists")
		w.Header().Set("Location", r.RequestURI)
		http.Error(w, "File already stored", http.StatusAlreadyReported)
		return
	}

	if status == filestore.StatusUploading && r.Header.Get("X-Shaman-Can-Defer-Upload") == "true" {
		logger.Info("someone is uploading this file and client can defer")
		http.Error(w, "File being uploaded, please defer", http.StatusAlreadyReported)
		return
	}
	logger.Info("receiving file")

	streamTo, err := fs.fileStore.OpenForUpload(checksum, filesize)
	if err != nil {
		logger.WithError(err).Error("unable to open file for writing uploaded data")
		http.Error(w, "Unable to open file", http.StatusInternalServerError)
		return
	}

	// clean up temporary file if it still exists at function exit.
	defer func() {
		streamTo.Close()
		fs.fileStore.RemoveUploadedFile(streamTo.Name())
	}()

	// Abort this uploadwhen the file has been finished by someone else.
	uploadDone := make(chan struct{})
	uploadAlreadyCompleted := false
	defer close(uploadDone)
	receiverChannel := fs.receiveListenerFor(checksum, filesize)
	go func() {
		select {
		case <-receiverChannel:
		case <-uploadDone:
			close(receiverChannel)
			return
		}

		logger := logger.WithField("path", localPath)
		logger.Info("file was completed during someone else's upload")

		uploadAlreadyCompleted = true
		err := r.Body.Close()
		if err != nil {
			logger.WithError(err).Warning("error closing connection")
		}
	}()

	written, actualChecksum, err := hasher.Copy(streamTo, bodyReader)
	if err != nil {
		if closeErr := streamTo.Close(); closeErr != nil {
			logger.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"closeError":    closeErr,
			}).Error("error closing local file after other I/O error occured")
		}

		logger = logger.WithError(err)
		if uploadAlreadyCompleted {
			logger.Debug("aborted upload")
			w.Header().Set("Location", r.RequestURI)
			http.Error(w, "File already stored", http.StatusAlreadyReported)
		} else if err == io.ErrUnexpectedEOF {
			logger.Info("unexpected EOF, client probably just disconnected")
		} else {
			logger.Warning("unable to copy request body to file")
			http.Error(w, "I/O error", http.StatusInternalServerError)
		}
		return
	}

	if err := streamTo.Close(); err != nil {
		logger.WithError(err).Warning("error closing local file")
		http.Error(w, "I/O error", http.StatusInternalServerError)
		return
	}

	if written != filesize {
		logger.WithFields(logrus.Fields{
			"declaredSize": filesize,
			"actualSize":   written,
		}).Warning("mismatch between expected and actual size")
		http.Error(w,
			fmt.Sprintf("Received %d bytes but you promised %d", written, filesize),
			http.StatusExpectationFailed)
		return
	}

	if actualChecksum != checksum {
		logger.WithFields(logrus.Fields{
			"declaredChecksum": checksum,
			"actualChecksum":   actualChecksum,
		}).Warning("mismatch between expected and actual checksum")
		http.Error(w,
			"Declared and actual checksums differ",
			http.StatusExpectationFailed)
		return
	}

	logger.WithFields(logrus.Fields{
		"receivedBytes": written,
		"checksum":      actualChecksum,
		"tempFile":      streamTo.Name(),
	}).Debug("File received")

	if err := fs.fileStore.MoveToStored(checksum, filesize, streamTo.Name()); err != nil {
		logger.WithFields(logrus.Fields{
			"tempFile":      streamTo.Name(),
			logrus.ErrorKey: err,
		}).Error("unable to move file from 'upload' to 'stored' storage")
		http.Error(w,
			"unable to move file from 'upload' to 'stored' storage",
			http.StatusInternalServerError)
		return
	}

	http.Error(w, "", http.StatusNoContent)
}
