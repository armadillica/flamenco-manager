/**
 * Checks active tasks to see if their worker is still alive & running.
 */
package flamenco

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
)

// Interval for checking all active tasks for timeouts.
const taskTimeoutCheckInterval = 1 * time.Second
const taskTimoutInitialSleep = 5 * time.Minute

// TaskTimeoutChecker periodically times out tasks if the worker hasn't sent any update recently.
type TaskTimeoutChecker struct {
	closable
	config  *Conf
	session *mgo.Session
}

// CreateTaskTimeoutChecker creates a new TaskTimeoutChecker.
func CreateTaskTimeoutChecker(config *Conf, session *mgo.Session) *TaskTimeoutChecker {
	return &TaskTimeoutChecker{
		makeClosable(),
		config, session,
	}
}

// Go starts a new goroutine to perform the periodic checking.
func (ttc *TaskTimeoutChecker) Go() {
	ttc.closableAdd(1)
	go func() {
		session := ttc.session.Copy()
		db := session.DB("")
		defer session.Close()
		defer ttc.closableDone()
		defer log.Info("TaskTimeoutChecker: shutting down.")

		// Start with a delay, so that workers get a chance to push their updates
		// after the manager has started up.
		ok := KillableSleep("TaskTimeoutChecker-initial", taskTimoutInitialSleep, &ttc.closable)
		if !ok {
			log.Info("TaskTimeoutChecker: Killable sleep was killed, not even starting checker.")
			return
		}

		timer := Timer("TaskTimeoutCheck", taskTimeoutCheckInterval, false, &ttc.closable)

		for _ = range timer {
			ttc.check(db)
		}
	}()
}

// Close gracefully shuts down the task timeout checker goroutine.
func (ttc *TaskTimeoutChecker) Close() {
	ttc.closableCloseAndWait()
	log.Debug("TaskTimeoutChecker: shutdown complete.")
}

func (ttc *TaskTimeoutChecker) check(db *mgo.Database) {
	timeoutThreshold := UtcNow().Add(-ttc.config.ActiveTaskTimeoutInterval)
	log.Debugf("Failing all active tasks that have not been touched since %s", timeoutThreshold)

	var timedoutTasks []Task
	// find all active tasks that either have never been pinged, or were pinged long ago.
	query := M{
		"status": "active",
		"$or": []M{
			M{"last_worker_ping": M{"$lte": timeoutThreshold}},
			M{"last_worker_ping": M{"$exists": false}},
		},
	}
	projection := M{
		"_id":              1,
		"last_worker_ping": 1,
		"worker_id":        1,
		"worker":           1,
		"name":             1,
	}
	if err := db.C("flamenco_tasks").Find(query).Select(projection).All(&timedoutTasks); err != nil {
		log.Warningf("Error finding timed-out tasks: %s", err)
	}

	for _, task := range timedoutTasks {
		ttc.timeoutTask(&task, db)
	}
}

func (ttc *TaskTimeoutChecker) timeoutTask(task *Task, db *mgo.Database) {
	log.Warningf("Task %s (%s) timed out", task.Name, task.ID.Hex())
	var ident string

	if task.WorkerID != nil {
		worker, err := FindWorkerByID(*task.WorkerID, db)
		if err != nil {
			log.Errorf("Unable to find worker %v for task %s: %s",
				task.WorkerID.Hex(), task.ID.Hex(), err)
			ident = err.Error()
		} else {
			ident = worker.Identifier()
			worker.TimeoutOnTask(task, db)
		}
	} else if task.Worker != "" {
		ident = task.Worker
	} else {
		ident = "-no worker-"
	}

	tupdate := TaskUpdate{
		TaskID:     task.ID,
		TaskStatus: "failed",
		Activity:   fmt.Sprintf("Task timed out on worker %s", ident),
		Log: fmt.Sprintf(
			"%s Task %s (%s) timed out, was active but untouched since %s. "+
				"Was handled by worker %s",
			UtcNow().Format(IsoFormat), task.Name, task.ID.Hex(), task.LastWorkerPing, ident),
	}
	QueueTaskUpdate(&tupdate, db)
}
