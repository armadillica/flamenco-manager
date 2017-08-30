package flamenco

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"gopkg.in/yaml.v2"
)

// Conf represents the Manager's configuration file.
type Conf struct {
	DatabaseURL   string   `yaml:"database_url"`
	DatabasePath  string   `yaml:"database_path"`
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

	WatchForLatestImage string `yaml:"watch_for_latest_image"`

	SSDPDiscovery  bool   `yaml:"ssdp_discovery"`
	SSDPDeviceUUID string `yaml:"ssdp_device_uuid"`
}

// GetConf parses flamenco-manager.yaml and returns its contents as a Conf object.
func GetConf() (Conf, error) {
	return LoadConf("flamenco-manager.yaml")
}

// LoadConf parses the given file and returns its contents as a Conf object.
func LoadConf(filename string) (Conf, error) {
	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		return Conf{}, err
	}

	// Construct the struct with some more or less sensible defaults.
	c := Conf{
		DatabasePath:                "./db",
		DownloadTaskSleep:           300 * time.Second,
		DownloadTaskRecheckThrottle: 10 * time.Second,
		TaskUpdatePushMaxInterval:   30 * time.Second,
		TaskUpdatePushMaxCount:      10,
		CancelTaskFetchInterval:     10 * time.Second,
		ActiveTaskTimeoutInterval:   1 * time.Minute,
		ActiveWorkerTimeoutInterval: 15 * time.Minute,
		// Days are assumed to be 24 hours long. This is not exactly accurate, but should
		// be accurate enough for this type of cleanup.
		TaskCleanupMaxAge: 14 * 24 * time.Hour,
		SSDPDiscovery:     true,
		SSDPDeviceUUID:    "7401c189-ef69-434b-b4d8-56d00075faf5",
	}
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		return c, fmt.Errorf("unmarshal: %v", err)
	}

	// Parse URL
	c.Flamenco, err = url.Parse(c.FlamencoStr)
	if err != nil {
		return c, fmt.Errorf("bad Flamenco URL: %v", err)
	}

	c.checkDatabase()

	foundDuplicate := false
	for varname, perplatform := range c.PathReplacementByVarname {
		// Check variable/path replacement duplicates.
		_, found := c.VariablesByVarname[varname]
		if found {
			log.Errorf("Variable '%s' defined as both regular and path replacement variable", varname)
			foundDuplicate = true
		}

		// Remove trailing slashes from replacement paths, since there should be a slash after
		// each path replacement variable anyway.
		for platform, value := range perplatform {
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
		return c, fmt.Errorf("duplicate variables found")
	}

	if err := c.Write("written.yaml"); err != nil {
		log.Fatalf("Error writing configuration file: %s", err)
	}

	return c, nil
}

func (c *Conf) checkDatabase() {
	// At least one of DatabasePath or DatabaseURL must be given.
	if c.DatabasePath == "" && c.DatabaseURL == "" {
		log.Fatal("Configure either database_path or database_url; the cannot both be empty.")
	}

	// If not empty, convert DatabasePath to an absolute path.
	if c.DatabasePath != "" {
		abspath, err := filepath.Abs(c.DatabasePath)
		if err != nil {
			log.Fatalf("Unable to make database path %s absolute: %s", c.DatabasePath, err)
		}
		c.DatabasePath = abspath
	}
}

// Write saves the current in-memory configuration to a YAML file.
func (c *Conf) Write(filename string) error {
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

	log.Infof("Config file written to %s", filename)
	return nil
}

// HasTLS returns true if both the TLS certificate and key files are configured.
func (c *Conf) HasTLS() bool {
	return c.TLSCert != "" && c.TLSKey != ""
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
