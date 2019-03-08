package httpserver

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
