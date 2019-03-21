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
	"os"
	"path"
	"testing"
	"time"

	"github.com/armadillica/flamenco-manager/shaman/config"
	"github.com/armadillica/flamenco-manager/shaman/filestore"
	"github.com/stretchr/testify/assert"
)

func createTestManager() (*Manager, func()) {
	conf, confCleanup := config.CreateTestConfig()
	fileStore := filestore.New(conf)
	manager := NewManager(conf, fileStore)
	return manager, confCleanup
}

func TestSymlinkToCheckout(t *testing.T) {
	manager, cleanup := createTestManager()
	defer cleanup()

	// Fake an older file.
	blobPath := path.Join(manager.checkoutBasePath, "jemoeder.blob")
	err := ioutil.WriteFile(blobPath, []byte("op je hoofd"), 0600)
	assert.Nil(t, err)

	wayBackWhen := time.Now().Add(-time.Hour * 24 * 100)
	err = os.Chtimes(blobPath, wayBackWhen, wayBackWhen)
	assert.Nil(t, err)

	symlinkRelativePath := "path/to/jemoeder.txt"
	err = manager.SymlinkToCheckout(blobPath, manager.checkoutBasePath, symlinkRelativePath)
	assert.Nil(t, err)

	// Wait for touch() calls to be done.
	manager.wg.Wait()

	// The blob should have been touched to indicate it was referenced just now.
	stat, err := os.Stat(blobPath)
	assert.Nil(t, err)
	assert.True(t,
		stat.ModTime().After(wayBackWhen),
		"File must be touched (%v must be later than %v)", stat.ModTime(), wayBackWhen)

	symlinkPath := path.Join(manager.checkoutBasePath, symlinkRelativePath)
	stat, err = os.Lstat(symlinkPath)
	assert.Nil(t, err)
	assert.True(t, stat.Mode()&os.ModeType == os.ModeSymlink,
		"%v should be a symlink", symlinkPath)
}
