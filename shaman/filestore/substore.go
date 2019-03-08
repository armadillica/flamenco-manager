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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

type storageBin struct {
	basePath      string
	dirName       string
	hasTempSuffix bool
	fileSuffix    string
}

var (
	errNoWriteAllowed = errors.New("writing is only allowed in storage bins with a temp suffix")
)

func (s *storageBin) storagePrefix(partialPath string) string {
	return path.Join(s.basePath, s.dirName, partialPath)
}

// Returns whether 'someFullPath' is pointing to a path inside our storage for the given partial path.
// Only looks at the paths, does not perform any filesystem checks to see the file is actually there.
func (s *storageBin) contains(partialPath, someFullPath string) bool {
	expectedPrefix := s.storagePrefix(partialPath)
	return len(expectedPrefix) < len(someFullPath) && expectedPrefix == someFullPath[:len(expectedPrefix)]
}

// pathOrGlob returns either a path, or a glob when hasTempSuffix=true.
func (s *storageBin) pathOrGlob(partialPath string) string {
	pathOrGlob := s.storagePrefix(partialPath)
	if s.hasTempSuffix {
		pathOrGlob += "-*"
	}
	pathOrGlob += s.fileSuffix
	return pathOrGlob
}

// resolve finds a file '{basePath}/{dirName}/partialPath*{fileSuffix}'
// and returns its path. The * glob pattern is only used when
// hasTempSuffix is true.
func (s *storageBin) resolve(partialPath string) string {
	pathOrGlob := s.pathOrGlob(partialPath)

	if !s.hasTempSuffix {
		_, err := os.Stat(pathOrGlob)
		if err != nil {
			return ""
		}
		return pathOrGlob
	}

	matches, _ := filepath.Glob(pathOrGlob)
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// pathFor(somePath) returns that path inside the storage bin, including proper suffix.
// Note that this is only valid for bins without temp suffixes.
func (s *storageBin) pathFor(partialPath string) string {
	return s.storagePrefix(partialPath) + s.fileSuffix
}

// openForWriting makes sure there is a place to write to.
func (s *storageBin) openForWriting(partialPath string) (*os.File, error) {
	if !s.hasTempSuffix {
		return nil, errNoWriteAllowed
	}

	pathOrGlob := s.pathOrGlob(partialPath)
	dirname, filename := path.Split(pathOrGlob)

	if err := os.MkdirAll(dirname, 0777); err != nil {
		return nil, err
	}
	return ioutil.TempFile(dirname, filename)
}
