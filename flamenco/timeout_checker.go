package flamenco

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
)

// Interval for checking all active tasks and workers for timeouts.
const timeoutCheckInterval = 1 * time.Minute

// Delay for the intial check. This gives workers a chance to reconnect to the Manager
// and send updates after the Manager has started.
const timeoutInitialSleep = 5 * time.Minute

// TimeoutChecker periodically times out tasks and workers if the worker hasn't sent any update recently.
type TimeoutChecker struct {
	closable
	config  *Conf
	session *mgo.Session
	queue   *TaskUpdateQueue
}

// CreateTimeoutChecker creates a new TimeoutChecker.
func CreateTimeoutChecker(config *Conf, session *mgo.Session, queue *TaskUpdateQueue) *TimeoutChecker {
	return &TimeoutChecker{
		makeClosable(),
		config,
		session,
		queue,
	}
}

// Go starts a new goroutine to perform the periodic checking.
func (ttc *TimeoutChecker) Go() {
	ttc.closableAdd(1)
	go func() {
		session := ttc.session.Copy()
		db := session.DB("")
		defer session.Close()
		defer ttc.closableDone()
		defer log.Info("TimeoutChecker: shutting down.")

		// Start with a delay, so that workers get a chance to push their updates
		// after the manager has started up.
		timer := Timer("TimeoutCheck", timeoutCheckInterval, timeoutInitialSleep, &ttc.closable)

		for range timer {
			ttc.checkTasks(db)
			ttc.checkWorkers(db)
		}
	}()
}

// Close gracefully shuts down the task timeout checker goroutine.
func (ttc *TimeoutChecker) Close() {
	log.Debug("TimeoutChecker: Close() called.")
	ttc.closableCloseAndWait()
	log.Debug("TimeoutChecker: shutdown complete.")
}

func (ttc *TimeoutChecker) checkTasks(db *mgo.Database) {
	timeoutThreshold := UtcNow().Add(-ttc.config.ActiveTaskTimeoutInterval)
	log.Debugf("Failing all active tasks that have not been touched since %s", timeoutThreshold)

	var timedoutTasks []Task
	// find all active tasks that either have never been pinged, or were pinged long ago.
	query := M{
		"status": statusActive,
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

func (ttc *TimeoutChecker) timeoutTask(task *Task, db *mgo.Database) {
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
		isManagerLocal: task.isManagerLocalTask(),
		TaskID:         task.ID,
		TaskStatus:     statusFailed,
		Activity:       fmt.Sprintf("Task timed out on worker %s", ident),
		Log: fmt.Sprintf(
			"%s Task %s (%s) timed out, was active but untouched since %s. "+
				"Was handled by worker %s",
			UtcNow().Format(IsoFormat), task.Name, task.ID.Hex(), task.LastWorkerPing, ident),
	}
	ttc.queue.QueueTaskUpdate(task, &tupdate, db)
}

func (ttc *TimeoutChecker) checkWorkers(db *mgo.Database) {
	timeoutThreshold := UtcNow().Add(-ttc.config.ActiveWorkerTimeoutInterval)
	log.Debugf("Failing all awake workers that have not been seen since %s", timeoutThreshold)

	var timedoutWorkers []Worker
	// find all awake workers that either have never been seen, or were seen long ago.
	query := M{
		"status": workerStatusAwake,
		"$or": []M{
			M{"last_activity": M{"$lte": timeoutThreshold}},
			M{"last_activity": M{"$exists": false}},
		},
	}
	projection := M{
		"_id":      1,
		"nickname": 1,
		"address":  1,
		"status":   1,
	}
	if err := db.C("flamenco_workers").Find(query).Select(projection).All(&timedoutWorkers); err != nil {
		log.Warningf("Error finding timed-out workers: %s", err)
	}

	for _, worker := range timedoutWorkers {
		worker.Timeout(db)
	}
}
