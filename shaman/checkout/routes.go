package checkout

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
	"fmt"
	"net/http"
	"strings"

	"github.com/armadillica/flamenco-manager/shaman/filestore"
	"github.com/armadillica/flamenco-manager/shaman/httpserver"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/armadillica/flamenco-manager/shaman/auth"
)

// Responses for each line of a checkout definition file.
const (
	responseFileUnkown       = "file-unknown"
	responseAlreadyUploading = "already-uploading"
	responseError            = "ERROR"
)

// AddRoutes adds HTTP routes to the muxer.
func (m *Manager) AddRoutes(router *mux.Router, auther auth.Authenticator) {
	router.Handle("/checkout/requirements", auther.WrapFunc(m.reportRequirements)).Methods("POST")
	router.Handle("/checkout/create/{checkoutID}", auther.WrapFunc(m.createCheckout)).Methods("POST")
}

func (m *Manager) reportRequirements(w http.ResponseWriter, r *http.Request) {
	logger := packageLogger.WithFields(auth.RequestLogFields(r))
	logger.Debug("user requested checkout requirements")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Header.Get("Content-Type") != "text/plain" {
		http.Error(w, "Expecting text/plain content type", http.StatusBadRequest)
		return
	}

	bodyReader, err := httpserver.DecompressedReader(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer bodyReader.Close()

	// Unfortunately, Golang doesn't allow us (for good reason) to send a reply while
	// still reading the response. See https://github.com/golang/go/issues/4637
	responseLines := []string{}
	alreadyRequested := map[string]bool{}
	reader := NewDefinitionReader(r.Context(), bodyReader)
	for line := range reader.Read() {
		fileKey := fmt.Sprintf("%s/%d", line.Checksum, line.FileSize)
		if alreadyRequested[fileKey] {
			// User asked for this (checksum, filesize) tuple already.
			continue
		}

		path, status := m.fileStore.ResolveFile(line.Checksum, line.FileSize, filestore.ResolveEverything)

		response := ""
		switch status {
		case filestore.StatusDoesNotExist:
			// Caller can upload this file immediately.
			response = responseFileUnkown
		case filestore.StatusUploading:
			// Caller should postpone uploading this file until all 'does-not-exist' files have been uploaded.
			response = responseAlreadyUploading
		case filestore.StatusStored:
			// We expect this file to be sent soon, though, so we need to
			// 'touch' it to make sure it won't be GC'd in the mean time.
			go touch(path)

			// Only send a response when the caller needs to do something.
			continue
		default:
			logger.WithFields(logrus.Fields{
				"path":     path,
				"status":   status,
				"checksum": line.Checksum,
				"filesize": line.FileSize,
			}).Error("invalid status returned by ResolveFile")
			continue
		}

		alreadyRequested[fileKey] = true
		responseLines = append(responseLines, fmt.Sprintf("%s %s\n", response, line.FilePath))
	}
	if reader.Err != nil {
		logger.WithError(reader.Err).Warning("error reading checkout definition")
		http.Error(w, fmt.Sprintf("%s %v\n", responseError, reader.Err), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(strings.Join(responseLines, "")))
}

func (m *Manager) createCheckout(w http.ResponseWriter, r *http.Request) {
	checkoutID := mux.Vars(r)["checkoutID"]

	logger := packageLogger.WithFields(auth.RequestLogFields(r)).WithField("checkoutID", checkoutID)
	logger.Debug("user requested checkout creation")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Header.Get("Content-Type") != "text/plain" {
		http.Error(w, "Expecting text/plain content type", http.StatusBadRequest)
		return
	}
	bodyReader, err := httpserver.DecompressedReader(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer bodyReader.Close()

	// Actually create the checkout.
	resolvedCheckoutInfo, err := m.PrepareCheckout(checkoutID)
	if err != nil {
		switch err {
		case ErrInvalidCheckoutID:
			http.Error(w, fmt.Sprintf("invalid checkout ID '%s'", checkoutID), http.StatusBadRequest)
		case ErrCheckoutAlreadyExists:
			http.Error(w, fmt.Sprintf("checkout '%s' already exists", checkoutID), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// The checkout directory was created, so if anything fails now, it should be erased.
	var checkoutOK bool
	defer func() {
		if !checkoutOK {
			m.EraseCheckout(checkoutID)
		}
	}()

	responseLines := []string{}
	reader := NewDefinitionReader(r.Context(), bodyReader)
	for line := range reader.Read() {
		blobPath, status := m.fileStore.ResolveFile(line.Checksum, line.FileSize, filestore.ResolveStoredOnly)
		if status != filestore.StatusStored {
			// Caller should upload this file before we can create the checkout.
			responseLines = append(responseLines, fmt.Sprintf("%s %s\n", responseFileUnkown, line.FilePath))
			continue
		}

		if err := m.SymlinkToCheckout(blobPath, resolvedCheckoutInfo.absolutePath, line.FilePath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if reader.Err != nil {
		http.Error(w, fmt.Sprintf("ERROR %v\n", reader.Err), http.StatusBadRequest)
		return
	}

	// If there was any file missing, we should just stop now.
	if len(responseLines) > 0 {
		http.Error(w, strings.Join(responseLines, ""), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(resolvedCheckoutInfo.RelativePath))

	checkoutOK = true // Prevent the checkout directory from being erased again.
	logger.Info("checkout created")
}
