package config

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
