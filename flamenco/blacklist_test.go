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

	job1 bson.ObjectId
	job2 bson.ObjectId

	// 'file-management', 'blender-render', and 'video-encoding' tasks
	task1fm *Task
	task1br *Task
	task1ve *Task
	task2fm *Task
	task2br *Task
	task2ve *Task
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
		SupportedTaskTypes: []string{"sleeping", "testing", "file-management", "blender-render", "video-encoding"},
		Nickname:           "workerLnx",
	}
	if err := StoreNewWorker(&s.workerLnx, s.db); err != nil {
		c.Fatal("Unable to insert test workerLnx", err)
	}
	s.workerWin = Worker{
		Platform:           "windows",
		SupportedTaskTypes: s.workerLnx.SupportedTaskTypes,
		Nickname:           "workerWin",
	}
	if err := StoreNewWorker(&s.workerWin, s.db); err != nil {
		c.Fatal("Unable to insert test workerWin", err)
	}

	// Insert a number of tasks of different type & job.
	s.job1 = bson.NewObjectId()
	s.job2 = bson.NewObjectId()

	createTask := func(jobID bson.ObjectId, taskType string) *Task {
		task := ConstructTestTask(bson.NewObjectId().Hex(), taskType)
		task.Job = jobID
		if err := s.db.C("flamenco_tasks").Insert(task); err != nil {
			c.Fatal("Unable to insert test task", err)
		}
		return &task
	}

	s.task1fm = createTask(s.job1, "file-management")
	s.task1br = createTask(s.job1, "blender-render")
	s.task1ve = createTask(s.job1, "video-encoding")

	s.task2fm = createTask(s.job2, "file-management")
	s.task2br = createTask(s.job2, "blender-render")
	s.task2ve = createTask(s.job2, "video-encoding")
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
	assert.Nil(c, s.wbl.Add(s.workerLnx.ID, s.task1fm))
	assert.Nil(c, s.wbl.Add(s.workerLnx.ID, s.task1br))
	assert.Nil(c, s.wbl.Add(s.workerLnx.ID, s.task2ve))

	assert.Nil(c, s.wbl.Add(s.workerWin.ID, s.task1ve))

	blacklist := s.wbl.BlacklistForWorker(s.workerLnx.ID)
	// Note that this is a rather sensitive test, as I'm not sure whether the ordering is
	// stable across different machines. It seems stable on mine, though.
	expect := M{"$nor": []M{
		M{"job": s.job2, "task_type": M{"$in": []string{"video-encoding"}}},
		M{"job": s.job1, "task_type": M{"$in": []string{"file-management", "blender-render"}}},
	}}
	assert.Equal(c, expect, blacklist)

	blacklist = s.wbl.BlacklistForWorker(s.workerWin.ID)
	expect = M{"$nor": []M{
		M{"job": s.job1, "task_type": M{"$in": []string{"video-encoding"}}},
	}}
	assert.Equal(c, expect, blacklist)
}

func (s *BlacklistTestSuite) TestWorkersLeft(c *check.C) {
	// This worker does not support the 'testing' task type,
	// and thus shouldn't be counted.
	idleWorker := Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"idling"},
		Nickname:           "idler",
	}
	if err := StoreNewWorker(&idleWorker, s.db); err != nil {
		c.Fatal("Unable to insert test idler", err)
	}

	assert.Equal(c, 2, len(s.wbl.WorkersLeft(s.job1, "blender-render")))

	assert.Nil(c, s.wbl.Add(s.workerLnx.ID, s.task1br))
	assert.Equal(c, 1, len(s.wbl.WorkersLeft(s.job1, "blender-render")))

	assert.Nil(c, s.wbl.Add(s.workerWin.ID, s.task1br))
	assert.Equal(c, 0, len(s.wbl.WorkersLeft(s.job1, "blender-render")))

	// Job2 should not be influenced by blacklist of job 1.
	assert.Equal(c, 2, len(s.wbl.WorkersLeft(s.job2, "blender-render")))

	// Different task type on same job should also not be influenced.
	assert.Equal(c, 2, len(s.wbl.WorkersLeft(s.job1, "file-management")))

	// Non-existing job should also work.
	assert.Equal(c, 2, len(s.wbl.WorkersLeft(bson.NewObjectId(), "file-management")))

	// Non-existing task type should also work, but return no workers.
	assert.Equal(c, 0, len(s.wbl.WorkersLeft(s.job1, "je-moeder")))
}
