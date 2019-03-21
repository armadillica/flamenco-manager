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

package bundledmongo

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kardianos/osext"
	log "github.com/sirupsen/logrus"
)

// Ensures the directory exists, or otherwise log.Fatal()s the process to death.
func ensureDirExists(directory, description string) {
	stat, err := os.Stat(directory)
	if os.IsNotExist(err) {
		log.Infof("Creating %s %s", description, directory)

		if err = os.MkdirAll(directory, 0700); err != nil {
			log.Fatalf("Unable to create %s %s: %s", description, directory, err)
		}

		return
	} else if err != nil {
		log.Fatalf("Unable to inspect %s %s: %s", description, directory, err)
	}

	if !stat.IsDir() {
		log.Fatalf("%s %s exists, but is not a directory. Move it out of the way.",
			strings.Title(description), directory)
	}
}

// Returns the filename as an absolute path.
// Relative paths are interpreted relative to the flamenco-manager executable.
func relativeToExecutable(filename string) (string, error) {
	if filepath.IsAbs(filename) {
		return filename, nil
	}

	exedirname, err := osext.ExecutableFolder()
	if err != nil {
		return "", err
	}

	return filepath.Join(exedirname, filename), nil
}

// Returns the absolute path of the mongod executable.
func absMongoDbPath() (string, error) {
	return relativeToExecutable(mongoDPath)
}
