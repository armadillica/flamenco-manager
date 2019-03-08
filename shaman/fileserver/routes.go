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
