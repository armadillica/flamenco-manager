package config

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
	"fmt"
	"io/ioutil"
	"os"
	"time"

	yaml "gopkg.in/yaml.v2"
)

const (
	configFilename = "shaman.yaml"
)

// Config contains all the Shaman configuration
type Config struct {
	// Used only for unit tests, so that they know where the temporary
	// directory created for this test is located.
	TestTempDir string `yaml:"-"`

	FileStorePath string `yaml:"fileStorePath"`
	CheckoutPath  string `yaml:"checkoutPath"`

	GarbageCollect GarbageCollect `yaml:"garbageCollect"`
}

// GarbageCollect contains the config options for the GC.
type GarbageCollect struct {
	// How frequently garbage collection is performed on the file store:
	Period time.Duration `yaml:"period"`
	// How old files must be before they are GC'd:
	MaxAge time.Duration `yaml:"maxAge"`
	// Paths to check for symlinks before GC'ing files.
	ExtraCheckoutDirs []string `yaml:"extraCheckoutPaths"`
}

// Load loads a config YAML file and returns its parsed content.
func Load(filename string) (Config, error) {
	if filename == "" {
		filename = configFilename
	}
	logger := packageLogger.WithField("filename", filename)

	config := Config{
		FileStorePath: "../shaman-file-store",
		CheckoutPath:  "../shaman-checkout",

		GarbageCollect: GarbageCollect{
			Period:            0,
			MaxAge:            31 * 24 * time.Hour,
			ExtraCheckoutDirs: []string{},
		},
	}

	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warning("config file not found, using defaults")
			return config, nil
		}
		return config, err
	}
	logger.Info("loading config file")

	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		return config, fmt.Errorf("unmarshal: %v", err)
	}

	return config, nil
}
