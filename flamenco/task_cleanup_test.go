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
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	check "gopkg.in/check.v1"
	mgo "gopkg.in/mgo.v2"
)

type TaskCleanupTestSuite struct {
	config  Conf
	session *mgo.Session
	db      *mgo.Database
	sched   *TaskScheduler
}

var _ = check.Suite(&TaskCleanupTestSuite{})

func (s *TaskCleanupTestSuite) SetUpSuite(c *check.C) {
	s.config = GetTestConfig()
	s.session = MongoSession(&s.config)
	s.db = s.session.DB("")
}

func (s *TaskCleanupTestSuite) TearDownTest(c *check.C) {
	log.Info("TaskCleanupTestSuite tearing down test, dropping database.")
	s.db.DropDatabase()
}

func (s *TaskCleanupTestSuite) TestTaskCleanup(c *check.C) {
	coll := s.db.C("flamenco_tasks")

	// Obsolete task, never touched by worker and no last_updated field.
	obsoleteTask := ConstructTestTask("aaaaaaaaaaaaaaaaaaaaaaaa", "testing")
	obsoleteTask.LastWorkerPing = nil
	obsoleteTask.LastUpdated = nil
	if err := coll.Insert(obsoleteTask); err != nil {
		c.Fatal("Unable to insert obsoleteTask", err)
	}

	// Stale task, never touched by worker.
	staleTask := ConstructTestTask("baaaaaaaaaaaaaaaaaaaaaaa", "testing")
	staleDate := time.Date(2016, time.March, 29, 13, 38, 5, 0, time.UTC)
	staleTask.LastWorkerPing = nil
	staleTask.LastUpdated = &staleDate
	if err := coll.Insert(staleTask); err != nil {
		c.Fatal("Unable to insert staleTask", err)
	}

	// Very old task, but recently touched by worker.
	oldTask := ConstructTestTask("caaaaaaaaaaaaaaaaaaaaaaa", "testing")
	recentDate := UtcNow().Add(-3 * time.Hour)
	oldTask.LastWorkerPing = &recentDate
	oldTask.LastUpdated = &staleDate
	if err := coll.Insert(oldTask); err != nil {
		c.Fatal("Unable to insert oldTask", err)
	}

	// Recently updated task.
	recentTask := ConstructTestTask("daaaaaaaaaaaaaaaaaaaaaaa", "testing")
	recentTask.LastWorkerPing = &recentDate
	recentTask.LastUpdated = &recentDate
	if err := coll.Insert(recentTask); err != nil {
		c.Fatal("Unable to insert recentTask", err)
	}

	notifier := createTaskCleanerEx(&s.config, s.session, 0, 1*time.Hour)

	// Run one iteration.
	notifier.Go()
	time.Sleep(50 * time.Millisecond)
	notifier.Close()

	// There should be two tasks left in the database.
	var found Task
	if err := coll.FindId(oldTask.ID).One(&found); err != nil {
		c.Fatal("Unable to find oldTask", err)
	}
	if err := coll.FindId(recentTask.ID).One(&found); err != nil {
		c.Fatal("Unable to find oldTask", err)
	}

	count, err := coll.Count()
	if err != nil {
		c.Fatal("Unable to count tasks", err)
	}
	assert.Equal(c, 2, count)
}
