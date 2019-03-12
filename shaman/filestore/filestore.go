package filestore

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
	"os"
	"path"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/armadillica/flamenco-manager/shaman/config"
)

// Store represents the default Shaman file store.
type Store struct {
	baseDir string

	uploading storageBin
	stored    storageBin
}

// New returns a new file store.
func New(conf config.Config) Storage {
	packageLogger.WithField("storageDir", conf.FileStorePath).Info("opening file store")
	store := &Store{
		conf.FileStorePath,
		storageBin{conf.FileStorePath, "uploading", true, ".tmp"},
		storageBin{conf.FileStorePath, "stored", false, ".blob"},
	}
	store.createDirectoryStructure()
	return store
}

// Create the base directory structure for this store.
func (s *Store) createDirectoryStructure() {
	mkdir := func(subdir string) {
		path := path.Join(s.baseDir, subdir)
		logger := packageLogger.WithField("path", path)
		stat, err := os.Stat(path)

		if err == nil {
			if stat.IsDir() {
				// Exists and is a directory; nothing to do.
				return
			}
			logger.Fatal("path exists but is not a directory")
		}

		if !os.IsNotExist(err) {
			logger.WithError(err).Fatal("unable to stat directory")
		}

		logger.Debug("creating directory")
		if err := os.MkdirAll(path, 0777); err != nil {
			logger.WithError(err).Fatal("unable to create directory")
		}
	}

	mkdir(s.uploading.dirName)
	mkdir(s.stored.dirName)
}

// StoragePath returns the directory path of the 'stored' storage bin.
func (s *Store) StoragePath() string {
	return path.Join(s.stored.basePath, s.stored.dirName)
}

// BasePath returns the directory path of the storage.
func (s *Store) BasePath() string {
	return s.baseDir
}

// Returns the checksum/filesize dependent parts of the file's path.
// To be combined with a base directory, status directory, and status-dependent suffix.
func (s *Store) partialFilePath(checksum string, filesize int64) string {
	return path.Join(checksum[0:2], checksum[2:], strconv.FormatInt(filesize, 10))
}

// ResolveFile checks the status of the file in the store.
func (s *Store) ResolveFile(checksum string, filesize int64, storedOnly StoredOnly) (path string, status FileStatus) {
	partial := s.partialFilePath(checksum, filesize)

	logger := packageLogger.WithFields(logrus.Fields{
		"checksum":    checksum,
		"filesize":    filesize,
		"partialPath": partial,
		"storagePath": s.baseDir,
	})

	if path = s.stored.resolve(partial); path != "" {
		// logger.WithField("path", path).Debug("found stored file")
		return path, StatusStored
	}
	if storedOnly != ResolveEverything {
		// logger.Debug("file does not exist in 'stored' state")
		return "", StatusDoesNotExist
	}

	if path = s.uploading.resolve(partial); path != "" {
		logger.WithField("path", path).Debug("found currently uploading file")
		return path, StatusUploading
	}
	// logger.Debug("file does not exist")
	return "", StatusDoesNotExist
}

// OpenForUpload returns a file pointer suitable to stream an uploaded file to.
func (s *Store) OpenForUpload(checksum string, filesize int64) (*os.File, error) {
	partial := s.partialFilePath(checksum, filesize)
	return s.uploading.openForWriting(partial)
}

// MoveToStored moves a file from 'uploading' to 'stored' storage.
// It is assumed that the checksum and filesize have been verified.
func (s *Store) MoveToStored(checksum string, filesize int64, uploadedFilePath string) error {
	// Check that the uploaded file path is actually in the 'uploading' storage.
	partial := s.partialFilePath(checksum, filesize)
	if !s.uploading.contains(partial, uploadedFilePath) {
		return ErrNotInUploading
	}

	// Move to the other storage bin.
	targetPath := s.stored.pathFor(partial)
	targetDir, _ := path.Split(targetPath)
	if err := os.MkdirAll(targetDir, 0777); err != nil {
		return err
	}
	logger := packageLogger.WithFields(logrus.Fields{
		"uploadedPath": uploadedFilePath,
		"storagePath":  targetPath,
	})
	logger.Debug("moving uploaded file to storage")
	if err := os.Rename(uploadedFilePath, targetPath); err != nil {
		return err
	}

	s.RemoveUploadedFile(uploadedFilePath)
	return nil
}

func (s *Store) removeFile(filePath string) error {
	err := os.Remove(filePath)
	if err != nil {
		packageLogger.WithError(err).Debug("unable to delete file; ignoring")
	}

	// Clean up directory structure, but ignore any errors (dirs may not be empty)
	directory := path.Dir(filePath)
	os.Remove(directory)
	os.Remove(path.Dir(directory))

	return err
}

// RemoveUploadedFile removes a file from the 'uploading' storage bin.
// Errors are ignored.
func (s *Store) RemoveUploadedFile(filePath string) {
	// Check that the file path is actually in the 'uploading' storage.
	if !s.uploading.contains("", filePath) {
		packageLogger.WithField("file", filePath).Error(
			"filestore.Store.RemoveUploadedFile called with file not in 'uploading' storage bin")
		return
	}
	s.removeFile(filePath)
}

// RemoveStoredFile removes a file from the 'stored' storage bin.
func (s *Store) RemoveStoredFile(filePath string) error {
	// Check that the file path is actually in the 'stored' storage.
	if !s.stored.contains("", filePath) {
		packageLogger.WithField("file", filePath).Error(
			"filestore.Store.RemoveStoredFile called with file not in 'stored' storage bin")
		return os.ErrNotExist
	}
	return s.removeFile(filePath)
}
