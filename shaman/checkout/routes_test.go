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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/armadillica/flamenco-manager/shaman/filestore"
	"github.com/armadillica/flamenco-manager/shaman/httpserver"
)

func TestReportRequirements(t *testing.T) {
	manager, cleanup := createTestManager()
	defer cleanup()

	defFile, err := ioutil.ReadFile("definition_test_example.txt")
	assert.Nil(t, err)
	compressedDefFile := httpserver.CompressBuffer(defFile)

	// 5 files, all ending in newline, so defFileLines has trailing "" element.
	defFileLines := strings.Split(string(defFile), "\n")
	assert.Equal(t, 6, len(defFileLines), defFileLines)

	respRec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/checkout/requirement", compressedDefFile)
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Content-Encoding", "gzip")
	manager.reportRequirements(respRec, req)

	bodyBytes, err := ioutil.ReadAll(respRec.Body)
	assert.Nil(t, err)
	body := string(bodyBytes)

	assert.Equal(t, respRec.Code, http.StatusOK, body)

	// We should not be required to upload the same file twice,
	// so another-routes.go should not be in the response.
	lines := strings.Split(body, "\n")
	expectLines := []string{
		"file-unknown definition.go",
		"file-unknown logging.go",
		"file-unknown manager.go",
		"file-unknown routes.go",
		"",
	}
	assert.EqualValues(t, expectLines, lines)
}

func TestCreateCheckout(t *testing.T) {
	manager, cleanup := createTestManager()
	defer cleanup()

	filestore.LinkTestFileStore(manager.fileStore.BasePath())

	defFile, err := ioutil.ReadFile("../_test_file_store/checkout_definition.txt")
	assert.Nil(t, err)
	compressedDefFile := httpserver.CompressBuffer(defFile)

	respRec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/checkout/create/{checkoutID}", compressedDefFile)
	req = mux.SetURLVars(req, map[string]string{
		"checkoutID": "jemoeder",
	})
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Content-Encoding", "gzip")
	logrus.SetLevel(logrus.DebugLevel)
	manager.createCheckout(respRec, req)

	bodyBytes, err := ioutil.ReadAll(respRec.Body)
	assert.Nil(t, err)
	body := string(bodyBytes)
	assert.Equal(t, http.StatusOK, respRec.Code, body)

	// Check the symlinks of the checkout
	coPath := path.Join(manager.checkoutBasePath, "er", "jemoeder")
	assert.FileExists(t, path.Join(coPath, "subdir", "replacer.py"))
	assert.FileExists(t, path.Join(coPath, "feed.py"))
	assert.FileExists(t, path.Join(coPath, "httpstuff.py"))
	assert.FileExists(t, path.Join(coPath, "filesystemstuff.py"))

	storePath := manager.fileStore.StoragePath()
	assertLinksTo(t, path.Join(coPath, "subdir", "replacer.py"),
		path.Join(storePath, "59", "0c148428d5c35fab3ebad2f3365bb469ab9c531b60831f3e826c472027a0b9", "3367.blob"))
	assertLinksTo(t, path.Join(coPath, "feed.py"),
		path.Join(storePath, "80", "b749c27b2fef7255e7e7b3c2029b03b31299c75ff1f1c72732081c70a713a3", "7488.blob"))
	assertLinksTo(t, path.Join(coPath, "httpstuff.py"),
		path.Join(storePath, "91", "4853599dd2c351ab7b82b219aae6e527e51518a667f0ff32244b0c94c75688", "486.blob"))
	assertLinksTo(t, path.Join(coPath, "filesystemstuff.py"),
		path.Join(storePath, "d6", "fc7289b5196cc96748ea72f882a22c39b8833b457fe854ef4c03a01f5db0d3", "7217.blob"))
}

func assertLinksTo(t *testing.T, linkPath, expectedTarget string) {
	actualTarget, err := os.Readlink(linkPath)
	assert.Nil(t, err)
	assert.Equal(t, expectedTarget, actualTarget)
}
