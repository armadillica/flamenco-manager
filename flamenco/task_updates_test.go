package flamenco

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path"
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
	sched    *TaskScheduler
	queue    *TaskUpdateQueue
}

var _ = check.Suite(&TaskUpdatesTestSuite{})

func (s *TaskUpdatesTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()

	s.config = GetTestConfig()

	taskLogsPath, err := ioutil.TempDir("", "testlogs")
	assert.Nil(c, err)
	s.config.TaskLogsPath = taskLogsPath

	s.session = MongoSession(&s.config)
	s.db = s.session.DB("")
	s.upstream = ConnectUpstream(&s.config, s.session)
	s.queue = CreateTaskUpdateQueue(&s.config)
	s.sched = CreateTaskScheduler(&s.config, s.upstream, s.session, s.queue)
}

func (s *TaskUpdatesTestSuite) TearDownTest(c *check.C) {
	log.Info("SchedulerTestSuite tearing down test, dropping database.")
	os.RemoveAll(s.config.TaskLogsPath)

	s.upstream.Close()
	s.db.DropDatabase()
	httpmock.DeactivateAndReset()
}

func (s *TaskUpdatesTestSuite) sendTaskUpdate(c *check.C,
	taskID, workerID bson.ObjectId,
	status, activity, log string) {
	s.sendTaskUpdateWithCode(c, taskID, workerID, status, activity, log, http.StatusNoContent)
}

func (s *TaskUpdatesTestSuite) sendTaskUpdateWithCode(c *check.C,
	taskID, workerID bson.ObjectId,
	status, activity, log string,
	expectedStatusCode int) {

	tupdate := TaskUpdate{
		TaskID:     taskID,
		TaskStatus: status,
		Activity:   activity,
		Log:        log,
	}
	payloadBytes, err := json.Marshal(tupdate)
	assert.Nil(c, err)
	respRec, ar := WorkerTestRequestWithBody(workerID, bytes.NewBuffer(payloadBytes), "POST", "/tasks/"+taskID.Hex()+"/update")
	s.queue.QueueTaskUpdateFromWorker(respRec, ar, s.db, taskID)
	assert.Equal(c, expectedStatusCode, respRec.Code)
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

	tup := CreateTaskUpdatePusher(&s.config, s.upstream, s.session, s.queue)
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

	s.sendTaskUpdate(c, task1.ID, worker1.ID, "", "doing stuff by worker1", "")

	// Because of this update, the task should be assigned to worker 1
	assert.Nil(c, tasksColl.FindId(task1.ID).One(&task1))
	assert.Equal(c, *task1.WorkerID, worker1.ID)
	assert.Equal(c, task1.Activity, "doing stuff by worker1")

	// An update by worker 2 should fail.
	s.sendTaskUpdateWithCode(c, task1.ID, worker2.ID, "", "doing stuff by worker2", "", http.StatusConflict)

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

		// This update should be accepted, but not change the task's status.
		s.sendTaskUpdate(c, task.ID, worker1.ID, statusActive, "doing stuff by worker1", "")

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

	taskSched := CreateTaskScheduler(&s.config, s.upstream, s.session, s.queue)

	// Because of this update, the task should be assigned to worker 1.
	s.sendTaskUpdate(c, task1.ID, worker1.ID, statusActive, "doing stuff by worker1", "")
	assert.Nil(c, tasksColl.FindId(task1.ID).One(&task1))
	assert.Equal(c, worker1.ID, *task1.WorkerID)
	assert.Equal(c, "doing stuff by worker1", task1.Activity)

	// Worker 1 signs off, so task becomes available again for scheduling to worker 2.
	respRec, ar := WorkerTestRequest(worker1.ID, "POST", "/sign-off")
	WorkerSignOff(respRec, ar, s.db, s.sched)
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
	s.sendTaskUpdate(c, task1.ID, worker2.ID, statusFailed, "doing stuff by worker2", "")
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

func (s *TaskUpdatesTestSuite) TestLogHandling(c *check.C) {
	tasksColl := s.db.C("flamenco_tasks")
	queueColl := s.db.C(queueMgoCollection)

	task := ConstructTestTask("1aaaaaaaaaaaaaaaaaaaaaaa", "testing")
	assert.Nil(c, tasksColl.Insert(task))

	worker := Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"testing"},
	}
	assert.Nil(c, StoreNewWorker(&worker, s.db))

	logEntry1 := "many\nlines\nof\nlogging\nproduced\nby\nthis\nworker\nso\nmany\nmany\nmany\nlines\nit's\ncrazy.\n"
	s.sendTaskUpdate(c, task.ID, worker.ID, statusActive, "doing stuff by worker", logEntry1)

	// Because of this update, the task should be assigned to worker 1
	found := Task{}
	assert.Nil(c, tasksColl.FindId(task.ID).One(&found))
	assert.Equal(c, *found.WorkerID, worker.ID)
	assert.Equal(c, found.Activity, "doing stuff by worker")
	assert.Equal(c, found.Log, "by\nthis\nworker\nso\nmany\nmany\nmany\nlines\nit's\ncrazy.\n",
		"The last 10 log lines should have been stored with the task.")

	// The outgoing queue should not have the entire log, but just the last 10 lines.
	var queuedUpdates []TaskUpdate
	assert.Nil(c, queueColl.Find(bson.M{"task_id": task.ID}).All(&queuedUpdates))
	assert.Equal(c, 1, len(queuedUpdates))
	assert.Equal(c, "doing stuff by worker", queuedUpdates[0].Activity)
	assert.Equal(c, found.Log, queuedUpdates[0].Log,
		"The last 10 log lines should have been queued.")

	// Check the log file
	logdir, logfname := s.queue.taskLogPath(task.Job, task.ID)
	logFilename := path.Join(logdir, logfname)
	contents, err := ioutil.ReadFile(logFilename)
	assert.Nil(c, err)
	assert.Equal(c, logEntry1, string(contents))

	// A subsequent update should append to the log file but not to the task.
	// Also, all logs should be complete lines, so the missing newline should be added.
	logEntry2 := "just\nsome\nmore\nlines"
	s.sendTaskUpdate(c, task.ID, worker.ID, statusActive, "more stuff by worker", logEntry2)

	assert.Nil(c, queueColl.Find(bson.M{"task_id": task.ID}).All(&queuedUpdates))
	assert.Equal(c, 2, len(queuedUpdates))
	assert.Equal(c, "more stuff by worker", queuedUpdates[1].Activity)
	assert.Equal(c, logEntry2+"\n", queuedUpdates[1].Log,
		"For a short update the entire log should be stored.")

	contents, err = ioutil.ReadFile(logFilename)
	assert.Nil(c, err)
	assert.Equal(c, logEntry1+logEntry2+"\n", string(contents))
}

func (s *TaskUpdatesTestSuite) TestTrimLogForTaskUpdate(c *check.C) {
	assert.Equal(c, "", trimLogForTaskUpdate(""))
	assert.Equal(c, "one line\ntwo lines\n", trimLogForTaskUpdate("one line\ntwo lines"))
	assert.Equal(c, "one line\ntwo lines\n", trimLogForTaskUpdate("one line\ntwo lines\n"))

	assert.Equal(c, "by\nthis\nworker\nso\nmany\nmany\nmany\nlines\nit's\ncrazy.\n",
		trimLogForTaskUpdate("many\nlines\nof\nlogging\nproduced\nby\nthis\nworker\nso\nmany\nmany\nmany\nlines\nit's\ncrazy.\n"))
}

func (s *TaskUpdatesTestSuite) TestUnknownJobIDValue(c *check.C) {
	var uninitialized bson.ObjectId
	assert.Equal(c, uninitialized, unknownJobID)
	assert.Equal(c, 0, len(unknownJobID))
	assert.Equal(c, "", unknownJobID.Hex())
}

func (s *TaskUpdatesTestSuite) TestLogRotation(c *check.C) {
	tasksColl := s.db.C("flamenco_tasks")

	task := ConstructTestTask("1aaaaaaaaaaaaaaaaaaaaaaa", "testing")
	assert.Nil(c, tasksColl.Insert(task))

	worker := Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"testing"},
	}
	assert.Nil(c, StoreNewWorker(&worker, s.db))

	logdir, logfname := s.queue.taskLogPath(task.Job, task.ID)
	logFilename := path.Join(logdir, logfname)

	assert.False(c, fileExists(logFilename), "The log file shouldn't exist yet")

	sendUpdate := func(status, activity, log string) {
		s.sendTaskUpdate(c, task.ID, worker.ID, status, activity, log)
	}

	read := func(filename string) string {
		content, err := ioutil.ReadFile(filename)
		assert.Nil(c, err)
		return string(content)
	}

	// This should create a log file.
	logEntry1 := "ENTRY 1: many\nlines\nof\nlogging\nproduced\nby\nthis\nworker\nso\nmany\nmany\nmany\nlines\nit's\ncrazy.\n"
	sendUpdate(statusActive, "doing stuff by worker", logEntry1)
	assert.Equal(c, logEntry1, read(logFilename))

	// A subsequent update should append to the same log file.
	logEntry2 := "ENTRY 2: Some\nmore\nlogging going on.\n"
	sendUpdate("", "some more stuff by worker", logEntry2)
	assert.Equal(c, logEntry1+logEntry2, read(logFilename))

	// Mark as completed -- TODO: check that this file gets GZipped in the background.
	logEntry3 := "ENTRY 3: final line\n"
	sendUpdate(statusCompleted, "done", logEntry3)
	assert.Equal(c, logEntry1+logEntry2+logEntry3, read(logFilename))

	// Re-queue the task.
	assert.Nil(c, s.db.C("flamenco_tasks").UpdateId(task.ID,
		bson.M{"$set": bson.M{"status": statusClaimedByManager}}))

	// Sending another update reactivates the task, and thus should produce a new log file.
	logEntry4 := "ENTRY 4: New run of this task\n"
	sendUpdate(statusActive, "reactivating task", logEntry4)
	assert.Equal(c, logEntry4, read(logFilename))
	assert.Equal(c, logEntry1+logEntry2+logEntry3, read(logFilename+".1"))
}
