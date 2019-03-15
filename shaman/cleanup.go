package shaman

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
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// Mapping from absolute path to the file's mtime.
type mtimeMap map[string]time.Time

// GCStats contains statistics of a garbage collection run.
type GCStats struct {
	numSymlinksChecked   int
	numOldFiles          int
	numUnusedOldFiles    int
	numStillUsedOldFiles int
	numFilesDeleted      int
	numFilesNotDeleted   int
	bytesDeleted         int64
}

func (s *Server) periodicCleanup() {
	defer packageLogger.Debug("shutting down period cleanup")
	defer s.wg.Done()

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
func (s *Server) GCStorage(doDryRun bool) (stats GCStats) {
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

	stats.numOldFiles = len(oldFiles)
	stats.numFilesNotDeleted = stats.numOldFiles
	logger.WithField("numOldFiles", stats.numOldFiles).Info("found old files, going to check for links")

	// Scan the checkout area and extra checkout paths, and discard any old file that is linked.
	dirsToCheck := []string{s.config.CheckoutPath}
	dirsToCheck = append(dirsToCheck, s.config.GarbageCollect.ExtraCheckoutDirs...)
	for _, checkDir := range dirsToCheck {
		if err := s.gcFilterLinkedFiles(checkDir, oldFiles, logger, &stats); err != nil {
			logger.WithFields(logrus.Fields{
				"checkoutPath":  checkDir,
				logrus.ErrorKey: err,
			}).Error("unable to walk checkout path to find symlinks")
			return
		}
	}
	stats.numStillUsedOldFiles = stats.numOldFiles - len(oldFiles)
	stats.numUnusedOldFiles = len(oldFiles)
	infoLogger := logger.WithFields(logrus.Fields{
		"numUnusedOldFiles":    stats.numUnusedOldFiles,
		"numStillUsedOldFiles": stats.numStillUsedOldFiles,
		"numSymlinksChecked":   stats.numSymlinksChecked,
	})

	if len(oldFiles) == 0 {
		infoLogger.Info("all old files are in use")
		return
	}

	infoLogger.Info("found unused old files, going to delete")

	stats.numFilesDeleted, stats.bytesDeleted = s.gcDeleteOldFiles(doDryRun, oldFiles, logger)
	stats.numFilesNotDeleted = stats.numOldFiles - stats.numFilesDeleted

	infoLogger.WithFields(logrus.Fields{
		"numFilesDeleted":    stats.numFilesDeleted,
		"numFilesNotDeleted": stats.numFilesNotDeleted,
		"freedBytes":         stats.bytesDeleted,
		"freedSize":          humanizeByteSize(stats.bytesDeleted),
	}).Info("removed unused old files")

	return
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
		// logger.WithFields(logrus.Fields{
		// 	"path":      path,
		// 	"mtime":     info.ModTime(),
		// 	"threshold": ageThreshold,
		// 	"isOld":     isOld,
		// }).Debug("comparing mtime")
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
func (s *Server) gcFilterLinkedFiles(checkoutPath string, oldFiles mtimeMap, logger *logrus.Entry, stats *GCStats) error {
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

		if stats != nil {
			stats.numSymlinksChecked++
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
