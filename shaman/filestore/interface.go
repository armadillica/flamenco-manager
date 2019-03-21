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

package filestore

import (
	"errors"
	"os"
)

// Storage is the interface for Shaman file stores.
type Storage interface {
	// ResolveFile checks the status of the file in the store and returns the actual path.
	ResolveFile(checksum string, filesize int64, storedOnly StoredOnly) (string, FileStatus)

	// OpenForUpload returns a file pointer suitable to stream an uploaded file to.
	OpenForUpload(checksum string, filesize int64) (*os.File, error)

	// BasePath returns the directory path of the storage.
	// This is the directory containing the 'stored' and 'uploading' directories.
	BasePath() string

	// StoragePath returns the directory path of the 'stored' storage bin.
	StoragePath() string

	// MoveToStored moves a file from 'uploading' storage to the actual 'stored' storage.
	MoveToStored(checksum string, filesize int64, uploadedFilePath string) error

	// RemoveUploadedFile removes a file from the 'uploading' storage.
	// This is intended to clean up files for which upload was aborted for some reason.
	RemoveUploadedFile(filePath string)

	// RemoveStoredFile removes a file from the 'stored' storage bin.
	// This is intended to garbage collect old, unused files.
	RemoveStoredFile(filePath string) error
}

// FileStatus represents the status of a file in the store.
type FileStatus int

// Valid statuses for files in the store.
const (
	StatusNotSet FileStatus = iota
	StatusDoesNotExist
	StatusUploading
	StatusStored
)

// StoredOnly indicates whether to resolve only 'stored' files or also 'uploading' or 'checking'.
type StoredOnly bool

// For the ResolveFile() call. This is more explicit than just true/false values.
const (
	ResolveStoredOnly StoredOnly = true
	ResolveEverything StoredOnly = false
)

// Predefined errors
var (
	ErrFileDoesNotExist = errors.New("file does not exist")
	ErrNotInUploading   = errors.New("file not stored in 'uploading' storage")
)
