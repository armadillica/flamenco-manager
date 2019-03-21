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
