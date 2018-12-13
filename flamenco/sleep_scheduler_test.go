package flamenco

import (
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	check "gopkg.in/check.v1"
)

type SleepSchedulerTestSuite struct {
	ss *SleepScheduler
	db *mgo.Database
}

var _ = check.Suite(&SleepSchedulerTestSuite{})

func (s *SleepSchedulerTestSuite) SetUpTest(c *check.C) {
	config := GetTestConfig()
	session := MongoSession(&config)
	s.db = session.DB("")
	s.ss = CreateSleepScheduler(session)
}

func (s *SleepSchedulerTestSuite) TearDownTest(c *check.C) {
	log.Info("SleepSchedulerTestSuite tearing down test, dropping database.")
	s.db.DropDatabase()
}

func (s *SleepSchedulerTestSuite) scheduleTest(c *check.C,
	expectedStatus string, timeToTest time.Time, schedule ScheduleInfo) {

	// If you don't want to test with an active schedule, use another function.
	schedule.ScheduleActive = true
	worker := Worker{
		ID:            bson.NewObjectId(),
		Nickname:      "je moeder",
		Address:       "op je hoofd",
		SleepSchedule: schedule,
	}
	assert.Equal(c, expectedStatus, s.ss.scheduledWorkerStatus(&worker, timeToTest))
}

func (s *SleepSchedulerTestSuite) TestNoSchedule(c *check.C) {
	worker := Worker{
		ID:       bson.NewObjectId(),
		Nickname: "je moeder",
		Address:  "op je hoofd",
	}
	assert.Equal(c, "", s.ss.scheduledWorkerStatus(&worker, time.Now()))
}

func (s *SleepSchedulerTestSuite) TestDisabledSchedule(c *check.C) {
	worker := Worker{
		ID:       bson.NewObjectId(),
		Nickname: "je moeder",
		Address:  "op je hoofd",
		SleepSchedule: ScheduleInfo{
			ScheduleActive: false,
		},
	}
	assert.Equal(c, "", s.ss.scheduledWorkerStatus(&worker, time.Now()))
}

func (s *SleepSchedulerTestSuite) TestNotToday(c *check.C) {
	now := time.Date(2018, 12, 13, 17, 5, 43, 0, time.Local)
	assert.Equal(c, now.Weekday(), time.Thursday)

	s.scheduleTest(c, workerStatusAwake, now, ScheduleInfo{
		DaysOfWeek: "mo tu we",
	})
}

func (s *SleepSchedulerTestSuite) TestTodayWithoutTime(c *check.C) {
	now := time.Date(2018, 12, 12, 17, 5, 43, 0, time.Local)
	assert.Equal(c, now.Weekday(), time.Wednesday)

	s.scheduleTest(c, workerStatusAsleep, now, ScheduleInfo{
		DaysOfWeek: "mo tu we",
	})
}

func (s *SleepSchedulerTestSuite) TestTodayTooEarlyToSleep(c *check.C) {
	// it's early in the morning, too early to sleep.
	now := time.Date(2018, 12, 12, 7, 5, 43, 0, time.Local)
	assert.Equal(c, now.Weekday(), time.Wednesday)

	s.scheduleTest(c, workerStatusAwake, now, ScheduleInfo{
		DaysOfWeek: "mo tu we",
		TimeStart:  &TimeOfDay{8, 0},
		TimeEnd:    &TimeOfDay{17, 0},
	})
}

func (s *SleepSchedulerTestSuite) TestTodayTooLateToSleep(c *check.C) {
	// it's late at night, too late to sleep.
	now := time.Date(2018, 12, 12, 23, 5, 43, 0, time.Local)
	assert.Equal(c, now.Weekday(), time.Wednesday)

	s.scheduleTest(c, workerStatusAwake, now, ScheduleInfo{
		DaysOfWeek: "mo tu we",
		TimeStart:  &TimeOfDay{8, 0},
		TimeEnd:    &TimeOfDay{17, 0},
	})
}

func (s *SleepSchedulerTestSuite) TestWorkingDay(c *check.C) {
	// working hours, someone is using the computer
	now := time.Date(2018, 12, 12, 8, 0, 43, 0, time.Local)
	assert.Equal(c, now.Weekday(), time.Wednesday)

	s.scheduleTest(c, workerStatusAsleep, now, ScheduleInfo{
		DaysOfWeek: "mo tu we",
		TimeStart:  &TimeOfDay{8, 0},
		TimeEnd:    &TimeOfDay{17, 0},
	})
}
