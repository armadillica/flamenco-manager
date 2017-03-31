package flamenco

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	auth "github.com/abbot/go-http-auth"
	"github.com/stretchr/testify/assert"
	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

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

func (s *SchedulerTestSuite) TestWorkerMayRun(t *check.C) {
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
