package flamenco

import (
	"bytes"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"

	check "gopkg.in/check.v1"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type DashboardTestSuite struct {
	config    Conf
	session   *mgo.Session
	db        *mgo.Database
	sleeper   *SleepScheduler
	dashboard *Dashboard
	worker    Worker
}

var _ = check.Suite(&DashboardTestSuite{})

func (s *DashboardTestSuite) SetUpTest(c *check.C) {
	s.config = GetTestConfig()
	s.session = MongoSession(&s.config)
	s.db = s.session.DB("")
	s.sleeper = CreateSleepScheduler(s.session)
	blacklist := CreateWorkerBlackList(&s.config, s.session)
	s.dashboard = CreateDashboard(&s.config, s.session, s.sleeper, blacklist, "unittest-1.0")
	s.worker = Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"sleeping"},
		Nickname:           "workerLnx",
		Status:             workerStatusAwake,
	}
	if err := StoreNewWorker(&s.worker, s.db); err != nil {
		c.Fatal("Unable to insert test worker", err)
	}
}

func (s *DashboardTestSuite) TearDownTest(c *check.C) {
	s.db.DropDatabase()
}

func (s *DashboardTestSuite) sendSleepSchedule(c *check.C,
	schedule ScheduleInfo,
	expectedStatusCode int) {

	respRec, req := testJSONRequest(&schedule, "POST", "/dash/set/sleep/schedule")
	req = mux.SetURLVars(req, map[string]string{"worker-id": s.worker.ID.Hex()})
	s.dashboard.setSleepSchedule(respRec, req)
	assert.Equal(c, expectedStatusCode, respRec.Code)
}

func (s *DashboardTestSuite) assertHasSchedule(c *check.C, schedule ScheduleInfo) {
	worker, err := FindWorkerByID(s.worker.ID, s.db)
	assert.Nil(c, err)

	workerSched := worker.SleepSchedule
	assert.Equal(c, schedule.ScheduleActive, workerSched.ScheduleActive)
	assert.Equal(c, schedule.DaysOfWeek, workerSched.DaysOfWeek)

	if schedule.TimeStart == nil {
		assert.Nil(c, workerSched.TimeStart)
	} else if workerSched.TimeStart == nil {
		assert.Fail(c, "worker schedule timeStart is unexpectedly nil, expected %v", schedule.TimeStart.String())
	} else {
		assert.True(c, schedule.TimeStart.Equals(*workerSched.TimeStart),
			"Expected TimeStart %v, but actual is %v", schedule.TimeStart.String(), workerSched.TimeStart.String())
	}

	if schedule.TimeEnd == nil {
		assert.Nil(c, workerSched.TimeEnd)
	} else if workerSched.TimeEnd == nil {
		assert.Fail(c, "worker schedule timeEnd is unexpectedly nil, expected %v", schedule.TimeEnd.String())
	} else {
		assert.True(c, schedule.TimeEnd.Equals(*workerSched.TimeEnd),
			"Expected TimeEnd %v, but actual is %v", schedule.TimeEnd.String(), workerSched.TimeEnd.String())
	}
}

func (s *DashboardTestSuite) TestEmptySleepSchedule(c *check.C) {
	respRec, req := testJSONRequest(bson.M{}, "POST", "/set-sleep-schedule/{worker-id}")
	req = mux.SetURLVars(req, map[string]string{"worker-id": s.worker.ID.Hex()})
	s.dashboard.setSleepSchedule(respRec, req)
	assert.Equal(c, http.StatusNoContent, respRec.Code)

	s.assertHasSchedule(c, ScheduleInfo{})
}

func (s *DashboardTestSuite) TestCompleteSleepSchedule(c *check.C) {
	schedule := ScheduleInfo{
		ScheduleActive: true,
		DaysOfWeek:     "mo tu we fr",
		TimeStart:      &TimeOfDay{6, 15},
		TimeEnd:        &TimeOfDay{17, 30},
	}
	s.sendSleepSchedule(c, schedule, http.StatusNoContent)
	s.assertHasSchedule(c, schedule)
}

func (s *DashboardTestSuite) TestPartialSleepSchedule(c *check.C) {
	schedule := ScheduleInfo{
		ScheduleActive: true,
		DaysOfWeek:     "mo tu we fr",
		TimeStart:      &TimeOfDay{6, 15},
	}
	s.sendSleepSchedule(c, schedule, http.StatusNoContent)
	s.assertHasSchedule(c, schedule)

	schedule.TimeStart = nil
	s.sendSleepSchedule(c, schedule, http.StatusNoContent)
	s.assertHasSchedule(c, schedule)

	schedule.DaysOfWeek = ""
	s.sendSleepSchedule(c, schedule, http.StatusNoContent)
	s.assertHasSchedule(c, schedule)
}

func (s *DashboardTestSuite) TestDaysOfWeekCleanup(c *check.C) {
	schedule := ScheduleInfo{
		ScheduleActive: true,
		DaysOfWeek:     " Mo   TU we\u00A0friday\n",
		TimeStart:      &TimeOfDay{6, 15},
	}
	s.sendSleepSchedule(c, schedule, http.StatusNoContent)

	schedule.DaysOfWeek = "mo tu we fr"
	s.assertHasSchedule(c, schedule)
}

func (s *DashboardTestSuite) TestScheduleOverride(c *check.C) {
	schedule := ScheduleInfo{
		ScheduleActive: true,
		DaysOfWeek:     "mo tu we fr",
		TimeStart:      &TimeOfDay{6, 15},
		TimeEnd:        &TimeOfDay{17, 30},
	}
	s.sendSleepSchedule(c, schedule, http.StatusNoContent)
	s.assertHasSchedule(c, schedule)

	// Explicitly send the worker to some status.
	data := url.Values{}
	data.Add("action", "set-status")
	data.Add("status", workerStatusAsleep)
	b := bytes.NewBuffer([]byte(data.Encode()))

	respRec, req := testRequestWithBody(b, "POST", "/worker-action/{worker-id}")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"worker-id": s.worker.ID.Hex()})

	s.dashboard.workerAction(respRec, req)
	assert.Equal(c, http.StatusNoContent, respRec.Code)

	// Test that the schedule is still active but the worker is properly requested to sleep.
	s.assertHasSchedule(c, schedule)

	found := Worker{}
	err := s.db.C("flamenco_workers").FindId(s.worker.ID).One(&found)
	assert.Nil(c, err)
	assert.Equal(c, workerStatusAsleep, found.StatusRequested)
}
