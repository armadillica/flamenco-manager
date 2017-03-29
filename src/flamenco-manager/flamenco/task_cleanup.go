package flamenco

import (
	"time"

	log "github.com/Sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
)

// Delay for the intial check. This gives the Manager some time to sync up with the Server
// and workers after a shutdown & restart. Slightly longer than the task timeout check
// initial delay, so that those two don't happen at the same time.
const taskCleanupInitialSleep = 6 * time.Minute

const taskCleanupCheckInterval = 10 * time.Minute

// TaskCleaner periodically deletes tasks that haven't been touched in a long time.
type TaskCleaner struct {
	closable
	config       *Conf
	session      *mgo.Session
	initialSleep time.Duration
	interval     time.Duration
}

// CreateTaskCleaner creates a new TaskCleaner with default timings.
func CreateTaskCleaner(config *Conf, session *mgo.Session) *TaskCleaner {
	return createTaskCleanerEx(config, session, taskCleanupInitialSleep, taskCleanupCheckInterval)
}

func createTaskCleanerEx(config *Conf, session *mgo.Session, initialSleep, interval time.Duration) *TaskCleaner {
	return &TaskCleaner{
		makeClosable(),
		config, session,
		initialSleep, interval,
	}
}

// Go starts a new goroutine to perform the periodic checking.
func (tc *TaskCleaner) Go() {
	tc.closableAdd(1)
	go func() {
		session := tc.session.Copy()
		db := session.DB("")
		defer session.Close()
		defer tc.closableDone()
		defer log.Info("TaskCleaner: shutting down.")

		// Start with a delay, so that workers get a chance to push their updates
		// after the manager has started up.
		timer := Timer("TaskTimeoutCheck", tc.interval, tc.initialSleep, &tc.closable)

		for _ = range timer {
			tc.check(db)
		}
	}()
}

// Close gracefully shuts down the task timeout checker goroutine.
func (tc *TaskCleaner) Close() {
	log.Debug("TaskCleaner: Close() called.")
	tc.closableCloseAndWait()
	log.Debug("TaskCleaner: shutdown complete.")
}

func (tc *TaskCleaner) check(db *mgo.Database) {
	cleanupBefore := UtcNow().Add(-tc.config.TaskCleanupMaxAge)
	log.Debugf("Removing all tasks that have not been touched since %s", cleanupBefore)

	// find all active tasks that either have never been pinged and were
	// sent to us a long time ago, or were pinged long ago.
	query := M{
		"$or": []M{
			M{"last_worker_ping": M{"$lte": cleanupBefore}},
			M{
				"last_worker_ping": M{"$exists": false},
				"last_updated":     M{"$lte": cleanupBefore},
			},
			M{
				"last_worker_ping": M{"$exists": false},
				"last_updated":     M{"$exists": false},
			},
		},
	}

	result, err := db.C("flamenco_tasks").RemoveAll(query)
	if err != nil {
		log.Warningf("Error cleaning up tasks: %s", err)
		return
	}
	log.Infof("Removed %d stale tasks.", result.Removed)
}
