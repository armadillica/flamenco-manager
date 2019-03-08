package shaman

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
	"time"

	"github.com/sirupsen/logrus"
)

// Mapping from absolute path to the file's mtime.
type mtimeMap map[string]time.Time

func (s *Server) periodicCleanup() {
	defer packageLogger.Debug("shutting down period cleanup")
	defer s.wg.Done()

	// Give the server some time to start up before we do the first GC.
	select {
	case <-s.shutdownChan:
		return
	case <-time.After(3 * time.Second):
	}

	for {
		s.GCStorage(false)

		select {
		case <-s.shutdownChan:
			return
		case <-time.After(s.config.GarbageCollect.Period):
		}
	}
}

func (s *Server) gcAgeThreshold() time.Time {
	return time.Now().Add(-s.config.GarbageCollect.MaxAge).Round(1 * time.Second)

}

// GCStorage performs garbage collection by deleting files from storage
// that are not symlinked in a checkout and haven't been touched since
// a threshold date.
func (s *Server) GCStorage(doDryRun bool) {
	ageThreshold := s.gcAgeThreshold()

	logger := packageLogger.WithFields(
		logrus.Fields{
			"checkoutPath":  s.config.CheckoutPath,
			"fileStorePath": s.fileStore.StoragePath(),
			"ageThreshold":  ageThreshold,
		})
	if doDryRun {
		logger = logger.WithField("dryRun", doDryRun)
	}

	logger.Info("performing garbage collection on storage")

	// Scan the storage for all the paths that are older than the threshold.
	oldFiles, err := s.gcFindOldFiles(ageThreshold, logger)
	if err != nil {
		logger.WithError(err).Error("unable to walk file store path to find old files")
		return
	}
	if len(oldFiles) == 0 {
		logger.Debug("found no old files during garbage collection scan")
		return
	}

	numOldFiles := len(oldFiles)
	logger.WithField("numOldFiles", numOldFiles).Info("found old files, going to check for links")

	// Scan the checkout area and extra checkout paths, and discard any old file that is linked.
	dirsToCheck := []string{s.config.CheckoutPath}
	dirsToCheck = append(dirsToCheck, s.config.GarbageCollect.ExtraCheckoutDirs...)
	for _, checkDir := range dirsToCheck {
		if err := s.gcFilterLinkedFiles(checkDir, oldFiles, logger); err != nil {
			logger.WithFields(logrus.Fields{
				"checkoutPath":  checkDir,
				logrus.ErrorKey: err,
			}).Error("unable to walk checkout path to find symlinks")
			return
		}
	}

	if len(oldFiles) == 0 {
		logger.Debug("all old files are in use")
		return
	}

	infoLogger := logger.WithFields(logrus.Fields{
		"numUnusedOldFiles":    len(oldFiles),
		"numStillUsedOldFiles": numOldFiles - len(oldFiles),
	})
	infoLogger.Info("found unused old files, going to delete")

	deletedFiles, deletedBytes := s.gcDeleteOldFiles(doDryRun, oldFiles, logger)

	infoLogger.WithFields(logrus.Fields{
		"numDeleted":    deletedFiles,
		"numNotDeleted": len(oldFiles) - deletedFiles,
		"freedBytes":    deletedBytes,
		"freedSize":     humanizeByteSize(deletedBytes),
	}).Info("removed unused old files")
}

func (s *Server) gcFindOldFiles(ageThreshold time.Time, logger *logrus.Entry) (mtimeMap, error) {
	oldFiles := mtimeMap{}
	visit := func(path string, info os.FileInfo, err error) error {
		select {
		case <-s.shutdownChan:
			return filepath.SkipDir
		default:
		}

		if err != nil {
			logger.WithError(err).Debug("error while walking file store path to find old files")
			return err
		}
		if info.IsDir() {
			return nil
		}
		modTime := info.ModTime()
		isOld := modTime.Before(ageThreshold)
		logger.WithFields(logrus.Fields{
			"path":      path,
			"mtime":     info.ModTime(),
			"threshold": ageThreshold,
			"isOld":     isOld,
		}).Debug("comparing mtime")
		if isOld {
			oldFiles[path] = modTime
		}
		return nil
	}
	if err := filepath.Walk(s.fileStore.StoragePath(), visit); err != nil {
		logger.WithError(err).Error("unable to walk file store path to find old files")
		return nil, err
	}

	return oldFiles, nil
}

// gcFilterLinkedFiles removes all still-symlinked paths from 'oldFiles'.
func (s *Server) gcFilterLinkedFiles(checkoutPath string, oldFiles mtimeMap, logger *logrus.Entry) error {
	logger = logger.WithField("checkoutPath", checkoutPath)

	visit := func(path string, info os.FileInfo, err error) error {
		select {
		case <-s.shutdownChan:
			return filepath.SkipDir
		default:
		}

		if err != nil {
			logger.WithError(err).Debug("error while walking checkout path while searching for symlinks")
			return err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink == 0 {
			return nil
		}

		linkTarget, err := filepath.EvalSymlinks(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}

			logger.WithFields(logrus.Fields{
				"linkPath":      path,
				logrus.ErrorKey: err,
			}).Warning("unable to determine target of symlink; ignoring")
			return nil
		}

		// Delete the link target from the old files, if it was there at all.
		delete(oldFiles, linkTarget)
		return nil
	}
	if err := filepath.Walk(checkoutPath, visit); err != nil {
		logger.WithError(err).Error("unable to walk checkout path while searching for symlinks")
		return err
	}

	return nil
}

func (s *Server) gcDeleteOldFiles(doDryRun bool, oldFiles mtimeMap, logger *logrus.Entry) (int, int64) {
	deletedFiles := 0
	var deletedBytes int64
	for path, lastSeenModTime := range oldFiles {
		pathLogger := logger.WithField("path", path)

		if stat, err := os.Stat(path); err != nil {
			if !os.IsNotExist(err) {
				pathLogger.WithError(err).Warning("unable to stat to-be-deleted file")
			}
		} else if stat.ModTime().After(lastSeenModTime) {
			pathLogger.Info("not deleting recently-touched file")
			continue
		} else {
			deletedBytes += stat.Size()
		}

		if doDryRun {
			pathLogger.Info("would delete unused file")
		} else {
			pathLogger.Info("deleting unused file")
			if err := s.fileStore.RemoveStoredFile(path); err == nil {
				deletedFiles++
			}
		}
	}

	return deletedFiles, deletedBytes
}
