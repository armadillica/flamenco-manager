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
	"os"
	"strconv"

	"github.com/sirupsen/logrus"

	"github.com/armadillica/flamenco-manager/shaman/filestore"
)

// serveFile only serves stored files (not 'uploading' or 'checking')
func (fs *FileServer) serveFile(ctx context.Context, w http.ResponseWriter, checksum string, filesize int64) {
	path, status := fs.fileStore.ResolveFile(checksum, filesize, filestore.ResolveStoredOnly)
	if status != filestore.StatusStored {
		http.Error(w, "File Not Found", http.StatusNotFound)
		return
	}

	logger := packageLogger.WithField("path", path)

	stat, err := os.Stat(path)
	if err != nil {
		logger.WithError(err).Error("unable to stat file")
		http.Error(w, "File Not Found", http.StatusNotFound)
		return
	}
	if stat.Size() != filesize {
		logger.WithFields(logrus.Fields{
			"realSize":     stat.Size(),
			"expectedSize": filesize,
		}).Error("file size in storage is corrupt")
		http.Error(w, "File Size Incorrect", http.StatusInternalServerError)
		return
	}

	infile, err := os.Open(path)
	if err != nil {
		logger.WithError(err).Error("unable to read file")
		http.Error(w, "File Not Found", http.StatusNotFound)
		return
	}

	filesizeStr := strconv.FormatInt(filesize, 10)
	w.Header().Set("Content-Type", "application/binary")
	w.Header().Set("Content-Length", filesizeStr)
	w.Header().Set("ETag", fmt.Sprintf("'%s-%s'", checksum, filesizeStr))
	w.Header().Set("X-Shaman-Checksum", checksum)

	written, err := io.Copy(w, infile)
	if err != nil {
		logger.WithError(err).Error("unable to copy file to writer")
		// Anything could have been sent by now, so just close the connection.
		return
	}
	logger.WithField("written", written).Debug("file send to writer")
}
