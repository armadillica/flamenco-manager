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
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/armadillica/flamenco-manager/shaman/config"
	"github.com/armadillica/flamenco-manager/shaman/filestore"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func createTestServer() (server *FileServer, cleanup func()) {
	config, configCleanup := config.CreateTestConfig()

	store := filestore.New(config)
	server = New(store)
	server.Go()

	cleanup = func() {
		server.Close()
		configCleanup()
	}
	return
}

func TestServeFile(t *testing.T) {
	server, cleanup := createTestServer()
	defer cleanup()

	payload := []byte("hähähä")
	checksum := "da-checksum-is-long-enough-like-this"
	filesize := int64(len(payload))

	server.fileStore.(*filestore.Store).MustStoreFileForTest(checksum, filesize, payload)

	respRec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/files/{checksum}/{filesize}", nil)
	req = mux.SetURLVars(req, map[string]string{
		"checksum": checksum,
		"filesize": strconv.FormatInt(filesize, 10),
	})
	server.ServeHTTP(respRec, req)

	assert.Equal(t, http.StatusOK, respRec.Code)
	assert.EqualValues(t, payload, respRec.Body.Bytes())
}
