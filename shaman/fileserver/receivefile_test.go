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
