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

package flamenco

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/armadillica/flamenco-manager/dynamicpool/dppoller"
	"github.com/armadillica/flamenco-manager/jwtauth"
	shamanconfig "github.com/armadillica/flamenco-manager/shaman/config"
	yaml "gopkg.in/yaml.v2"
)

const (
	configFilename   = "flamenco-manager.yaml"
	defaultServerURL = "https://cloud.blender.org/"

	latestConfigVersion = 2

	// relative to the Flamenco Server Base URL:
	jwtPublicKeysRelativeURL = "api/flamenco/jwt/public-keys"
)

var (
	// ErrMissingVariablePlatform is returned when a variable doesn't declare any valid platform for a certain value.
	ErrMissingVariablePlatform = errors.New("variable's value is missing platform declaration")
	// ErrBadDirection is returned when a direction doesn't match "oneway" or "twoway"
	ErrBadDirection = errors.New("variable's direction is invalid")

	// Valid values for the "mode" config variable.
	validModes = map[string]bool{
		"develop":    true,
		"production": true,
	}

	// Valid values for the "audience" tag of a ConfV2 variable.
	validAudiences = map[string]bool{
		"all":     true,
		"workers": true,
		"users":   true,
	}

	// The default configuration, use DefaultConfig() to obtain a copy.
	defaultConfig = Conf{
		Base: Base{
			Meta: ConfMeta{Version: latestConfigVersion},

			Mode:                        "production",
			ManagerName:                 "Flamenco Manager",
			Listen:                      ":8080",
			ListenHTTPS:                 ":8433",
			DatabasePath:                "./db",
			TaskLogsPath:                "./task-logs",
			DownloadTaskSleep:           10 * time.Minute,
			DownloadTaskRecheckThrottle: 10 * time.Second,
			TaskUpdatePushMaxInterval:   5 * time.Second,
			TaskUpdatePushMaxCount:      3000,
			CancelTaskFetchInterval:     10 * time.Second,
			ActiveTaskTimeoutInterval:   10 * time.Minute,
			ActiveWorkerTimeoutInterval: 1 * time.Minute,
			FlamencoStr:                 defaultServerURL,
			// Days are assumed to be 24 hours long. This is not exactly accurate, but should
			// be accurate enough for this type of cleanup.
			TaskCleanupMaxAge: 14 * 24 * time.Hour,
			SSDPDiscovery:     true,
			SSDPDeviceUUID:    "7401c189-ef69-434b-b4d8-56d00075faf5",

			BlacklistThreshold:         3,
			TaskFailAfterSoftFailCount: 3,

			WorkerCleanupStatus: []string{workerStatusOffline},

			TestTasks: TestTasks{
				BlenderRender: BlenderRenderConfig{
					JobStorage:   "{job_storage}/test-jobs",
					RenderOutput: "{render}/test-renders",
				},
			},

			Shaman: shamanconfig.Config{
				FileStorePath: "../shaman-file-store",
				CheckoutPath:  "../shaman-checkout",

				GarbageCollect: shamanconfig.GarbageCollect{
					Period:            0,
					MaxAge:            31 * 24 * time.Hour,
					ExtraCheckoutDirs: []string{},
				},
			},

			JWT: jwtauth.Config{
				DownloadKeysInterval: 1 * time.Hour,
			},
		},

		Variables: map[string]ConfV2Variable{
			"blender": ConfV2Variable{
				Direction: "oneway",
				Values: ConfV2VariableValues{
					ConfV2VariableValue{Platform: "linux", Audience: "users", Value: "/linux/path/to/blender"},
					ConfV2VariableValue{Platform: "linux", Audience: "workers", Value: "/farm/path/to/blender"},
					ConfV2VariableValue{Platform: "windows", Value: "C:/windows/path/to/blender.exe"},
					ConfV2VariableValue{Platform: "darwin", Value: "/Volumes/Applications/Blender/blender"},
				},
			},
			"ffmpeg": ConfV2Variable{
				Direction: "oneway",
				Values: ConfV2VariableValues{
					ConfV2VariableValue{Platform: "linux", Value: "/usr/bin/ffmpeg"},
					ConfV2VariableValue{Platform: "windows", Value: "C:/windows/path/to/ffmpeg.exe"},
					ConfV2VariableValue{Platform: "darwin", Value: "/Volumes/Applications/FFmpeg/ffmpeg"},
				},
			},
			"render": ConfV2Variable{
				Direction: "twoway",
				Values: ConfV2VariableValues{
					ConfV2VariableValue{Platform: "linux", Value: "/render"},
					ConfV2VariableValue{Platform: "windows", Value: "R:"},
					ConfV2VariableValue{Platform: "darwin", Value: "/Volumes/render"},
				},
			},
			"job_storage": ConfV2Variable{
				Direction: "twoway",
				Values: ConfV2VariableValues{
					ConfV2VariableValue{Platform: "linux", Value: "/shared/flamenco-jobs"},
					ConfV2VariableValue{Platform: "windows", Value: "S:"},
					ConfV2VariableValue{Platform: "darwin", Value: "/Volumes/shared/flamenco-jobs"},
				},
			},
		},
	}
)

// BlenderRenderConfig represents the configuration required for a test render.
type BlenderRenderConfig struct {
	JobStorage   string `yaml:"job_storage"`
	RenderOutput string `yaml:"render_output"`
}

// TestTasks represents the 'test_tasks' key in the Manager's configuration file.
type TestTasks struct {
	BlenderRender BlenderRenderConfig `yaml:"test_blender_render"`
}

// ConfMeta contains configuration file metadata.
type ConfMeta struct {
	// Version of the config file structure.
	Version int `yaml:"version"`
}

// Base contains those settings that are shared by all configuration versions.
type Base struct {
	Meta ConfMeta `yaml:"_meta"`

	Mode          string   `yaml:"mode"` // either "develop" or "production"
	ManagerName   string   `yaml:"manager_name"`
	DatabaseURL   string   `yaml:"database_url"`
	DatabasePath  string   `yaml:"database_path"`
	TaskLogsPath  string   `yaml:"task_logs_path"`
	Listen        string   `yaml:"listen"`
	ListenHTTPS   string   `yaml:"listen_https"`
	OwnURL        string   `yaml:"own_url"` // sent to workers via SSDP/UPnP
	FlamencoStr   string   `yaml:"flamenco"`
	Flamenco      *url.URL `yaml:"-"`
	ManagerID     string   `yaml:"manager_id"`
	ManagerSecret string   `yaml:"manager_secret,omitempty"`

	// TLS certificate management. TLSxxx has priority over ACME.
	TLSKey         string `yaml:"tlskey"`
	TLSCert        string `yaml:"tlscert"`
	ACMEDomainName string `yaml:"acme_domain_name"` // for the ACME Let's Encrypt client

	DownloadTaskSleep time.Duration `yaml:"download_task_sleep"`

	/* The number of seconds between rechecks when there are no more tasks for workers.
	 * If set to 0, will not throttle at all.
	 * If set to -1, will never check when a worker asks for a task (so only every
	 * download_task_sleep_seconds seconds). */
	DownloadTaskRecheckThrottle time.Duration `yaml:"download_task_recheck_throttle"`

	TaskUpdatePushMaxInterval time.Duration `yaml:"task_update_push_max_interval"`
	TaskUpdatePushMaxCount    int           `yaml:"task_update_push_max_count"`
	CancelTaskFetchInterval   time.Duration `yaml:"cancel_task_fetch_max_interval"`

	ActiveTaskTimeoutInterval   time.Duration `yaml:"active_task_timeout_interval"`
	ActiveWorkerTimeoutInterval time.Duration `yaml:"active_worker_timeout_interval"`

	TaskCleanupMaxAge   time.Duration `yaml:"task_cleanup_max_age"`
	WorkerCleanupMaxAge time.Duration `yaml:"worker_cleanup_max_age"`
	WorkerCleanupStatus []string      `yaml:"worker_cleanup_status"`

	/* This many failures (on a given job+task type combination) will ban a worker
	 * from that task type on that job. */
	BlacklistThreshold int `yaml:"blacklist_threshold"`

	// When this many workers have tried the task and failed, it will be hard-failed
	// (even when there are workers left that could technically retry the task).
	TaskFailAfterSoftFailCount int `yaml:"task_fail_after_softfail_count"`

	WatchForLatestImage string `yaml:"watch_for_latest_image"`

	SSDPDiscovery  bool   `yaml:"ssdp_discovery"`
	SSDPDeviceUUID string `yaml:"ssdp_device_uuid"`

	TestTasks TestTasks `yaml:"test_tasks"`

	// Shaman configuration settings.
	Shaman shamanconfig.Config `yaml:"shaman"`

	// Authentication settings.
	JWT                      jwtauth.Config `yaml:"user_authentication"`
	WorkerRegistrationSecret string         `yaml:"worker_registration_secret"`

	// Dynamic worker pools (Azure Batch, Google Compute, AWS, that sort).
	DynamicPoolPlatforms *dppoller.Config `yaml:"dynamic_pool_platforms,omitempty"`

	Websetup *WebsetupConf `yaml:"websetup,omitempty"`
}

// Conf is the latest version of the configuration.
// Currently it is version 2.
type Conf struct {
	Base `yaml:",inline"`

	// Variable name → Variable definition
	Variables map[string]ConfV2Variable `yaml:"variables"`

	// audience + platform + variable name → variable value.
	// Used to look up variables for a given platform and audience.
	// The 'audience' is never "all" or ""; only concrete audiences are stored here.
	VariablesLookup map[string]map[string]map[string]string `yaml:"-"`
}

// ConfV2Variable defines a variable (in config version 2)
type ConfV2Variable struct {
	// Either "oneway" or "twoway"
	Direction string `yaml:"direction" json:"direction"`
	// Mapping from variable value to audience/platform definition.
	Values ConfV2VariableValues `yaml:"values" json:"values"`
}

// ConfV2VariableValues is the list of values of a variable.
type ConfV2VariableValues []ConfV2VariableValue

// ConfV2VariableValue defines which audience and platform see which value.
type ConfV2VariableValue struct {
	// Audience defines who will use this variable, either "all", "workers", or "users". Empty string is "all".
	Audience string `yaml:"audience,omitempty" json:"audience,omitempty"`

	// Platforms that use this value. Only one of "Platform" and "Platforms" may be set.
	Platform  string   `yaml:"platform,omitempty" json:"platform,omitempty"`
	Platforms []string `yaml:"platforms,omitempty,flow" json:"platforms,omitempty,flow"`

	// The actual value of the variable for this audience+platform.
	Value string `yaml:"value" json:"value"`
}

// WebsetupConf are settings used by the web setup mode.
type WebsetupConf struct {
	// When true, the websetup will hide certain settings that are infrastructure-specific.
	// For example, it hides MongoDB choice, port numbers, task log directory, all kind of
	// hosting-specific things. This is used, for example, by the automated Azure deployment
	// to avoid messing up settings that are specific to that particular installation.
	HideInfraSettings bool `yaml:"hide_infra_settings"`
}

// GetConf parses flamenco-manager.yaml and returns its contents as a Conf object.
func GetConf() (Conf, error) {
	return LoadConf(configFilename)
}

// DefaultConfig returns a copy of the default configuration.
func DefaultConfig() Conf {
	c := defaultConfig
	c.Meta.Version = latestConfigVersion
	c.constructVariableLookupTable()
	return c
}

// LoadConf parses the given file and returns its contents as a Conf object.
func LoadConf(filename string) (Conf, error) {
	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		return DefaultConfig(), err
	}

	// First parse attempt, find the version.
	baseConf := Base{}
	if err := yaml.Unmarshal(yamlFile, &baseConf); err != nil {
		log.WithError(err).Fatal("unable to parse YAML as Base config")
	}
	logger := log.WithField("version", baseConf.Meta.Version)
	if baseConf.Meta.Version < latestConfigVersion {
		logger = logger.WithField("latestVersion", latestConfigVersion)
	}

	// Second parse attempt, based on the version found.
	c := DefaultConfig()
	switch baseConf.Meta.Version {
	case 0, 1:
		logger.Info("upgrading config to version 2")
		confv1 := defaultConfigV1()
		if err := yaml.Unmarshal(yamlFile, &confv1); err != nil {
			logger.WithError(err).Fatal("unable to parse YAML")
		}
		confv1.upgradeToV2(&c)
	case 2:
		log.Debug("interpreting settings as version 2")
		if err := yaml.Unmarshal(yamlFile, &c); err != nil {
			logger.WithError(err).Fatal("unable to parse YAML")
		}
	}

	c.constructVariableLookupTable()
	c.parseURLs()
	c.checkMode(c.Mode)
	c.checkDatabase()
	c.checkVariables()
	c.checkTLS()

	return c, nil
}

func (c *Conf) constructVariableLookupTable() {
	lookup := map[string]map[string]map[string]string{}

	// Construct a list of all audiences except "" and "all"
	concreteAudiences := []string{}
	isWildcard := map[string]bool{"": true, "all": true}
	for audience := range validAudiences {
		if isWildcard[audience] {
			continue
		}
		concreteAudiences = append(concreteAudiences, audience)
	}
	log.WithFields(log.Fields{
		"concreteAudiences": concreteAudiences,
		"isWildcard":        isWildcard,
	}).Debug("constructing variable lookup table")

	// setValue expands wildcard audiences into concrete ones.
	var setValue func(audience, platform, name, value string)
	setValue = func(audience, platform, name, value string) {
		if isWildcard[audience] {
			for _, aud := range concreteAudiences {
				setValue(aud, platform, name, value)
			}
			return
		}

		if lookup[audience] == nil {
			lookup[audience] = map[string]map[string]string{}
		}
		if lookup[audience][platform] == nil {
			lookup[audience][platform] = map[string]string{}
		}
		log.WithFields(log.Fields{
			"audience": audience,
			"platform": platform,
			"name":     name,
			"value":    value,
		}).Debug("setting variable")
		lookup[audience][platform][name] = value
	}

	// Construct the lookup table for each audience+platform+name
	for name, variable := range c.Variables {
		log.WithFields(log.Fields{
			"name":     name,
			"variable": variable,
		}).Debug("handling variable")
		for _, value := range variable.Values {

			// Two-way values should not end in path separator.
			// Given a variable 'apps' with value '/path/to/apps',
			// '/path/to/apps/blender' should be remapped to '{apps}/blender'.
			if variable.Direction == "twoway" {
				if strings.Contains(value.Value, "\\") {
					log.WithFields(log.Fields{
						"variable": name,
						"value":    value,
					}).Warning("Backslash found in variable value. Change paths to use forward slashes instead.")
				}
				value.Value = strings.TrimRight(value.Value, "/")
			}

			if value.Platform != "" {
				setValue(value.Audience, value.Platform, name, value.Value)
			}
			for _, platform := range value.Platforms {
				setValue(value.Audience, platform, name, value.Value)
			}
		}
	}
	log.WithFields(log.Fields{
		"variables": c.Variables,
		"lookup":    lookup,
	}).Debug("constructed lookup table")
	c.VariablesLookup = lookup
}

// ExpandVariables converts "{variable name}" to the value that belongs to the given audience and platform.
func (c *Conf) ExpandVariables(valueToExpand, audience, platform string) string {
	audienceMap := c.VariablesLookup[audience]
	if audienceMap == nil {
		log.WithFields(log.Fields{
			"valueToExpand": valueToExpand,
			"audience":      audience,
			"platform":      platform,
		}).Warning("no variables defined for this audience")
		return valueToExpand
	}

	platformMap := audienceMap[platform]
	if platformMap == nil {
		log.WithFields(log.Fields{
			"valueToExpand": valueToExpand,
			"audience":      audience,
			"platform":      platform,
		}).Warning("no variables defined for this platform given this audience")
		return valueToExpand
	}

	// Variable replacement
	for varname, varvalue := range platformMap {
		placeholder := fmt.Sprintf("{%s}", varname)
		valueToExpand = strings.Replace(valueToExpand, placeholder, varvalue, -1)
	}

	return valueToExpand
}

// checkVariables performs some basic checks on variable definitions.
// Note that the returned error only reflects the last-found error.
// All errors are logged, though.
func (c *Conf) checkVariables() error {
	var err error

	directionNames := []string{"oneway", "twoway"}
	validDirections := map[string]bool{}
	for _, direction := range directionNames {
		validDirections[direction] = true
	}

	for name, variable := range c.Variables {
		if !validDirections[variable.Direction] {
			log.WithFields(log.Fields{
				"name":      name,
				"direction": variable.Direction,
			}).Errorf("variable has invalid direction, choose from %v", directionNames)
			err = ErrBadDirection
		}
		for valueIndex, value := range variable.Values {
			// No platforms at all.
			if value.Platform == "" && len(value.Platforms) == 0 {
				log.WithFields(log.Fields{
					"name":  name,
					"value": value,
				}).Error("variable has a platformless value")

				err = ErrMissingVariablePlatform
				continue
			}

			// Both Platform and Platforms.
			if value.Platform != "" && len(value.Platforms) > 0 {
				log.WithFields(log.Fields{
					"name":      name,
					"value":     value,
					"platform":  value.Platform,
					"platforms": value.Platforms,
				}).Warning("variable has a both 'platform' and 'platforms' set")
				value.Platforms = append(value.Platforms, value.Platform)
				value.Platform = ""
			}

			if value.Audience == "" {
				value.Audience = "all"
			} else if !validAudiences[value.Audience] {
				log.WithFields(log.Fields{
					"name":     name,
					"value":    value,
					"audience": value.Audience,
				}).Error("variable invalid audience")
			}

			variable.Values[valueIndex] = value
		}
	}

	return err
}

func (c *Conf) checkDatabase() {
	// At least one of DatabasePath or DatabaseURL must be given.
	if c.DatabasePath == "" && c.DatabaseURL == "" {
		log.Fatal("Configure either database_path or database_url; the cannot both be empty.")
	}
}

// Overwrite stores this configuration object as flamenco-manager.yaml.
func (c *Conf) Overwrite() error {
	tempFilename := configFilename + "~"
	if err := c.Write(tempFilename); err != nil {
		return err
	}

	log.Debugf("Renaming %s to %s", tempFilename, configFilename)
	if err := os.Rename(tempFilename, configFilename); err != nil {
		return err
	}

	log.Infof("Saved configuration file to %s", configFilename)
	return nil
}

// Write saves the current in-memory configuration to a YAML file.
func (c *Conf) Write(filename string) error {
	// Convert back to string representation if necessary.
	if c.Flamenco != nil {
		c.FlamencoStr = c.Flamenco.String()
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	fmt.Fprintln(f, "# Configuration file for Flamenco Manager.")
	fmt.Fprintln(f, "# For an explanation of the fields, refer to flamenco-manager-example.yaml")
	fmt.Fprintln(f, "#")
	fmt.Fprintln(f, "# NOTE: this file will be overwritten by Flamenco Manager's web-based configuration system.")
	fmt.Fprintln(f, "#")
	now := time.Now()
	fmt.Fprintf(f, "# This file was written on %s\n\n", now.Format("2006-01-02 15:04:05 -07:00"))

	n, err := f.Write(data)
	if err != nil {
		return err
	}
	if n < len(data) {
		return io.ErrShortWrite
	}
	if err = f.Close(); err != nil {
		return err
	}

	log.Debugf("Config file written to %s", filename)
	return nil
}

// HasCustomTLS returns true if both the TLS certificate and key files are configured.
func (c *Conf) HasCustomTLS() bool {
	return c.TLSCert != "" && c.TLSKey != ""
}

// HasTLS returns true if either a custom certificate or ACME/Let's Encrypt is used.
func (c *Conf) HasTLS() bool {
	return c.ACMEDomainName != "" || c.HasCustomTLS()
}

// OverrideMode checks the mode parameter for validity and logs that it's being overridden.
func (c *Conf) OverrideMode(mode string) {
	if mode == c.Mode {
		log.WithField("mode", mode).Warning("trying to override run mode with current value; ignoring")
		return
	}
	c.checkMode(mode)
	log.WithFields(log.Fields{
		"configured_mode": c.Mode,
		"current_mode":    mode,
	}).Warning("overriding run mode")
	c.Mode = mode
}

func (c *Conf) checkMode(mode string) {
	// Check mode for validity
	if !validModes[mode] {
		keys := make([]string, 0, len(validModes))
		for k := range validModes {
			keys = append(keys, k)
		}
		log.WithFields(log.Fields{
			"valid_values":  keys,
			"current_value": mode,
		}).Fatal("bad value for 'mode' configuration parameter")
	}
}

func (c *Conf) checkTLS() {
	hasTLS := c.HasCustomTLS()

	if hasTLS && c.ListenHTTPS == "" {
		c.ListenHTTPS = c.Listen
		c.Listen = ""
	}

	if !hasTLS || c.ACMEDomainName == "" {
		return
	}

	log.WithFields(log.Fields{
		"tlscert":          c.TLSCert,
		"tlskey":           c.TLSKey,
		"acme_domain_name": c.ACMEDomainName,
	}).Warning("ACME/Let's Encrypt will not be used because custom certificate is specified")
	c.ACMEDomainName = ""
}

func (c *Conf) parseURLs() {
	var err error

	if c.FlamencoStr == "" {
		c.FlamencoStr = defaultServerURL
	}
	c.Flamenco, err = url.Parse(c.FlamencoStr)
	if err != nil {
		log.WithFields(log.Fields{
			"url":        c.FlamencoStr,
			log.ErrorKey: err,
		}).Error("bad Flamenco URL configured")
		return
	}

	if jwtURL, err := c.Flamenco.Parse(jwtPublicKeysRelativeURL); err != nil {
		log.WithFields(log.Fields{
			"url":        c.Flamenco.String(),
			log.ErrorKey: err,
		}).Error("unable to construct URL to get JWT public keys")
	} else {
		c.JWT.PublicKeysURL = jwtURL.String()
	}
}

// GetTestConfig returns the configuration for unit tests.
func GetTestConfig() Conf {
	_, myFilename, _, _ := runtime.Caller(0)
	myDir := path.Dir(myFilename)

	conf, err := LoadConf(path.Join(myDir, "flamenco-manager.yaml"))
	if err != nil {
		log.Fatalf("Unable to load config: %s", err)
	}

	return conf
}
