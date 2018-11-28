package flamenco

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	auth "github.com/abbot/go-http-auth"
	log "github.com/sirupsen/logrus"
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
	notifier *UpstreamNotifier
}

var _ = check.Suite(&WorkerTestSuite{})

func (s *WorkerTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()

	config := GetTestConfig()
	session := MongoSession(&config)
	s.db = session.DB("")

	s.upstream = ConnectUpstream(&config, session)
	s.sched = CreateTaskScheduler(&config, s.upstream, session)
	s.notifier = CreateUpstreamNotifier(&config, s.upstream, session)

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
	request.RemoteAddr = "[::1]:65532"

	ar := &auth.AuthenticatedRequest{Request: *request, Username: workerID.Hex()}
	if ar == nil {
		panic("WorkerTestRequest: ar is nil")
	}

	return respRec, ar
}

func (s *WorkerTestSuite) TestWorkerSignOn(t *check.C) {
	serverUpdateMethod := "POST"
	serverUpdateURL := "http://localhost:51234/api/flamenco/managers/5852bc5198377351f95d103e/update"
	serverUpdateKey := serverUpdateMethod + " " + serverUpdateURL

	signon := func(body string) {
		respRec, ar := WorkerTestRequestWithBody(
			s.workerLnx.ID, strings.NewReader(body),
			"POST", "/sign-on")
		WorkerSignOn(respRec, ar, s.db, s.notifier)
		assert.Equal(t, 204, respRec.Code)
	}

	found := Worker{}
	getworker := func() {
		err := s.db.C("flamenco_workers").FindId(s.workerLnx.ID).One(&found)
		assert.Nil(t, err, "unable to find workerLnx: %s", err)
	}

	// Empty signon doc -> no change
	signon("{}")
	getworker()
	assert.Equal(t, []string{"sleeping"}, found.SupportedTaskTypes)
	assert.Equal(t, "workerLnx", found.Nickname)
	assert.Equal(t, 0, httpmock.GetCallCountInfo()[serverUpdateKey],
		"Unexpected %s request to %s", serverUpdateMethod, serverUpdateURL)

	// Only change nickname
	signon("{\"nickname\": \"new-and-sparkly\"}")
	getworker()
	assert.Equal(t, []string{"sleeping"}, found.SupportedTaskTypes)
	assert.Equal(t, "new-and-sparkly", found.Nickname)
	assert.Equal(t, 0, httpmock.GetCallCountInfo()[serverUpdateKey],
		"Unexpected %s request to %s", serverUpdateMethod, serverUpdateURL)

	// Only change supported task types
	callMade := make(chan bool, 1)
	httpmock.RegisterResponder(
		serverUpdateMethod, serverUpdateURL,
		func(req *http.Request) (*http.Response, error) {
			defer func() { callMade <- true }()
			// TODO: test contents of request
			log.Info("HTTP POST to Flamenco was performed.")
			return httpmock.NewStringResponse(204, ""), nil
		},
	)
	signon("{\"supported_task_types\": [\"exr-merge\", \"unknown\"]}")
	getworker()
	assert.Equal(t, []string{"exr-merge", "unknown"}, found.SupportedTaskTypes)
	assert.Equal(t, "new-and-sparkly", found.Nickname)

	select {
	case <-callMade:
		break
	case <-time.After(250 * time.Millisecond):
		assert.Fail(t, "Timeout waiting for notification")
	}
	assert.Equal(t, 1, httpmock.GetCallCountInfo()[serverUpdateKey],
		"%s request to %s not made", serverUpdateMethod, serverUpdateURL)

	// Change both
	signon("{\"supported_task_types\": [\"blender-render\"], \"nickname\": \"another\"}")
	getworker()
	assert.Equal(t, []string{"blender-render"}, found.SupportedTaskTypes)
	assert.Equal(t, "another", found.Nickname)

	select {
	case <-callMade:
		break
	case <-time.After(250 * time.Millisecond):
		assert.Fail(t, "Timeout waiting for notification")
	}
	assert.Equal(t, 2, httpmock.GetCallCountInfo()[serverUpdateKey],
		"%s request to %s not made", serverUpdateMethod, serverUpdateURL)

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

func (s *WorkerTestSuite) TestWorkerSignOff(t *check.C) {
	signoff := func() {
		respRec, ar := WorkerTestRequest(s.workerLnx.ID, "POST", "/sign-off")
		WorkerSignOff(respRec, ar, s.db, s.sched)
		assert.Equal(t, 204, respRec.Code)
	}

	found := Worker{}
	getworker := func() {
		err := s.db.C("flamenco_workers").FindId(s.workerLnx.ID).One(&found)
		assert.Nil(t, err, "unable to find workerLnx: %s", err)
	}

	// Signing off when awake
	s.workerLnx.SetStatus(workerStatusAwake, s.db)
	signoff()
	getworker()
	assert.Equal(t, workerStatusOffline, found.Status)
	assert.Equal(t, "", found.StatusRequested)

	// Signing off when asleep
	s.workerLnx.SetStatus(workerStatusAsleep, s.db)
	signoff()
	getworker()
	assert.Equal(t, workerStatusOffline, found.Status)
	assert.Equal(t, workerStatusAsleep, found.StatusRequested)

	// Signing off when timed out
	s.workerLnx.SetStatus(workerStatusTimeout, s.db)
	signoff()
	getworker()
	assert.Equal(t, workerStatusOffline, found.Status)
	assert.Equal(t, "", found.StatusRequested)

	// Signing off when awake and shutdown requested
	s.workerLnx.SetStatus(workerStatusAwake, s.db)
	s.workerLnx.RequestStatusChange(workerStatusShutdown, s.db)
	signoff()
	getworker()
	assert.Equal(t, workerStatusOffline, found.Status)
	assert.Equal(t, "", found.StatusRequested)

	// Signing off when asleep
	s.workerLnx.SetStatus(workerStatusAsleep, s.db)
	s.workerLnx.RequestStatusChange(workerStatusShutdown, s.db)
	signoff()
	getworker()
	assert.Equal(t, workerStatusOffline, found.Status)
	assert.Equal(t, workerStatusAsleep, found.StatusRequested)

	// Signing off when timed out
	s.workerLnx.SetStatus(workerStatusTimeout, s.db)
	s.workerLnx.RequestStatusChange(workerStatusShutdown, s.db)
	signoff()
	getworker()
	assert.Equal(t, workerStatusOffline, found.Status)
	assert.Equal(t, "", found.StatusRequested)
}

// Tests receiving the status change via /may-i-run and /task
func (s *WorkerTestSuite) TestStatusChangeReceiving(t *check.C) {
	// Requesting a new status should set it both on the instance and on the database.
	err := s.workerLnx.RequestStatusChange(workerStatusAsleep, s.db)
	assert.Nil(t, err)
	assert.Equal(t, workerStatusAsleep, s.workerLnx.StatusRequested)
	assert.Equal(t, workerStatusAwake, s.workerLnx.Status)

	found := Worker{}
	err = s.db.C("flamenco_workers").FindId(s.workerLnx.ID).One(&found)
	assert.Nil(t, err, "Unable to find workerLnx")
	assert.Equal(t, workerStatusAsleep, found.StatusRequested)
	assert.Equal(t, workerStatusAwake, found.Status)

	// The worker should get this status when either calling may-i-run or asking for a new task.
	// TODO(sybren) determine what to do upon sign-on.

	// Store task in DB and ask if we're allowed to keep running it.
	task := ConstructTestTask("aaaaaaaaaaaaaaaaaaaaaaaa", "sleeping")
	if err := s.db.C("flamenco_tasks").Insert(task); err != nil {
		t.Fatal("Unable to insert test task", err)
	}
	respRec, ar := WorkerTestRequest(s.workerLnx.ID, "GET", "/may-i-run/%s", task.ID.Hex())
	s.sched.WorkerMayRunTask(respRec, ar, s.db, task.ID)
	assert.Equal(t, http.StatusOK, respRec.Code)

	resp := MayKeepRunningResponse{}
	parseJSON(t, respRec, 200, &resp)
	assert.Equal(t, false, resp.MayKeepRunning)
	assert.Equal(t, "asleep", resp.StatusRequested)
	assert.NotEqual(t, "", resp.Reason)

	// Try fetching a new task, this should also fail with the new status in there.
	respRec, ar = WorkerTestRequest(s.workerLnx.ID, "POST", "/task")
	s.sched.ScheduleTask(respRec, ar)
	schedResp := WorkerStatus{}
	parseJSON(t, respRec, http.StatusLocked, &schedResp)
	assert.Equal(t, workerStatusAsleep, schedResp.StatusRequested)
}

func (s *WorkerTestSuite) TestAckStatusChange(t *check.C) {
	err := s.workerLnx.RequestStatusChange(workerStatusAsleep, s.db)
	assert.Nil(t, err)
	err = s.workerLnx.AckStatusChange(workerStatusAsleep, s.db)
	assert.Nil(t, err)

	assert.Equal(t, "", s.workerLnx.StatusRequested)
	assert.Equal(t, workerStatusAsleep, s.workerLnx.Status)

	found := Worker{}
	err = s.db.C("flamenco_workers").FindId(s.workerLnx.ID).One(&found)
	assert.Nil(t, err, "Unable to find workerLnx")
	assert.Equal(t, "", found.StatusRequested)
	assert.Equal(t, workerStatusAsleep, found.Status)
}

func (s *WorkerTestSuite) TestTimeout(t *check.C) {
	s.workerLnx.SetStatus(workerStatusAsleep, s.db)
	s.workerLnx.Timeout(s.db)

	assert.Equal(t, workerStatusAsleep, s.workerLnx.StatusRequested)
	assert.Equal(t, workerStatusTimeout, s.workerLnx.Status)

	found := Worker{}
	err := s.db.C("flamenco_workers").FindId(s.workerLnx.ID).One(&found)
	assert.Nil(t, err, "Unable to find workerLnx")
	assert.Equal(t, workerStatusAsleep, found.StatusRequested)
	assert.Equal(t, workerStatusTimeout, found.Status)
}

func (s *WorkerTestSuite) TestStatusChangeNotRequestable(t *check.C) {
	teststatus := func(status string) {
		err := s.workerLnx.RequestStatusChange(status, s.db)
		assert.NotNil(t, err)
		assert.Equal(t, "", s.workerLnx.StatusRequested)
		assert.Equal(t, workerStatusAwake, s.workerLnx.Status)

		found := Worker{}
		err = s.db.C("flamenco_workers").FindId(s.workerLnx.ID).One(&found)
		assert.Nil(t, err, "Unable to find workerLnx")
		assert.Equal(t, "", found.StatusRequested)
		assert.Equal(t, workerStatusAwake, found.Status)
	}

	teststatus(workerStatusOffline)
	teststatus(workerStatusTimeout)
}

func (s *WorkerTestSuite) TestAckTimeout(t *check.C) {
	// Ack'ing a timeout shouldn't work unless the worker is actually in timeout state.
	err := s.workerLnx.AckTimeout(s.db)
	assert.NotNil(t, err)

	s.workerLnx.Status = workerStatusTimeout
	err = s.workerLnx.AckTimeout(s.db)
	assert.Nil(t, err)

	assert.Equal(t, "", s.workerLnx.StatusRequested)
	assert.Equal(t, workerStatusOffline, s.workerLnx.Status)

	found := Worker{}
	err = s.db.C("flamenco_workers").FindId(s.workerLnx.ID).One(&found)
	assert.Nil(t, err, "Unable to find workerLnx")
	assert.Equal(t, "", found.StatusRequested)
	assert.Equal(t, workerStatusOffline, found.Status)
}

func (s *WorkerTestSuite) TestWorkerPingedTaskEffectOnStatus(t *check.C) {
	task := ConstructTestTask("aaaaaaaaaaaaaaaaaaaaaaaa", "sleeping")
	if err := s.db.C("flamenco_tasks").Insert(task); err != nil {
		t.Fatal("Unable to insert test task", err)
	}
	respRec, ar := WorkerTestRequest(s.workerLnx.ID, "GET", "/task")
	s.sched.ScheduleTask(respRec, ar)

	// Force the task into a non-runnable status, then ping the task again.
	// This shouldn't change the task status.
	if err := s.db.C("flamenco_tasks").UpdateId(task.ID,
		bson.M{"$set": bson.M{"status": "failed"}}); err != nil {
		t.Fatal("Unable to update test task", err)
	}

	loadTask := func() *Task {
		dbTask := Task{}
		if err := s.db.C("flamenco_tasks").FindId(task.ID).One(&dbTask); err != nil {
			t.Fatal("Unable to find task in DB", err)
		}
		return &dbTask
	}

	prePingTimestamp := loadTask().LastWorkerPing
	assert.NotNil(t, prePingTimestamp)
	WorkerPingedTask(s.workerLnx.ID, task.ID, "active", s.db)

	dbTask := loadTask()
	assert.Equal(t, "failed", dbTask.Status)
	assert.Condition(t, func() bool {
		return dbTask.LastWorkerPing.After(*prePingTimestamp)
	})
}

func (s *WorkerTestSuite) TestErrorStatus(t *check.C) {
	err := s.workerLnx.AckStatusChange(workerStatusError, s.db)
	assert.Nil(t, err)

	assert.Equal(t, "", s.workerLnx.StatusRequested)
	assert.Equal(t, workerStatusError, s.workerLnx.Status)

	found := Worker{}
	err = s.db.C("flamenco_workers").FindId(s.workerLnx.ID).One(&found)
	assert.Nil(t, err, "Unable to find workerLnx")
	assert.Equal(t, "", found.StatusRequested)
	assert.Equal(t, workerStatusError, found.Status)
}
