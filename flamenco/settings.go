package flamenco

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	yaml "gopkg.in/yaml.v2"
)

const (
	configFilename   = "flamenco-manager.yaml"
	defaultServerURL = "https://cloud.blender.org/"
)

var (
	// ErrDuplicateVariables is returned when the same name is used as regular and path-replacement variable.
	ErrDuplicateVariables = errors.New("duplicate variables found")

	// Valid values for the "mode" config variable.
	validModes = map[string]bool{
		"develop":    true,
		"production": true,
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

// Conf represents the Manager's configuration file.
type Conf struct {
	Mode          string   `yaml:"mode"` // either "develop" or "production"
	DatabaseURL   string   `yaml:"database_url"`
	DatabasePath  string   `yaml:"database_path"`
	TaskLogsPath  string   `yaml:"task_logs_path"`
	Listen        string   `yaml:"listen"`
	OwnURL        string   `yaml:"own_url"`
	FlamencoStr   string   `yaml:"flamenco"`
	Flamenco      *url.URL `yaml:"-"`
	ManagerID     string   `yaml:"manager_id"`
	ManagerSecret string   `yaml:"manager_secret"`
	TLSKey        string   `yaml:"tlskey"`
	TLSCert       string   `yaml:"tlscert"`

	DownloadTaskSleep time.Duration `yaml:"download_task_sleep"`

	/* The number of seconds between rechecks when there are no more tasks for workers.
	 * If set to 0, will not throttle at all.
	 * If set to -1, will never check when a worker asks for a task (so only every
	 * download_task_sleep_seconds seconds). */
	DownloadTaskRecheckThrottle time.Duration `yaml:"download_task_recheck_throttle"`

	/* Variables, stored differently in YAML and these settings.
	 * Variables:             variable name -> platform -> value
	 * VariablesPerPlatform:  platform -> variable name -> value
	 */
	VariablesByVarname  map[string]map[string]string `yaml:"variables"`
	VariablesByPlatform map[string]map[string]string `yaml:"-"`

	PathReplacementByVarname  map[string]map[string]string `yaml:"path_replacement"`
	PathReplacementByPlatform map[string]map[string]string `yaml:"-"`

	TaskUpdatePushMaxInterval time.Duration `yaml:"task_update_push_max_interval"`
	TaskUpdatePushMaxCount    int           `yaml:"task_update_push_max_count"`
	CancelTaskFetchInterval   time.Duration `yaml:"cancel_task_fetch_max_interval"`

	ActiveTaskTimeoutInterval   time.Duration `yaml:"active_task_timeout_interval"`
	ActiveWorkerTimeoutInterval time.Duration `yaml:"active_worker_timeout_interval"`

	TaskCleanupMaxAge time.Duration `yaml:"task_cleanup_max_age"`

	/* This many failures (on a given job+task type combination) will ban a worker
	 * from that task type on that job. */
	BlacklistThreshold int `yaml:"blacklist_threshold"`

	WatchForLatestImage string `yaml:"watch_for_latest_image"`

	SSDPDiscovery  bool   `yaml:"ssdp_discovery"`
	SSDPDeviceUUID string `yaml:"ssdp_device_uuid"`

	TestTasks TestTasks `yaml:"test_tasks"`
}

// GetConf parses flamenco-manager.yaml and returns its contents as a Conf object.
func GetConf() (Conf, error) {
	return LoadConf(configFilename)
}

// LoadConf parses the given file and returns its contents as a Conf object.
func LoadConf(filename string) (Conf, error) {
	yamlFile, err := ioutil.ReadFile(filename)

	// Construct the struct with some more or less sensible defaults.
	c := Conf{
		Mode:                        "production",
		Listen:                      ":8083",
		DatabasePath:                "./db",
		TaskLogsPath:                "./task-logs",
		DownloadTaskSleep:           5 * time.Minute,
		DownloadTaskRecheckThrottle: 10 * time.Second,
		TaskUpdatePushMaxInterval:   30 * time.Second,
		TaskUpdatePushMaxCount:      50,
		CancelTaskFetchInterval:     30 * time.Second,
		ActiveTaskTimeoutInterval:   3 * time.Minute,
		ActiveWorkerTimeoutInterval: 15 * time.Minute,
		FlamencoStr:                 defaultServerURL,
		// Days are assumed to be 24 hours long. This is not exactly accurate, but should
		// be accurate enough for this type of cleanup.
		TaskCleanupMaxAge:  14 * 24 * time.Hour,
		SSDPDiscovery:      true,
		SSDPDeviceUUID:     "7401c189-ef69-434b-b4d8-56d00075faf5",
		BlacklistThreshold: 3,

		VariablesByVarname: map[string]map[string]string{
			"blender": map[string]string{
				"linux":   "/linux/path/to/blender",
				"windows": "C:/windows/path/to/blender.exe",
				"darwin":  "/Volume/Applications/Blender/blender",
			},
		},

		PathReplacementByVarname: map[string]map[string]string{
			"render": map[string]string{
				"linux":   "/render",
				"windows": "R:/",
				"darwin":  "/Volume/render",
			},
			"job_storage": map[string]string{
				"linux":   "/shared/flamenco-jobs",
				"windows": "S:/",
				"darwin":  "/Volume/shared/flamenco-jobs",
			},
		},

		TestTasks: TestTasks{
			BlenderRender: BlenderRenderConfig{
				JobStorage:   "{job_storage}/test-jobs",
				RenderOutput: "{render}/test-renders",
			},
		},
	}
	if err != nil {
		c.processVariables()
		return c, err
	}

	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		c.processVariables()
		return c, fmt.Errorf("unmarshal: %v", err)
	}

	// Parse URL
	if c.FlamencoStr == "" {
		c.FlamencoStr = defaultServerURL
	}
	c.Flamenco, err = url.Parse(c.FlamencoStr)
	if err != nil {
		log.WithFields(log.Fields{
			"url":        c.FlamencoStr,
			log.ErrorKey: err,
		}).Error("bad Flamenco URL configured")
	}

	c.checkMode(c.Mode)
	c.checkDatabase()

	err = c.processVariables()
	if err != nil {
		return c, err
	}
	return c, nil
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
	// Convert back to string representation.
	if c.Flamenco == nil {
		c.FlamencoStr = ""
	} else {
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

// HasTLS returns true if both the TLS certificate and key files are configured.
func (c *Conf) HasTLS() bool {
	return c.TLSCert != "" && c.TLSKey != ""
}

// processVariables takes by-varname and turns it into by-platform, and checks for duplicates
// between regular and path-replacement variables.
func (c *Conf) processVariables() error {
	foundDuplicate := false
	for varname, perplatform := range c.PathReplacementByVarname {
		// Check variable/path replacement duplicates.
		_, found := c.VariablesByVarname[varname]
		if found {
			log.WithField("variable", varname).Error("Variable defined as both regular and path replacement variable")
			foundDuplicate = true
		}

		// Remove trailing slashes from replacement paths, since there should be a slash after
		// each path replacement variable anyway.
		for platform, value := range perplatform {
			if strings.Contains(value, "\\") {
				log.WithFields(log.Fields{
					"variable": varname,
					"platform": platform,
					"value":    value,
				}).Warning("Backslash found in path replacement variable. Change those to forward slashes instead.")
			}
			perplatform[platform] = strings.TrimRight(value, "/")
		}
	}

	transposeVariableMatrix(&c.VariablesByVarname, &c.VariablesByPlatform)
	transposeVariableMatrix(&c.PathReplacementByVarname, &c.PathReplacementByPlatform)

	for platform, vars := range c.VariablesByPlatform {
		log.Debugf("Variables for '%s'", platform)
		for name, value := range vars {
			log.Debugf("     %15s = %s", name, value)
		}
	}

	for platform, vars := range c.PathReplacementByPlatform {
		log.Debugf("Paths for '%s'", platform)
		for name, value := range vars {
			log.Debugf("     %15s = %s", name, value)
		}
	}

	if foundDuplicate {
		return ErrDuplicateVariables
	}
	return nil
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

func transposeVariableMatrix(in, out *map[string]map[string]string) {
	*out = make(map[string]map[string]string)
	for varname, perplatform := range *in {
		for platform, varvalue := range perplatform {
			if (*out)[platform] == nil {
				(*out)[platform] = make(map[string]string)
			}
			(*out)[platform][varname] = varvalue
		}
	}
}

// GetTestConfig returns the configuration for unit tests.
func GetTestConfig() Conf {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if filepath.Base(cwd) != "flamenco" {
		log.Fatalf("Expecting tests to run from flamenco package dir, not from %v", cwd)
	}

	conf, err := GetConf()
	if err != nil {
		log.Fatalf("Unable to load config: %s", err)
	}

	return conf
}
