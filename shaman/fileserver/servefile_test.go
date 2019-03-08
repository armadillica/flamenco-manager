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
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/armadillica/flamenco-manager/shaman/config"
	"github.com/armadillica/flamenco-manager/shaman/filestore"
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
