package flamenco

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
	wr      *WorkerRemover
}

var _ = check.Suite(&WorkerRemoverTestSuite{})

func (s *WorkerRemoverTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()

	config := GetTestConfig()
	config.WorkerCleanupMaxAge = 30 * time.Second

	s.session = MongoSession(&config)
	s.db = s.session.DB("")
	s.wr = CreateWorkerRemover(&config, s.session)

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
	wr := CreateWorkerRemover(&config, s.session)
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
