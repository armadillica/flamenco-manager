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
