package fileserver

/* ***** BEGIN MIT LICENSE BLOCK *****
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be
 * included in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 * ***** END MIT LICENCE BLOCK *****
 */

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/armadillica/flamenco-manager/shaman/auth"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// AddRoutes adds this package's routes to the Router.
func (fs *FileServer) AddRoutes(router *mux.Router, auther auth.Authenticator) {
	router.Handle("/files/{checksum}/{filesize}", auther.Wrap(fs)).Methods("GET", "POST", "OPTIONS")
}

func (fs *FileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logger := packageLogger.WithFields(auth.RequestLogFields(r))

	checksum, filesize, err := parseRequestVars(w, r)
	if err != nil {
		logger.WithError(err).Warning("invalid request")
		return
	}

	logger = logger.WithFields(logrus.Fields{
		"checksum": checksum,
		"filesize": filesize,
	})

	switch r.Method {
	case http.MethodOptions:
		logger.Info("checking file")
		fs.checkFile(r.Context(), w, checksum, filesize)
	case http.MethodGet:
		// TODO: make optional or just delete:
		logger.Info("serving file")
		fs.serveFile(r.Context(), w, checksum, filesize)
	case http.MethodPost:
		fs.receiveFile(r.Context(), w, r, checksum, filesize)
	default:
		// This should never be reached due to the router options, but just in case.
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func parseRequestVars(w http.ResponseWriter, r *http.Request) (string, int64, error) {
	vars := mux.Vars(r)
	checksum, ok := vars["checksum"]
	if !ok {
		http.Error(w, "missing checksum", http.StatusBadRequest)
		return "", 0, errors.New("missing checksum")
	}
	// Arbitrary minimum length, but we can fairly safely assume that all
	// hashing methods used produce a hash of at least 32 characters.
	if len(checksum) < 32 {
		http.Error(w, "checksum suspiciously short", http.StatusBadRequest)
		return "", 0, errors.New("checksum suspiciously short")
	}

	filesizeStr, ok := vars["filesize"]
	if !ok {
		http.Error(w, "missing filesize", http.StatusBadRequest)
		return "", 0, errors.New("missing filesize")
	}
	filesize, err := strconv.ParseInt(filesizeStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid filesize", http.StatusBadRequest)
		return "", 0, fmt.Errorf("invalid filesize: %v", err)
	}

	return checksum, filesize, nil
}
