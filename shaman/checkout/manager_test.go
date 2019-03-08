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
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/armadillica/flamenco-manager/shaman/config"
	"github.com/armadillica/flamenco-manager/shaman/filestore"
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
