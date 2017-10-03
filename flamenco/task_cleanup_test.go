package flamenco

import (
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	check "gopkg.in/check.v1"
	mgo "gopkg.in/mgo.v2"
)

type TaskCleanupTestSuite struct {
	config  *Conf
	session *mgo.Session
	db      *mgo.Database
	sched   *TaskScheduler
}

var _ = check.Suite(&TaskCleanupTestSuite{})

func (s *TaskCleanupTestSuite) SetUpTest(c *check.C) {
	config := GetTestConfig()
	s.config = &config
	s.session = MongoSession(s.config)
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

	notifier := createTaskCleanerEx(s.config, s.session, 0, 1*time.Hour)

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
