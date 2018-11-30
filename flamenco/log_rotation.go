package flamenco

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

type numberedPath struct {
	path     string
	number   int
	basepath string
}

// byNumber implements the sort.Interface for numberedPath objects,
// and sorts in reverse (so highest number first).
type byNumber []numberedPath

func (a byNumber) Len() int           { return len(a) }
func (a byNumber) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byNumber) Less(i, j int) bool { return a[i].number > a[j].number }

func createNumberedPath(path string) numberedPath {
	dotIndex := strings.LastIndex(path, ".")
	if dotIndex < 0 {
		return numberedPath{path, -1, path}
	}
	asInt, err := strconv.Atoi(path[dotIndex+1:])
	if err != nil {
		return numberedPath{path, -1, path}
	}
	return numberedPath{path, asInt, path[:dotIndex]}
}

// rotateLogFile renames 'logpath' to 'logpath.1', and increases numbers for already-existing files.
func rotateLogFile(logpath string) error {
	logger := log.WithField("logpath", logpath)

	// Don't do anything if the file doesn't exist yet.
	_, err := os.Stat(logpath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("log file does not exist, no need to rotate")
			return nil
		}
		logger.WithError(err).Warning("unable to stat logfile")
		return err
	}

	pattern := logpath + ".*"
	existing, err := filepath.Glob(pattern)
	if err != nil {
		logger.WithError(err).WithField("glob", pattern).Warning("rotateLogFile: unable to glob")
		return err
	}
	if existing == nil {
		logger.Debug("rotateLogFile: no existing files to rotate")
	} else {
		// Rotate the files in reverse numerical order (so n→n+1 comes after n+1→n+2)
		var numbered = make(byNumber, len(existing))
		for idx := range existing {
			numbered[idx] = createNumberedPath(existing[idx])
		}
		sort.Sort(numbered)

		for _, numberedPath := range numbered {
			newName := numberedPath.basepath + "." + strconv.Itoa(numberedPath.number+1)
			err := os.Rename(numberedPath.path, newName)
			if err != nil {
				logger.WithFields(log.Fields{
					"from_path":  numberedPath.path,
					"to_path":    newName,
					log.ErrorKey: err,
				}).Error("rotateLogFile: unable to rename log file")
			}
		}
	}

	// Rotate the pointed-to file.
	newName := logpath + ".1"
	if err := os.Rename(logpath, newName); err != nil {
		logger.WithField("new_name", newName).WithError(err).Error("rotateLogFile: unable to rename log file for rotating")
		return err
	}

	return nil
}
