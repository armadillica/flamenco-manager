package flamenco

import (
	"time"

	"gopkg.in/mgo.v2/bson"
)

// Some of the statuses we recognise.
// TODO Sybren: list all used statuses here as constants, instead of using string literals.
const (
	statusQueued           = "queued"
	statusClaimedByManager = "claimed-by-manager"
	statusActive           = "active"
	statusCanceled         = "canceled"
	statusFailed           = "failed"
	statusCompleted        = "completed"
	statusCancelRequested  = "cancel-requested"
)

// Possible worker statuses.
const (
	workerStatusStarting = "starting" // signed on but not done anything yet
	workerStatusOffline  = "offline"
	workerStatusShutdown = "shutdown" // will go offline soon
	workerStatusAwake    = "awake"
	workerStatusTesting  = "testing"
	workerStatusTimeout  = "timeout"
	workerStatusAsleep   = "asleep" // listens to a wakeup call, but performs no tasks.
)

// Command is an executable part of a Task
type Command struct {
	Name     string `bson:"name" json:"name"`
	Settings bson.M `bson:"settings" json:"settings"`
}

// Task contains a Flamenco task, with some BSON-only fields for local Manager use.
type Task struct {
	ID          bson.ObjectId   `bson:"_id,omitempty" json:"_id,omitempty"`
	Etag        string          `bson:"_etag,omitempty" json:"_etag,omitempty"`
	Job         bson.ObjectId   `bson:"job,omitempty" json:"job"`
	Manager     bson.ObjectId   `bson:"manager,omitempty" json:"manager"`
	Project     bson.ObjectId   `bson:"project,omitempty" json:"project"`
	User        bson.ObjectId   `bson:"user,omitempty" json:"user"`
	Name        string          `bson:"name" json:"name"`
	Status      string          `bson:"status" json:"status"`
	Priority    int             `bson:"priority" json:"priority"`
	JobPriority int             `bson:"job_priority" json:"job_priority"`
	JobType     string          `bson:"job_type" json:"job_type"`
	TaskType    string          `bson:"task_type" json:"task_type"`
	Commands    []Command       `bson:"commands" json:"commands"`
	Log         string          `bson:"log,omitempty" json:"log,omitempty"`
	Activity    string          `bson:"activity,omitempty" json:"activity,omitempty"`
	Parents     []bson.ObjectId `bson:"parents,omitempty" json:"parents,omitempty"`
	Worker      string          `bson:"worker,omitempty" json:"worker,omitempty"`

	// Internal bookkeeping
	WorkerID       *bson.ObjectId `bson:"worker_id,omitempty" json:"-"`        // The worker assigned to this task.
	LastWorkerPing *time.Time     `bson:"last_worker_ping,omitempty" json:"-"` // When a worker last said it was working on this. Might not have been a task update.
	LastUpdated    *time.Time     `bson:"last_updated,omitempty" json:"-"`     // when we have last seen an update.
}

type aggregationPipelineResult struct {
	// For internal MongoDB querying only
	Task *Task `bson:"task"`
}

// ScheduledTasks contains a dependency graph response from Server.
type ScheduledTasks struct {
	Depsgraph []Task `json:"depsgraph"`
}

// TaskUpdate is both sent from Worker to Manager, as well as from Manager to Server.
type TaskUpdate struct {
	ID                        bson.ObjectId `bson:"_id" json:"_id"`
	TaskID                    bson.ObjectId `bson:"task_id" json:"task_id,omitempty"`
	TaskStatus                string        `bson:"task_status,omitempty" json:"task_status,omitempty"`
	ReceivedOnManager         time.Time     `bson:"received_on_manager" json:"received_on_manager"`
	Activity                  string        `bson:"activity,omitempty" json:"activity,omitempty"`
	TaskProgressPercentage    int           `bson:"task_progress_percentage" json:"task_progress_percentage"`
	CurrentCommandIdx         int           `bson:"current_command_idx" json:"current_command_idx"`
	CommandProgressPercentage int           `bson:"command_progress_percentage" json:"command_progress_percentage"`
	Log                       string        `bson:"log,omitempty" json:"log,omitempty"`
	Worker                    string        `bson:"worker" json:"worker"`

	isManagerLocal bool // when true, this update should only be applied locally and not be sent upstream.
}

// TaskUpdateResponse is received from Server.
type TaskUpdateResponse struct {
	ModifiedCount    int             `json:"modified_count"`
	HandledUpdateIds []bson.ObjectId `json:"handled_update_ids,omitempty"`
	CancelTasksIds   []bson.ObjectId `json:"cancel_task_ids,omitempty"`
}

// WorkerRegistration is sent by the Worker to register itself at this Manager.
type WorkerRegistration struct {
	Secret             string   `json:"secret"`
	Platform           string   `json:"platform"`
	SupportedTaskTypes []string `json:"supported_task_types"`
	Nickname           string   `json:"nickname"`
}

// WorkerSignonDoc is sent by the Worker upon sign-on.
type WorkerSignonDoc struct {
	SupportedTaskTypes []string `json:"supported_task_types,omitempty"`
	Nickname           string   `json:"nickname,omitempty"`
}

// WorkerStatus indicates that a status change was requested on the worker.
// It is sent as response by the scheduler when a worker is not allowed to receive a new task.
type WorkerStatus struct {
	// For controlling sleeping & waking up. For values, see the workerStatusXXX constants.
	StatusRequested string `bson:"status_requested" json:"status_requested"`
}

// Worker contains all information about a specific Worker.
// Some fields come from the WorkerRegistration, whereas others are filled by us.
type Worker struct {
	ID                 bson.ObjectId  `bson:"_id,omitempty" json:"_id,omitempty"`
	Secret             string         `bson:"-" json:"-"`
	HashedSecret       []byte         `bson:"hashed_secret" json:"-"`
	Nickname           string         `bson:"nickname" json:"nickname"`
	Address            string         `bson:"address" json:"address"`
	Status             string         `bson:"status" json:"status"`
	Platform           string         `bson:"platform" json:"platform"`
	CurrentTask        *bson.ObjectId `bson:"current_task,omitempty" json:"current_task,omitempty"`
	TimeCost           int            `bson:"time_cost" json:"time_cost"`
	LastActivity       *time.Time     `bson:"last_activity,omitempty" json:"last_activity,omitempty"`
	SupportedTaskTypes []string       `bson:"supported_task_types" json:"supported_task_types"`
	Software           string         `bson:"software" json:"software"`

	// For dashboard
	CurrentTaskStatus  string     `bson:"current_task_status,omitempty" json:"current_task_status,omitempty"`
	CurrentTaskUpdated *time.Time `bson:"current_task_updated,omitempty" json:"current_task_updated,omitempty"`

	// For controlling sleeping & waking up. For values, see the workerStatusXXX constants.
	StatusRequested string `bson:"status_requested" json:"status_requested"`
}

// StartupNotification sent to upstream Flamenco Server upon startup. This is a combination
// of settings (see settings.go) and information from the database.
type StartupNotification struct {
	// Settings
	ManagerURL               string                       `json:"manager_url"`
	VariablesByVarname       map[string]map[string]string `json:"variables"`
	PathReplacementByVarname map[string]map[string]string `json:"path_replacement"`

	// From our local database
	NumberOfWorkers int `json:"nr_of_workers"`
}

// MayKeepRunningResponse is sent to workers to indicate whether they can keep running their task.
type MayKeepRunningResponse struct {
	MayKeepRunning bool   `json:"may_keep_running"`
	Reason         string `json:"reason,omitempty"`

	// For controlling sleeping & waking up. For values, see the workerStatusXXX constants.
	StatusRequested string `json:"status_requested,omitempty"`
}

// SettingsInMongo contains settings we want to be able to update from
// within Flamenco Manager itself, so those are stored in MongoDB.
type SettingsInMongo struct {
	DepsgraphLastModified *string `bson:"depsgraph_last_modified"`
}

// StatusReport is sent in response to a query on the / URL.
type StatusReport struct {
	NrOfWorkers       int      `json:"nr_of_workers"`
	NrOfTasks         int      `json:"nr_of_tasks"`
	UpstreamQueueSize int      `json:"upstream_queue_size"`
	Version           string   `json:"version"`
	Workers           []Worker `json:"workers"`
	Server            string   `json:"server"`
}

// FileProduced is sent by the worker whenever it produces (e.g. renders)
// some file. This hooks into the fswatcher system.
type FileProduced struct {
	Paths []string `json:"paths"`
}
