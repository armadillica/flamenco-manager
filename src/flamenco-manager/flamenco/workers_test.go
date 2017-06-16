package flamenco

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	log "github.com/sirupsen/logrus"
	auth "github.com/abbot/go-http-auth"
	"github.com/stretchr/testify/assert"
	check "gopkg.in/check.v1"
	httpmock "gopkg.in/jarcoal/httpmock.v1"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type WorkerTestSuite struct {
	workerLnx Worker
	workerWin Worker

	db       *mgo.Database
	upstream *UpstreamConnection
	sched    *TaskScheduler
}

var _ = check.Suite(&WorkerTestSuite{})

func (s *WorkerTestSuite) SetUpTest(c *check.C) {
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

func (s *WorkerTestSuite) TearDownTest(c *check.C) {
	log.Info("WorkerTestSuite tearing down test, dropping database.")
	s.upstream.Close()
	s.db.DropDatabase()
	httpmock.DeactivateAndReset()
}

func WorkerTestRequest(workerID bson.ObjectId, method, url string, vargs ...interface{}) (*httptest.ResponseRecorder, *auth.AuthenticatedRequest) {
	return WorkerTestRequestWithBody(workerID, nil, method, url, vargs...)
}

func WorkerTestRequestWithBody(workerID bson.ObjectId, body io.Reader, method, url string, vargs ...interface{}) (*httptest.ResponseRecorder, *auth.AuthenticatedRequest) {
	respRec := httptest.NewRecorder()
	if respRec == nil {
		panic("WorkerTestRequest: respRec is nil")
	}

	request, err := http.NewRequest(method, fmt.Sprintf(url, vargs...), body)
	if err != nil {
		panic(err)
	}
	if request == nil {
		panic("WorkerTestRequest: request is nil")
	}

	ar := &auth.AuthenticatedRequest{Request: *request, Username: workerID.Hex()}
	if ar == nil {
		panic("WorkerTestRequest: ar is nil")
	}

	return respRec, ar
}

func (s *WorkerTestSuite) TestWorkerMayRun(t *check.C) {
	// Store task in DB.
	task := ConstructTestTask("aaaaaaaaaaaaaaaaaaaaaaaa", "sleeping")
	if err := s.db.C("flamenco_tasks").Insert(task); err != nil {
		t.Fatal("Unable to insert test task", err)
	}

	// Make sure the scheduler gives us this task.
	respRec, ar := WorkerTestRequest(s.workerLnx.ID, "GET", "/task")
	s.sched.ScheduleTask(respRec, ar)

	// Right after obtaining the task, we should be allowed to keep running it.
	respRec, ar = WorkerTestRequest(s.workerLnx.ID, "GET", "/may-i-run/%s", task.ID.Hex())
	WorkerMayRunTask(respRec, ar, s.db, task.ID)

	resp := MayKeepRunningResponse{}
	parseJSON(t, respRec, 200, &resp)
	assert.Equal(t, "", resp.Reason)
	assert.Equal(t, true, resp.MayKeepRunning)

	// If we now change the task status to "cancel-requested", the worker should be denied.
	assert.Nil(t, s.db.C("flamenco_tasks").UpdateId(task.ID,
		bson.M{"$set": bson.M{"status": "cancel-requested"}}))
	respRec, ar = WorkerTestRequest(s.workerLnx.ID, "GET", "/may-i-run/%s", task.ID.Hex())
	WorkerMayRunTask(respRec, ar, s.db, task.ID)

	resp = MayKeepRunningResponse{}
	parseJSON(t, respRec, 200, &resp)
	assert.Equal(t, false, resp.MayKeepRunning)

	// Changing status back to "active", but assigning to another worker
	assert.Nil(t, s.db.C("flamenco_tasks").UpdateId(task.ID, bson.M{"$set": bson.M{
		"status":    "active",
		"worker_id": s.workerWin.ID,
	}}))
	respRec, ar = WorkerTestRequest(s.workerLnx.ID, "GET", "/may-i-run/%s", task.ID.Hex())
	WorkerMayRunTask(respRec, ar, s.db, task.ID)

	resp = MayKeepRunningResponse{}
	parseJSON(t, respRec, 200, &resp)
	assert.Equal(t, false, resp.MayKeepRunning)
}

func (s *WorkerTestSuite) TestWorkerSignOn(t *check.C) {
	signon := func(body string) {
		respRec, ar := WorkerTestRequestWithBody(
			s.workerLnx.ID, strings.NewReader(body),
			"POST", "/sign-on")
		WorkerSignOn(respRec, ar, s.db)
		assert.Equal(t, 204, respRec.Code)
	}

	found := Worker{}
	getworker := func() {
		err := s.db.C("flamenco_workers").FindId(s.workerLnx.ID).One(&found)
		if err != nil {
			t.Fatal("Unable to find workerLnx: ", err)
		}
	}

	// Empty signon doc -> no change
	signon("{}")
	getworker()
	assert.Equal(t, []string{"sleeping"}, found.SupportedTaskTypes)
	assert.Equal(t, "workerLnx", found.Nickname)

	// Only change nickname
	signon("{\"nickname\": \"new-and-sparkly\"}")
	getworker()
	assert.Equal(t, []string{"sleeping"}, found.SupportedTaskTypes)
	assert.Equal(t, "new-and-sparkly", found.Nickname)

	// Only change supported task types
	signon("{\"supported_task_types\": [\"exr-merge\", \"unknown\"]}")
	getworker()
	assert.Equal(t, []string{"exr-merge", "unknown"}, found.SupportedTaskTypes)
	assert.Equal(t, "new-and-sparkly", found.Nickname)

	// Change both
	signon("{\"supported_task_types\": [\"blender-render\"], \"nickname\": \"another\"}")
	getworker()
	assert.Equal(t, []string{"blender-render"}, found.SupportedTaskTypes)
	assert.Equal(t, "another", found.Nickname)

	// Test that the current task is cleared.
	assert.Nil(t, s.db.C("flamenco_workers").UpdateId(
		s.workerLnx.ID,
		bson.M{"$set": bson.M{"current_task": bson.ObjectIdHex("1234567890ab1234567890ab")}},
	))
	getworker()
	assert.Equal(t, "1234567890ab1234567890ab", found.CurrentTask.Hex())
	signon("{}")
	getworker()
	assert.Nil(t, found.CurrentTask)
}
