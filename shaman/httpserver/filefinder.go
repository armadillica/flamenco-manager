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

package httpserver

import (
	"os"
	"path/filepath"

	"github.com/kardianos/osext"
	"github.com/sirupsen/logrus"
)

// RootPath returns the filename prefix to find bundled files.
// Files are searched for relative to the current working directory as well as relative
// to the currently running executable.
func RootPath(fileToFind string) string {
	logger := packageLogger.WithField("fileToFind", fileToFind)

	// Find as relative path, i.e. relative to CWD.
	_, err := os.Stat(fileToFind)
	if err == nil {
		logger.Debug("found in current working directory")
		return ""
	}

	// Find relative to executable folder.
	exedirname, err := osext.ExecutableFolder()
	if err != nil {
		logger.WithError(err).Error("unable to determine the executable's directory")
		return ""
	}

	if _, err := os.Stat(filepath.Join(exedirname, fileToFind)); os.IsNotExist(err) {
		cwd, err := os.Getwd()
		if err != nil {
			logger.WithError(err).Error("unable to determine current working directory")
		}
		logger.WithFields(logrus.Fields{
			"cwd":        cwd,
			"exedirname": exedirname,
		}).Error("unable to find file")
		return ""
	}

	// Append a slash so that we can later just concatenate strings.
	logrus.WithField("exedirname", exedirname).Debug("found file")
	return exedirname + string(os.PathSeparator)
}
