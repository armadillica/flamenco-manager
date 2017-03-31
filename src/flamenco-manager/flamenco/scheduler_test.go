package flamenco

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	log "github.com/Sirupsen/logrus"
	auth "github.com/abbot/go-http-auth"
	"github.com/stretchr/testify/assert"

	check "gopkg.in/check.v1"
	"gopkg.in/jarcoal/httpmock.v1"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type SchedulerTestSuite struct {
	workerLnx Worker
	workerWin Worker

	db       *mgo.Database
	upstream *UpstreamConnection
	sched    *TaskScheduler
}

var _ = check.Suite(&SchedulerTestSuite{})

func parseJSON(c *check.C, respRec *httptest.ResponseRecorder, expectedStatus int, parsed interface{}) {
	assert.Equal(c, expectedStatus, respRec.Code)
	headers := respRec.Header()
	assert.Equal(c, "application/json", headers.Get("Content-Type"))

	decoder := json.NewDecoder(respRec.Body)
	if err := decoder.Decode(&parsed); err != nil {
		c.Fatalf("Unable to decode JSON: %s", err)
	}
}

func (s *SchedulerTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()

	config := GetTestConfig()
	session := MongoSession(&config)
	s.db = session.DB("")

	s.upstream = ConnectUpstream(&config, session)
	s.sched = CreateTaskScheduler(&config, s.upstream, session)

	// Store workers in DB, on purpose in the opposite order as the tasks.
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

func (s *SchedulerTestSuite) TearDownTest(c *check.C) {
	log.Info("SchedulerTestSuite tearing down test, dropping database.")
	s.upstream.Close()
	s.db.DropDatabase()
	httpmock.DeactivateAndReset()
}

/**
 * In this test we don't mock the upstream HTTP connection, so it's normal to see
 * errors about failed requests. These are harmless. As a matter of fact, testing
 * in such error conditions is good; task scheduling should keep working.
 */
func (s *SchedulerTestSuite) TestVariableReplacement(t *check.C) {
	// Store task in DB.
	task1 := ConstructTestTask("aaaaaaaaaaaaaaaaaaaaaaaa", "testing")
	if err := s.db.C("flamenco_tasks").Insert(task1); err != nil {
		t.Fatal("Unable to insert test task", err)
	}
	task2 := ConstructTestTask("1aaaaaaaaaaaaaaaaaaaaaaa", "sleeping")
	if err := s.db.C("flamenco_tasks").Insert(task2); err != nil {
		t.Fatal("Unable to insert test task 2", err)
	}

	// Perform HTTP request
	respRec := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/task", nil)
	ar := &auth.AuthenticatedRequest{Request: *request, Username: s.workerLnx.ID.Hex()}
	s.sched.ScheduleTask(respRec, ar)

	// Check the response JSON
	jsonTask := Task{}
	parseJSON(t, respRec, 200, &jsonTask)
	assert.Equal(t, "active", jsonTask.Status)
	assert.Equal(t, "unittest", jsonTask.JobType)
	assert.Equal(t, "sleeping", jsonTask.TaskType)
	assert.Equal(t, "Running Blender from /opt/myblenderbuild/blender",
		jsonTask.Commands[0].Settings["message"])

	// Check worker with other task type
	ar = &auth.AuthenticatedRequest{Request: *request, Username: s.workerWin.ID.Hex()}
	s.sched.ScheduleTask(respRec, ar)

	// Check the response JSON
	parseJSON(t, respRec, 200, &jsonTask)
	assert.Equal(t, "active", jsonTask.Status)
	assert.Equal(t, "unittest", jsonTask.JobType)
	assert.Equal(t, "testing", jsonTask.TaskType)
	assert.Equal(t, "Running Blender from c:/temp/blender.exe",
		jsonTask.Commands[0].Settings["message"])

}

func (s *SchedulerTestSuite) TestSchedulerOrderByPriority(t *check.C) {
	// Store task in DB.
	task1 := ConstructTestTaskWithPrio("1aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 50)
	if err := s.db.C("flamenco_tasks").Insert(task1); err != nil {
		t.Fatal("Unable to insert test task1", err)
	}
	task2 := ConstructTestTaskWithPrio("2aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 100)
	if err := s.db.C("flamenco_tasks").Insert(task2); err != nil {
		t.Fatal("Unable to insert test task 2", err)
	}

	// Perform HTTP request to the scheduler.
	respRec := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/task", nil)
	ar := &auth.AuthenticatedRequest{Request: *request, Username: s.workerLnx.ID.Hex()}
	s.sched.ScheduleTask(respRec, ar)

	// We should have gotten task 2, because it has the highest priority.
	jsonTask := Task{}
	parseJSON(t, respRec, 200, &jsonTask)
	assert.Equal(t, task2.ID.Hex(), jsonTask.ID.Hex())
}

func (s *SchedulerTestSuite) TestSchedulerOrderByJobPriority(t *check.C) {
	// Store task in DB.
	task1 := ConstructTestTaskWithPrio("1aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 50)
	task1.JobPriority = 10
	if err := s.db.C("flamenco_tasks").Insert(task1); err != nil {
		t.Fatal("Unable to insert test task1", err)
	}
	task2 := ConstructTestTaskWithPrio("2aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 100)
	task2.JobPriority = 5
	if err := s.db.C("flamenco_tasks").Insert(task2); err != nil {
		t.Fatal("Unable to insert test task 2", err)
	}

	// Perform HTTP request to the scheduler.
	respRec := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/task", nil)
	ar := &auth.AuthenticatedRequest{Request: *request, Username: s.workerLnx.ID.Hex()}
	s.sched.ScheduleTask(respRec, ar)

	// We should have gotten task 1, because its job has the highest priority.
	jsonTask := Task{}
	parseJSON(t, respRec, 200, &jsonTask)
	assert.Equal(t, task1.ID.Hex(), jsonTask.ID.Hex())
}

/**
 * The failure case, where the TaskScheduler cannot reach the Server to check
 * the task for updates, is already implicitly handled in the TestVariableReplacement
 * test case; a Responder for that endpoint isn't registered there, and thus it results
 * in a connection error.
 */
func (s *SchedulerTestSuite) TestSchedulerVerifyUpstreamCanceled(t *check.C) {
	// Store task in DB.
	task1 := ConstructTestTaskWithPrio("1aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 50)
	if err := s.db.C("flamenco_tasks").Insert(task1); err != nil {
		t.Fatal("Unable to insert test task1", err)
	}
	task2 := ConstructTestTaskWithPrio("2aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 100)
	if err := s.db.C("flamenco_tasks").Insert(task2); err != nil {
		t.Fatal("Unable to insert test task 2", err)
	}

	timeout := TimeoutAfter(1 * time.Second)
	defer close(timeout)

	// Mock that the task with highest priority was actually canceled on the Server.
	httpmock.RegisterResponder(
		"GET",
		"http://localhost:51234/api/flamenco/tasks/2aaaaaaaaaaaaaaaaaaaaaaa",
		func(req *http.Request) (*http.Response, error) {
			defer func() { timeout <- false }()
			log.Info("GET from manager received on server, sending back updated task.")

			// same task, but with changed status.
			changedTask := task2
			changedTask.Status = "canceled"
			return httpmock.NewJsonResponse(200, &changedTask)
		},
	)

	// Perform HTTP request to the scheduler.
	respRec := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/task", nil)
	ar := &auth.AuthenticatedRequest{Request: *request, Username: s.workerLnx.ID.Hex()}
	s.sched.ScheduleTask(respRec, ar)

	timedout := <-timeout
	assert.False(t, timedout, "HTTP GET to Flamenco Server not performed")

	// Check the response JSON
	jsonTask := Task{}
	parseJSON(t, respRec, 200, &jsonTask)

	// We should have gotten task 1, because task 2 was canceled.
	assert.Equal(t, task1.ID.Hex(), jsonTask.ID.Hex())

	// In our queue, task 2 should have been canceled, since it was canceled on the server.
	foundTask := Task{}
	err := s.db.C("flamenco_tasks").FindId(task2.ID).One(&foundTask)
	assert.Equal(t, nil, err)
	assert.Equal(t, "canceled", foundTask.Status)
}

func (s *SchedulerTestSuite) TestSchedulerVerifyUpstreamPrioChange(t *check.C) {
	// Store task in DB.
	task1 := ConstructTestTaskWithPrio("1aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 50)
	if err := s.db.C("flamenco_tasks").Insert(task1); err != nil {
		t.Fatal("Unable to insert test task1", err)
	}
	task2 := ConstructTestTaskWithPrio("2aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 100)
	if err := s.db.C("flamenco_tasks").Insert(task2); err != nil {
		t.Fatal("Unable to insert test task 2", err)
	}

	timeout := TimeoutAfter(1 * time.Second)
	defer close(timeout)

	// Mock that the task with highest priority was actually canceled on the Server.
	httpmock.RegisterResponder(
		"GET",
		"http://localhost:51234/api/flamenco/tasks/2aaaaaaaaaaaaaaaaaaaaaaa",
		func(req *http.Request) (*http.Response, error) {
			defer func() { timeout <- false }()
			log.Info("GET from manager received on server, sending back updated task.")

			// same task, but with changed status.
			changedTask := task2
			changedTask.Priority = 5
			return httpmock.NewJsonResponse(200, &changedTask)
		},
	)

	// Perform HTTP request to the scheduler.
	respRec := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/task", nil)
	ar := &auth.AuthenticatedRequest{Request: *request, Username: s.workerLnx.ID.Hex()}
	s.sched.ScheduleTask(respRec, ar)

	timedout := <-timeout
	assert.False(t, timedout, "HTTP GET to Flamenco Server not performed")

	// Check the response JSON
	jsonTask := Task{}
	parseJSON(t, respRec, 200, &jsonTask)

	// We should have gotten task 1, because task 2 was lowered in prio.
	assert.Equal(t, task1.ID.Hex(), jsonTask.ID.Hex())

	// In our queue, task 2 should have been lowered in prio, and task1 should be active.
	foundTask := Task{}
	err := s.db.C("flamenco_tasks").FindId(task2.ID).One(&foundTask)
	assert.Equal(t, nil, err)
	assert.Equal(t, "queued", foundTask.Status)
	assert.Equal(t, 5, foundTask.Priority)

	err = s.db.C("flamenco_tasks").FindId(task1.ID).One(&foundTask)
	assert.Equal(t, nil, err)
	assert.Equal(t, "active", foundTask.Status)
	assert.Equal(t, 50, foundTask.Priority)
}

func (s *SchedulerTestSuite) TestSchedulerVerifyUpstreamDeleted(t *check.C) {
	// Store task in DB.
	task1 := ConstructTestTaskWithPrio("1aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 50)
	if err := s.db.C("flamenco_tasks").Insert(task1); err != nil {
		t.Fatal("Unable to insert test task1", err)
	}
	task2 := ConstructTestTaskWithPrio("2aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 100)
	if err := s.db.C("flamenco_tasks").Insert(task2); err != nil {
		t.Fatal("Unable to insert test task 2", err)
	}

	timeout := TimeoutAfter(1 * time.Second)
	defer close(timeout)

	// Mock that the task with highest priority was actually canceled on the Server.
	httpmock.RegisterResponder(
		"GET",
		"http://localhost:51234/api/flamenco/tasks/2aaaaaaaaaaaaaaaaaaaaaaa",
		func(req *http.Request) (*http.Response, error) {
			defer func() { timeout <- false }()
			log.Info("GET from manager received on server, sending back 404.")
			return httpmock.NewStringResponse(404, ""), nil
		},
	)

	// Perform HTTP request to the scheduler.
	respRec := httptest.NewRecorder()
	request, _ := http.NewRequest("GET", "/task", nil)
	ar := &auth.AuthenticatedRequest{Request: *request, Username: s.workerLnx.ID.Hex()}
	s.sched.ScheduleTask(respRec, ar)

	timedout := <-timeout
	assert.False(t, timedout, "HTTP GET to Flamenco Server not performed")

	// Check the response JSON
	jsonTask := Task{}
	parseJSON(t, respRec, 200, &jsonTask)

	// We should have gotten task 1, because task 2 was deleted.
	assert.Equal(t, task1.ID.Hex(), jsonTask.ID.Hex())

	// In our queue, task 2 should have been canceled, and task1 should be active.
	foundTask := Task{}
	err := s.db.C("flamenco_tasks").FindId(task2.ID).One(&foundTask)
	assert.Equal(t, nil, err)
	assert.Equal(t, "canceled", foundTask.Status)
	assert.Equal(t, 100, foundTask.Priority)

	err = s.db.C("flamenco_tasks").FindId(task1.ID).One(&foundTask)
	assert.Equal(t, nil, err)
	assert.Equal(t, "active", foundTask.Status)
	assert.Equal(t, 50, foundTask.Priority)
}

func (s *SchedulerTestSuite) TestParentTaskNotCompleted(c *check.C) {
	tasksColl := s.db.C("flamenco_tasks")

	// Task 1 is being worked on by workerWin
	task1 := ConstructTestTaskWithPrio("1aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 50)
	task1.Status = "active"
	task1.WorkerID = &s.workerWin.ID
	assert.Nil(c, tasksColl.Insert(task1))

	// Task 2 is unavailable due to its parent not being completed.
	task2 := ConstructTestTaskWithPrio("2aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 100)
	task2.Parents = []bson.ObjectId{task1.ID}
	task2.Status = "claimed-by-manager"
	assert.Nil(c, tasksColl.Insert(task2))

	// Fetch a task from the queue
	respRec, _ := WorkerTestRequest(s.workerLnx.ID, "TEST", "/whatevah")
	task := s.sched.fetchTaskFromQueueOrManager(respRec, s.db, &s.workerLnx)

	// We should not get any task back, since task1 is already taken, and task2
	// has a non-completed parent.
	assert.Nil(c, task, "Expected nil, got task %v instead", task)
	assert.Equal(c, http.StatusNoContent, respRec.Code)
}

func (s *SchedulerTestSuite) TestParentTaskCompleted(c *check.C) {
	tasksColl := s.db.C("flamenco_tasks")

	// Task 1 has been completed by workerWin
	task1 := ConstructTestTaskWithPrio("1aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 50)
	task1.Status = "completed"
	task1.WorkerID = &s.workerWin.ID
	assert.Nil(c, tasksColl.Insert(task1))

	// Task 2 is available due to its parent being completed.
	task2 := ConstructTestTaskWithPrio("2aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 100)
	task2.Parents = []bson.ObjectId{task1.ID}
	task2.Status = "claimed-by-manager"
	assert.Nil(c, tasksColl.Insert(task2))

	// Fetch a task from the queue
	respRec, _ := WorkerTestRequest(s.workerLnx.ID, "TEST", "/whatevah")
	task := s.sched.fetchTaskFromQueueOrManager(respRec, s.db, &s.workerLnx)
	assert.Equal(c, http.StatusOK, respRec.Code)

	// We should get task 2.
	assert.NotNil(c, task, "Expected task %s, got nil instead", task2.ID.Hex())
	if task != nil { // prevent nil pointer dereference
		assert.Equal(c, task.ID, task2.ID, "Expected task %s, got task %s instead",
			task2.ID.Hex(), task.ID.Hex())
	}
}

func (s *SchedulerTestSuite) TestParentTaskOneCompletedOneNot(c *check.C) {
	tasksColl := s.db.C("flamenco_tasks")

	// Task 1 is being worked on by workerWin
	task1 := ConstructTestTaskWithPrio("1aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 50)
	task1.Status = "active"
	task1.WorkerID = &s.workerWin.ID
	assert.Nil(c, tasksColl.Insert(task1))

	// Task 2 is already completed.
	task2 := ConstructTestTaskWithPrio("2aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 50)
	task2.Status = "completed"
	task2.WorkerID = &s.workerWin.ID
	assert.Nil(c, tasksColl.Insert(task2))

	// Task 3 is unavailable due to one of its parent not being completed.
	task3 := ConstructTestTaskWithPrio("3aaaaaaaaaaaaaaaaaaaaaaa", "sleeping", 100)
	task3.Parents = []bson.ObjectId{task1.ID, task2.ID}
	task3.Status = "claimed-by-manager"
	assert.Nil(c, tasksColl.Insert(task3))

	// Fetch a task from the queue
	respRec, _ := WorkerTestRequest(s.workerLnx.ID, "TEST", "/whatevah")
	task := s.sched.fetchTaskFromQueueOrManager(respRec, s.db, &s.workerLnx)

	// We should not get any task back.
	assert.Nil(c, task, "Expected nil, got task %v instead", task)
	assert.Equal(c, http.StatusNoContent, respRec.Code)
}
