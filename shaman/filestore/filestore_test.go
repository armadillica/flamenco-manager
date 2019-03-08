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
	"testing"

	"github.com/stretchr/testify/assert"
)

// mustCreateFile creates an empty file.
// The containing directory structure is created as well, if necessary.
func mustCreateFile(filepath string) {
	err := os.MkdirAll(path.Dir(filepath), 0777)
	if err != nil {
		panic(err)
	}

	file, err := os.Create(filepath)
	if err != nil {
		panic(err)
	}
	file.Close()
}

func TestCreateDirectories(t *testing.T) {
	store := CreateTestStore()
	defer CleanupTestStore(store)

	assert.Equal(t, path.Join(store.baseDir, "uploading", "x"), store.uploading.storagePrefix("x"))
	assert.Equal(t, path.Join(store.baseDir, "stored", "x"), store.stored.storagePrefix("x"))

	assert.DirExists(t, path.Join(store.baseDir, "uploading"))
	assert.DirExists(t, path.Join(store.baseDir, "stored"))
}

func TestResolveStoredFile(t *testing.T) {
	store := CreateTestStore()
	defer CleanupTestStore(store)

	foundPath, status := store.ResolveFile("abcdefxxx", 123, ResolveStoredOnly)
	assert.Equal(t, "", foundPath)
	assert.Equal(t, StatusDoesNotExist, status)

	fname := path.Join(store.baseDir, "stored", "ab", "cdefxxx", "123.blob")
	mustCreateFile(fname)

	foundPath, status = store.ResolveFile("abcdefxxx", 123, ResolveStoredOnly)
	assert.Equal(t, fname, foundPath)
	assert.Equal(t, StatusStored, status)

	foundPath, status = store.ResolveFile("abcdefxxx", 123, ResolveEverything)
	assert.Equal(t, fname, foundPath)
	assert.Equal(t, StatusStored, status)
}

func TestResolveUploadingFile(t *testing.T) {
	store := CreateTestStore()
	defer CleanupTestStore(store)

	foundPath, status := store.ResolveFile("abcdefxxx", 123, ResolveEverything)
	assert.Equal(t, "", foundPath)
	assert.Equal(t, StatusDoesNotExist, status)

	fname := path.Join(store.baseDir, "uploading", "ab", "cdefxxx", "123-unique-code.tmp")
	mustCreateFile(fname)

	foundPath, status = store.ResolveFile("abcdefxxx", 123, ResolveStoredOnly)
	assert.Equal(t, "", foundPath)
	assert.Equal(t, StatusDoesNotExist, status)

	foundPath, status = store.ResolveFile("abcdefxxx", 123, ResolveEverything)
	assert.Equal(t, fname, foundPath)
	assert.Equal(t, StatusUploading, status)
}

func TestOpenForUpload(t *testing.T) {
	store := CreateTestStore()
	defer CleanupTestStore(store)

	contents := []byte("je moešje")
	fileSize := int64(len(contents))

	file, err := store.OpenForUpload("abcdefxxx", fileSize)
	assert.Nil(t, err)
	file.Write(contents)
	file.Close()

	foundPath, status := store.ResolveFile("abcdefxxx", fileSize, ResolveEverything)
	assert.Equal(t, file.Name(), foundPath)
	assert.Equal(t, StatusUploading, status)

	readContents, err := ioutil.ReadFile(foundPath)
	assert.Nil(t, err)
	assert.EqualValues(t, contents, readContents)
}

func TestMoveToStored(t *testing.T) {
	store := CreateTestStore()
	defer CleanupTestStore(store)

	contents := []byte("je moešje")
	fileSize := int64(len(contents))

	err := store.MoveToStored("abcdefxxx", fileSize, "/just/some/path")
	assert.NotNil(t, err)

	file, err := store.OpenForUpload("abcdefxxx", fileSize)
	assert.Nil(t, err)
	file.Write(contents)
	file.Close()
	tempLocation := file.Name()

	err = store.MoveToStored("abcdefxxx", fileSize, file.Name())
	assert.Nil(t, err)

	foundPath, status := store.ResolveFile("abcdefxxx", fileSize, ResolveEverything)
	assert.NotEqual(t, file.Name(), foundPath)
	assert.Equal(t, StatusStored, status)

	assert.FileExists(t, foundPath)

	// The entire directory structure should be kept clean.
	assertDoesNotExist(t, tempLocation)
	assertDoesNotExist(t, path.Dir(tempLocation))
	assertDoesNotExist(t, path.Dir(path.Dir(tempLocation)))
}

func assertDoesNotExist(t *testing.T, path string) {
	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "%s should not exist, err=%v", path, err)
}
