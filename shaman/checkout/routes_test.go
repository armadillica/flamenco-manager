/* (c) 2019, Blender Foundation - Sybren A. Stüvel
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
 */

package checkout

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/armadillica/flamenco-manager/shaman/filestore"
	"github.com/armadillica/flamenco-manager/shaman/httpserver"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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
