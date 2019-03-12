package flamenco

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
	"time"

	log "github.com/sirupsen/logrus"
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

		for range timer {
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
	log.WithField("cleanup_before", cleanupBefore).Debug("Removing stale tasks")

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
		log.WithError(err).Warning("Error removing stale tasks")
		return
	}
	if result.Removed > 0 {
		log.WithField("count", result.Removed).Info("Removed stale tasks")
	}
}
