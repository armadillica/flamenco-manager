package flamenco

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"
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

	// Mock that the task with highest priority was actually canceled on the Server.
	httpmock.RegisterResponder(
		"POST",
		"http://localhost:51234/api/flamenco/managers/5852bc5198377351f95d103e/task-update-batch",
		func(req *http.Request) (*http.Response, error) {
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
	tup.Go()
	// Give the tup.Go() coroutine (and subsequent calls) time to run
	// and actually start running the pusher timer.
	time.Sleep(100 * time.Millisecond)

	tupDone := make(chan bool)
	go func() {
		tup.Close()
		tupDone <- true
	}()

	select {
	case <-tupDone:
		break
	case <-time.After(1 * time.Second):
		assert.FailNow(t, "Go() call took too much time")
	}
	assert.Equal(t, 1, httpmock.GetTotalCallCount(), "HTTP POST to Flamenco Server not performed")

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
	assert.Equal(c, *task1.WorkerID, worker1.ID)
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
	assert.Equal(c, *task1.WorkerID, worker1.ID)
	assert.Equal(c, task1.Activity, "doing stuff by worker1")
}

func (s *TaskUpdatesTestSuite) TestUpdateForCancelRequestedTask(c *check.C) {
	tasksColl := s.db.C("flamenco_tasks")

	worker1 := Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"testing"},
	}
	assert.Nil(c, StoreNewWorker(&worker1, s.db))

	testForStatus := func(taskID string, taskStatus string, lastActivity string) {
		task := ConstructTestTask(taskID, "testing")
		task.WorkerID = &worker1.ID
		task.Worker = worker1.Nickname
		task.Status = taskStatus
		task.Activity = lastActivity
		assert.Nil(c, tasksColl.Insert(task))

		tupdate := TaskUpdate{
			TaskID:     task.ID,
			TaskStatus: "active",
			Activity:   "doing stuff by worker1",
		}
		payload, err := json.Marshal(tupdate)
		assert.Nil(c, err)
		respRec, ar := WorkerTestRequestWithBody(worker1.ID, bytes.NewBuffer(payload), "POST", "/tasks/"+taskID+"/update")
		QueueTaskUpdateFromWorker(respRec, ar, s.db, task.ID)

		// This update should be accepted, but not change the task's status.
		assert.Equal(c, http.StatusNoContent, respRec.Code)

		assert.Nil(c, tasksColl.FindId(task.ID).One(&task))
		assert.Equal(c, worker1.ID, *task.WorkerID)
		assert.Equal(c, taskStatus, task.Status, lastActivity)
		assert.Equal(c, lastActivity, task.Activity)
	}
	testForStatus("1aaaaaaaaaaaaaaaaaaaaaaa", "cancel-requested", "Cancel requested by unittest")
	testForStatus("2aaaaaaaaaaaaaaaaaaaaaaa", "failed", "Failure forced by unittest")
}

func (s *TaskUpdatesTestSuite) TestTaskRescheduling(c *check.C) {
	tasksColl := s.db.C("flamenco_tasks")

	task1 := ConstructTestTask("1aaaaaaaaaaaaaaaaaaaaaaa", "testing")
	assert.Nil(c, tasksColl.Insert(task1))

	worker1 := Worker{
		Platform:           "linux",
		Nickname:           "worker1",
		SupportedTaskTypes: []string{"testing"},
	}
	worker2 := Worker{
		Platform:           "linux",
		Nickname:           "worker2",
		SupportedTaskTypes: []string{"testing"},
	}
	assert.Nil(c, StoreNewWorker(&worker1, s.db))
	assert.Nil(c, StoreNewWorker(&worker2, s.db))

	taskSched := CreateTaskScheduler(&s.config, s.upstream, s.session)

	tupdate := TaskUpdate{
		TaskID:     task1.ID,
		TaskStatus: "active",
		Activity:   "doing stuff by worker1",
	}
	payloadBytes, err := json.Marshal(tupdate)
	assert.Nil(c, err)
	respRec, ar := WorkerTestRequestWithBody(worker1.ID, bytes.NewBuffer(payloadBytes), "POST", "/tasks/1aaaaaaaaaaaaaaaaaaaaaaa/update")
	QueueTaskUpdateFromWorker(respRec, ar, s.db, task1.ID)
	assert.Equal(c, http.StatusNoContent, respRec.Code)

	// Because of this update, the task should be assigned to worker 1.
	assert.Nil(c, tasksColl.FindId(task1.ID).One(&task1))
	assert.Equal(c, worker1.ID, *task1.WorkerID)
	assert.Equal(c, "doing stuff by worker1", task1.Activity)

	// Worker 1 signs off, so task becomes available again for scheduling to worker 2.
	respRec, ar = WorkerTestRequest(worker1.ID, "POST", "/sign-off")
	WorkerSignOff(respRec, ar, s.db)
	respRec, ar = WorkerTestRequest(worker2.ID, "POST", "/task")
	taskSched.ScheduleTask(respRec, ar)
	assert.Equal(c, http.StatusOK, respRec.Code)

	assert.Nil(c, tasksColl.FindId(task1.ID).One(&task1))
	assert.Equal(c, "active", task1.Status)

	// Sleep a bit before we update the task again, so that we can clearly see a difference
	// in update timestamps.
	time.Sleep(250 * time.Millisecond)
	timestampBetweenUpdates := time.Now().UTC()
	time.Sleep(250 * time.Millisecond)

	// An update by worker 2 should be accepted.
	tupdate.Activity = "doing stuff by worker2"
	tupdate.TaskStatus = "failed"
	payloadBytes, err = json.Marshal(tupdate)
	assert.Nil(c, err)
	respRec, ar = WorkerTestRequestWithBody(worker2.ID, bytes.NewBuffer(payloadBytes), "POST", "/tasks/1aaaaaaaaaaaaaaaaaaaaaaa/update")
	QueueTaskUpdateFromWorker(respRec, ar, s.db, task1.ID)
	assert.Equal(c, http.StatusNoContent, respRec.Code)

	assert.Nil(c, tasksColl.FindId(task1.ID).One(&task1))
	assert.Equal(c, *task1.WorkerID, worker2.ID)
	assert.Equal(c, task1.Status, "failed")
	assert.Equal(c, task1.Activity, "doing stuff by worker2")

	// The workers now should have different CurrentTaskUpdated fields.
	workersColl := s.db.C("flamenco_workers")
	assert.Nil(c, workersColl.FindId(worker1.ID).One(&worker1))
	assert.Nil(c, workersColl.FindId(worker2.ID).One(&worker2))

	assert.NotNil(c, worker1.CurrentTaskUpdated)
	assert.NotNil(c, worker2.CurrentTaskUpdated)
	assert.True(c, worker1.CurrentTaskUpdated.Before(timestampBetweenUpdates))
	assert.True(c, worker2.CurrentTaskUpdated.After(timestampBetweenUpdates))
	assert.Equal(c, "active", worker1.CurrentTaskStatus)
	assert.Equal(c, "failed", worker2.CurrentTaskStatus)
}
