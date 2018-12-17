package flamenco

import (
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	check "gopkg.in/check.v1"
)

type BlacklistTestSuite struct {
	wbl *WorkerBlacklist
	db  *mgo.Database

	workerLnx Worker
	workerWin Worker
}

var _ = check.Suite(&BlacklistTestSuite{})

func (s *BlacklistTestSuite) SetUpTest(c *check.C) {
	config := GetTestConfig()
	session := MongoSession(&config)
	s.db = session.DB("")
	s.wbl = CreateWorkerBlackList(&config, session)
	s.wbl.EnsureDBIndices()

	s.workerLnx = Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"sleeping"},
		Nickname:           "workerLnx",
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

func (s *BlacklistTestSuite) TearDownTest(c *check.C) {
	log.Info("BlacklistTest tearing down test, dropping database.")
	s.db.DropDatabase()
}

func (s *BlacklistTestSuite) TestNonExistingWorker(c *check.C) {
	blacklist := s.wbl.BlacklistForWorker(bson.NewObjectId())
	assert.Equal(c, M{}, blacklist)
}

func (s *BlacklistTestSuite) TestEmpty(c *check.C) {
	blacklist := s.wbl.BlacklistForWorker(s.workerLnx.ID)
	assert.Equal(c, M{}, blacklist)
}

func (s *BlacklistTestSuite) TestBlacklist(c *check.C) {
	// Insert a number of tasks of different type & job.
	job1 := bson.NewObjectId()
	job2 := bson.NewObjectId()

	createTask := func(jobID bson.ObjectId, taskType string) *Task {
		task := ConstructTestTask(bson.NewObjectId().Hex(), taskType)
		task.Job = jobID
		if err := s.db.C("flamenco_tasks").Insert(task); err != nil {
			c.Fatal("Unable to insert test task", err)
		}
		return &task
	}

	task1fm := createTask(job1, "file-management")
	task1br := createTask(job1, "blender-render")
	task1ve := createTask(job1, "video-encoding")

	createTask(job2, "file-management")
	createTask(job2, "blender-render")
	task2ve := createTask(job2, "video-encoding")

	assert.Nil(c, s.wbl.Add(s.workerLnx.ID, task1fm))
	assert.Nil(c, s.wbl.Add(s.workerLnx.ID, task1br))
	assert.Nil(c, s.wbl.Add(s.workerLnx.ID, task2ve))
	assert.Nil(c, s.wbl.Add(s.workerWin.ID, task1ve))

	blacklist := s.wbl.BlacklistForWorker(s.workerLnx.ID)
	// Note that this is a rather sensitive test, as I'm not sure whether the ordering is
	// stable across different machines. It seems stable on mine, though.
	expect := M{"$nor": []M{
		M{"job": job2, "task_type": M{"$in": []string{"video-encoding"}}},
		M{"job": job1, "task_type": M{"$in": []string{"file-management", "blender-render"}}},
	}}
	assert.Equal(c, expect, blacklist)

	blacklist = s.wbl.BlacklistForWorker(s.workerWin.ID)
	expect = M{"$nor": []M{
		M{"job": job1, "task_type": M{"$in": []string{"video-encoding"}}},
	}}
	assert.Equal(c, expect, blacklist)
}
