package flamenco

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/stretchr/testify/assert"

	log "github.com/Sirupsen/logrus"
	check "gopkg.in/check.v1"
	"gopkg.in/jarcoal/httpmock.v1"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type TaskUpdatesTestSuite struct {
	config   Conf
	session  *mgo.Session
	db       *mgo.Database
	upstream *UpstreamConnection
}

var _ = check.Suite(&TaskUpdatesTestSuite{})

func (s *TaskUpdatesTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()

	s.config = GetTestConfig()
	s.session = MongoSession(&s.config)
	s.db = s.session.DB("")
	s.upstream = ConnectUpstream(&s.config, s.session)
}

func (s *TaskUpdatesTestSuite) TearDownTest(c *check.C) {
	log.Info("SchedulerTestSuite tearing down test, dropping database.")
	s.upstream.Close()
	s.db.DropDatabase()
	httpmock.DeactivateAndReset()
}

func (s *TaskUpdatesTestSuite) TestCancelRunningTasks(t *check.C) {
	tasksColl := s.db.C("flamenco_tasks")

	task1 := ConstructTestTask("1aaaaaaaaaaaaaaaaaaaaaaa", "testing")
	if err := tasksColl.Insert(task1); err != nil {
		t.Fatal("Unable to insert test task", err)
	}
	task2 := ConstructTestTask("2aaaaaaaaaaaaaaaaaaaaaaa", "sleeping")
	if err := tasksColl.Insert(task2); err != nil {
		t.Fatal("Unable to insert test task 2", err)
	}

	timeout := TimeoutAfter(1 * time.Second)
	defer close(timeout)

	// Mock that the task with highest priority was actually canceled on the Server.
	httpmock.RegisterResponder(
		"POST",
		"http://localhost:51234/api/flamenco/managers/5852bc5198377351f95d103e/task-update-batch",
		func(req *http.Request) (*http.Response, error) {
			defer func() { timeout <- false }()
			log.Info("POST from manager received on server, sending back TaskUpdateResponse.")

			resp := TaskUpdateResponse{
				CancelTasksIds: []bson.ObjectId{task2.ID},
			}
			return httpmock.NewJsonResponse(200, &resp)
		},
	)

	// Set up some decent timeouts so we don't have to wait forevah.
	s.config.TaskUpdatePushMaxInterval = 30 * time.Second
	s.config.TaskUpdatePushMaxCount = 4000
	s.config.CancelTaskFetchInterval = 300 * time.Millisecond

	tup := CreateTaskUpdatePusher(&s.config, s.upstream, s.session)
	defer tup.Close()

	go tup.Go()

	timedout := <-timeout
	assert.False(t, timedout, "HTTP POST to Flamenco Server not performed")

	// Give the tup.Go() coroutine (and subsequent calls) time to run.
	// the "timeout <- false" call in the responder is triggered before
	// that function is done working.
	time.Sleep(100 * time.Millisecond)

	// Check that one task was canceled and the other was not.
	taskDb := Task{}
	assert.Nil(t, tasksColl.FindId(task1.ID).One(&taskDb))
	assert.Equal(t, "queued", taskDb.Status)
	assert.Nil(t, tasksColl.FindId(task2.ID).One(&taskDb))
	assert.Equal(t, "canceled", taskDb.Status)
}

func (s *TaskUpdatesTestSuite) TestMultipleWorkersForOneTask(c *check.C) {
	tasksColl := s.db.C("flamenco_tasks")

	task1 := ConstructTestTask("1aaaaaaaaaaaaaaaaaaaaaaa", "testing")
	assert.Nil(c, tasksColl.Insert(task1))

	worker1 := Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"testing"},
	}
	worker2 := Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"testing"},
	}
	assert.Nil(c, StoreNewWorker(&worker1, s.db))
	assert.Nil(c, StoreNewWorker(&worker2, s.db))

	// Task should not be assigned to any worker
	assert.Nil(c, task1.WorkerID)

	tupdate := TaskUpdate{
		TaskID:   task1.ID,
		Activity: "doing stuff by worker1",
	}
	payloadBytes, err := json.Marshal(tupdate)
	assert.Nil(c, err)
	respRec, ar := WorkerTestRequestWithBody(worker1.ID, bytes.NewBuffer(payloadBytes), "POST", "/tasks/1aaaaaaaaaaaaaaaaaaaaaaa/update")
	QueueTaskUpdateFromWorker(respRec, ar, s.db, task1.ID)
	assert.Equal(c, 204, respRec.Code)

	// Because of this update, the task should be assigned to worker 1
	assert.Nil(c, tasksColl.FindId(task1.ID).One(&task1))
	assert.Equal(c, task1.WorkerID, task1.WorkerID)
	assert.Equal(c, task1.Activity, "doing stuff by worker1")

	// An update by worker 2 should fail.
	tupdate.Activity = "doing stuff by worker2"
	payloadBytes, err = json.Marshal(tupdate)
	assert.Nil(c, err)
	respRec, ar = WorkerTestRequestWithBody(worker2.ID, bytes.NewBuffer(payloadBytes), "POST", "/tasks/1aaaaaaaaaaaaaaaaaaaaaaa/update")
	QueueTaskUpdateFromWorker(respRec, ar, s.db, task1.ID)
	assert.Equal(c, http.StatusConflict, respRec.Code)

	// The task should still be assigned to worker 1
	assert.Nil(c, tasksColl.FindId(task1.ID).One(&task1))
	assert.Equal(c, task1.WorkerID, task1.WorkerID)
	assert.Equal(c, task1.Activity, "doing stuff by worker1")
}

func (s *TaskUpdatesTestSuite) TestUpdateForCancelRequestedTask(c *check.C) {
	tasksColl := s.db.C("flamenco_tasks")

	worker1 := Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"testing"},
	}
	assert.Nil(c, StoreNewWorker(&worker1, s.db))

	task1 := ConstructTestTask("1aaaaaaaaaaaaaaaaaaaaaaa", "testing")
	task1.WorkerID = &worker1.ID
	task1.Worker = worker1.Nickname
	task1.Status = "cancel-requested"
	task1.Activity = "Cancel requested by unittest"
	assert.Nil(c, tasksColl.Insert(task1))

	tupdate := TaskUpdate{
		TaskID:     task1.ID,
		TaskStatus: "active",
		Activity:   "doing stuff by worker1",
	}
	payload, err := json.Marshal(tupdate)
	assert.Nil(c, err)
	respRec, ar := WorkerTestRequestWithBody(worker1.ID, bytes.NewBuffer(payload), "POST", "/tasks/1aaaaaaaaaaaaaaaaaaaaaaa/update")
	QueueTaskUpdateFromWorker(respRec, ar, s.db, task1.ID)

	// This update should be rejected, and not change the task's status.
	assert.Equal(c, http.StatusConflict, respRec.Code)

	assert.Nil(c, tasksColl.FindId(task1.ID).One(&task1))
	assert.Equal(c, task1.WorkerID, task1.WorkerID)
	assert.Equal(c, task1.Status, "cancel-requested")
	assert.Equal(c, task1.Activity, "Cancel requested by unittest")
}
