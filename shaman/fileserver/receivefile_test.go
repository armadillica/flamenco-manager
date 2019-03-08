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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/armadillica/flamenco-manager/shaman/hasher"
	"github.com/armadillica/flamenco-manager/shaman/httpserver"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/armadillica/flamenco-manager/shaman/filestore"
)

func TestStoreFile(t *testing.T) {
	server, cleanup := createTestServer()
	defer cleanup()

	payload := []byte("hähähä")
	// Just to double-check it's encoded as UTF-8:
	assert.EqualValues(t, []byte("h\xc3\xa4h\xc3\xa4h\xc3\xa4"), payload)

	filesize := int64(len(payload))

	testWithChecksum := func(checksum string) *httptest.ResponseRecorder {
		compressedPayload := httpserver.CompressBuffer(payload)
		respRec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/files/{checksum}/{filesize}", compressedPayload)
		req = mux.SetURLVars(req, map[string]string{
			"checksum": checksum,
			"filesize": strconv.FormatInt(filesize, 10),
		})
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("X-Shaman-Original-Filename", "in-memory-file.txt")
		server.ServeHTTP(respRec, req)
		return respRec
	}

	var respRec *httptest.ResponseRecorder
	var path string
	var status filestore.FileStatus

	// A bad checksum should be rejected.
	badChecksum := "da-checksum-is-long-enough-like-this"
	respRec = testWithChecksum(badChecksum)
	assert.Equal(t, http.StatusExpectationFailed, respRec.Code)
	path, status = server.fileStore.ResolveFile(badChecksum, filesize, filestore.ResolveEverything)
	assert.Equal(t, filestore.StatusDoesNotExist, status)
	assert.Equal(t, "", path)

	// The correct checksum should be accepted.
	correctChecksum := hasher.Checksum(payload)
	respRec = testWithChecksum(correctChecksum)
	assert.Equal(t, http.StatusNoContent, respRec.Code)
	path, status = server.fileStore.ResolveFile(correctChecksum, filesize, filestore.ResolveEverything)
	assert.Equal(t, filestore.StatusStored, status)
	assert.FileExists(t, path)

	savedContent, err := ioutil.ReadFile(path)
	assert.Nil(t, err)
	assert.EqualValues(t, payload, savedContent, "The file should be saved uncompressed")
}
