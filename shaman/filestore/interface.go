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
