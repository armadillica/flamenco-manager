package flamenco

import (
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"time"

	log "github.com/Sirupsen/logrus"

	"gopkg.in/yaml.v2"
)

// Conf represents the Manager's configuration file.
type Conf struct {
	DatabaseURL   string   `yaml:"database_url"`
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

	TaskUpdatePushMaxInterval time.Duration `yaml:"task_update_push_max_interval"`
	TaskUpdatePushMaxCount    int           `yaml:"task_update_push_max_count"`
	CancelTaskFetchInterval   time.Duration `yaml:"cancel_task_fetch_max_interval"`

	ActiveTaskTimeoutInterval   time.Duration `yaml:"active_task_timeout_interval"`
	ActiveWorkerTimeoutInterval time.Duration `yaml:"active_worker_timeout_interval"`

	TaskCleanupMaxAge time.Duration `yaml:"task_cleanup_max_age"`

	WatchForLatestImage string `yaml:"watch_for_latest_image"`
}

// GetConf parses flamenco-manager.yaml and returns its contents as a Conf object.
func GetConf() Conf {
	yamlFile, err := ioutil.ReadFile("flamenco-manager.yaml")
	if err != nil {
		log.Fatalf("GetConf err   #%v ", err)
	}

	// Construct the struct with some more or less sensible defaults.
	c := Conf{
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
	}
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	// Parse URL
	c.Flamenco, err = url.Parse(c.FlamencoStr)
	if err != nil {
		log.Fatalf("Bad Flamenco URL: %v", err)
	}

	// Transpose the variables matrix.
	c.VariablesByPlatform = make(map[string]map[string]string)
	for varname, perplatform := range c.VariablesByVarname {
		for platform, varvalue := range perplatform {
			if c.VariablesByPlatform[platform] == nil {
				c.VariablesByPlatform[platform] = make(map[string]string)
			}
			c.VariablesByPlatform[platform][varname] = varvalue
		}
	}

	return c
}

// GetTestConfig returns the configuration for unit tests.
func GetTestConfig() Conf {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	if path.Base(cwd) != "flamenco" {
		log.Panic("Expecting tests to run from flamenco package dir.")
		os.Exit(2)
	}

	return GetConf()
}
