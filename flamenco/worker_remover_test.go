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
	"github.com/stretchr/testify/assert"
	check "gopkg.in/check.v1"
	httpmock "gopkg.in/jarcoal/httpmock.v1"
	mgo "gopkg.in/mgo.v2"
)

type WorkerRemoverTestSuite struct {
	workerLnx Worker
	workerWin Worker

	session *mgo.Session
	db      *mgo.Database
	sched   *TaskScheduler
	wr      *WorkerRemover
}

var _ = check.Suite(&WorkerRemoverTestSuite{})

func (s *WorkerRemoverTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()

	config := GetTestConfig()
	config.WorkerCleanupMaxAge = 30 * time.Second

	s.session = MongoSession(&config)
	s.db = s.session.DB("")

	upstream := ConnectUpstream(&config, s.session)
	blacklist := CreateWorkerBlackList(&config, s.session)
	queue := CreateTaskUpdateQueue(&config, blacklist)

	pusher := CreateTaskUpdatePusher(&config, upstream, s.session, queue, nil)
	s.sched = CreateTaskScheduler(&config, upstream, s.session, queue, blacklist, pusher)
	s.wr = CreateWorkerRemover(&config, s.session, s.sched)

	// Store workers in DB, on purpose in the opposite order as the tasks.
	s.workerLnx = Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"sleeping"},
		Nickname:           "workerLnx",
		Status:             workerStatusAwake,
	}
	if err := StoreNewWorker(&s.workerLnx, s.db); err != nil {
		c.Fatal("Unable to insert test workerLnx", err)
	}
	s.workerWin = Worker{
		Platform:           "windows",
		SupportedTaskTypes: []string{"testing"},
		Nickname:           "workerWin",
	}
	if err := StoreNewWorker(&s.workerWin, s.db); err != nil {
		c.Fatal("Unable to insert test workerWin", err)
	}
}

func (s *WorkerRemoverTestSuite) TearDownTest(c *check.C) {
	log.Info("WorkerRemoverTestSuite tearing down test, dropping database.")
	s.db.DropDatabase()
	httpmock.DeactivateAndReset()
}

func (s *WorkerRemoverTestSuite) TestZeroMaxAge(t *check.C) {
	config := GetTestConfig()
	assert.Equal(t, 0*time.Second, config.WorkerCleanupMaxAge)
	wr := CreateWorkerRemover(&config, s.session, nil)
	assert.Nil(t, wr)
}

func (s *WorkerRemoverTestSuite) TestAllWorkersFresh(t *check.C) {
	s.wr.cleanupWorkers(s.db)
	AssertWorkerExists(t, s.workerLnx.ID, s.db)
	AssertWorkerExists(t, s.workerWin.ID, s.db)
}

func (s *WorkerRemoverTestSuite) TestRemoveWorker(t *check.C) {
	beforeThreshold := time.Now().UTC().Add(-24 * time.Hour)
	coll := s.db.C("flamenco_workers")
	err := coll.UpdateId(s.workerLnx.ID, M{
		"$set": M{"last_activity": beforeThreshold},
	})
	assert.Nil(t, err)

	// Worker isn't offine, so it shouldn't be deleted.
	s.wr.cleanupWorkers(s.db)
	AssertWorkerExists(t, s.workerLnx.ID, s.db)
	AssertWorkerExists(t, s.workerWin.ID, s.db)

	// Should be deleted now.
	s.workerLnx.SetStatus(workerStatusOffline, s.db)
	s.wr.cleanupWorkers(s.db)
	AssertWorkerNotExists(t, s.workerLnx.ID, s.db)
	AssertWorkerExists(t, s.workerWin.ID, s.db)
}

func (s *WorkerRemoverTestSuite) TestRemoveTimeoutWorker(t *check.C) {
	beforeThreshold := time.Now().UTC().Add(-24 * time.Hour)
	coll := s.db.C("flamenco_workers")
	err := coll.UpdateId(s.workerLnx.ID, M{
		"$set": M{"last_activity": beforeThreshold},
	})
	assert.Nil(t, err)

	// By default this status shouldn't be cleaned up.
	s.workerLnx.SetStatus(workerStatusTimeout, s.db)
	s.wr.cleanupWorkers(s.db)
	AssertWorkerExists(t, s.workerLnx.ID, s.db)
	AssertWorkerExists(t, s.workerWin.ID, s.db)

	// Should be deleted now.
	s.wr.config.WorkerCleanupStatus = []string{workerStatusOffline, workerStatusTimeout}
	s.wr.cleanupWorkers(s.db)
	AssertWorkerNotExists(t, s.workerLnx.ID, s.db)
	AssertWorkerExists(t, s.workerWin.ID, s.db)
}

func (s *WorkerRemoverTestSuite) TestRequeueTasks(t *check.C) {
	beforeThreshold := time.Now().UTC().Add(-24 * time.Hour)
	workersColl := s.db.C("flamenco_workers")
	tasksColl := s.db.C("flamenco_tasks")

	// Create a task and assign it to the worker.
	task1 := ConstructTestTask("1aaaaaaaaaaaaaaaaaaaaaaa", "testing")
	assert.Nil(t, tasksColl.Insert(&task1))
	assert.Nil(t, s.sched.assignTaskToWorker(&task1, &s.workerLnx, s.db, log.WithField("testing", "testing")))

	// Fake a non-responsive worker.
	err := workersColl.UpdateId(s.workerLnx.ID, M{
		"$set": M{
			"last_activity": beforeThreshold,
			"status":        workerStatusTimeout,
		},
	})
	assert.Nil(t, err)

	// The task should have been requeued.
	s.wr.config.WorkerCleanupStatus = []string{workerStatusOffline, workerStatusTimeout}
	s.wr.cleanupWorkers(s.db)
	AssertWorkerNotExists(t, s.workerLnx.ID, s.db)

	found := Task{}
	err = tasksColl.FindId(task1.ID).One(&found)
	assert.Nil(t, err)
	assert.Equal(t, statusClaimedByManager, found.Status)
	assert.Equal(t, s.workerLnx.ID, *found.WorkerID)
	assert.Contains(t, found.Activity, workerCleanupTaskRequeueReason)
}
