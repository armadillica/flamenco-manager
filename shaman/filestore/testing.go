package filestore

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
	"path/filepath"
	"runtime"
	"time"

	"github.com/armadillica/flamenco-manager/shaman/config"
)

// CreateTestStore returns a Store that can be used for unit testing.
func CreateTestStore() *Store {
	tempDir, err := ioutil.TempDir("", "shaman-filestore-test-")
	if err != nil {
		panic(err)
	}

	conf := config.Config{
		FileStorePath: tempDir,
	}
	storage := New(conf)
	store, ok := storage.(*Store)
	if !ok {
		panic("storage should be *Store")
	}

	return store
}

// CleanupTestStore deletes a store returned by CreateTestStore()
func CleanupTestStore(store *Store) {
	if err := os.RemoveAll(store.baseDir); err != nil {
		panic(err)
	}
}

// MustStoreFileForTest allows a unit test to store some file in the 'stored' storage bin.
// Any error will cause a panic.
func (s *Store) MustStoreFileForTest(checksum string, filesize int64, contents []byte) {
	file, err := s.OpenForUpload(checksum, filesize)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	written, err := file.Write(contents)
	if err != nil {
		panic(err)
	}
	if written != len(contents) {
		panic("short write")
	}

	err = s.MoveToStored(checksum, filesize, file.Name())
	if err != nil {
		panic(err)
	}
}

// LinkTestFileStore creates a copy of _test_file_store by hard-linking files into a temporary directory.
// Panics if there are any errors.
func LinkTestFileStore(cloneTo string) {
	_, myFilename, _, _ := runtime.Caller(0)
	fileStorePath := path.Join(path.Dir(path.Dir(myFilename)), "_test_file_store")
	now := time.Now()

	visit := func(visitPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relpath, err := filepath.Rel(fileStorePath, visitPath)
		if err != nil {
			return err
		}

		targetPath := path.Join(cloneTo, relpath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		err = os.Link(visitPath, targetPath)
		if err != nil {
			return err
		}
		// Make sure we always test with fresh files by default.
		return os.Chtimes(targetPath, now, now)
	}
	if err := filepath.Walk(fileStorePath, visit); err != nil {
		panic(err)
	}
}
