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
	ss     *SleepScheduler
	db     *mgo.Database
	worker Worker
}

var _ = check.Suite(&SleepSchedulerTestSuite{})

func (s *SleepSchedulerTestSuite) SetUpTest(c *check.C) {
	config := GetTestConfig()
	session := MongoSession(&config)
	s.db = session.DB("")
	s.ss = CreateSleepScheduler(session)
	s.worker = Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"sleeping"},
		Nickname:           "workerLnx",
		Address:            "::1",
		Status:             workerStatusAwake,
	}
	if err := StoreNewWorker(&s.worker, s.db); err != nil {
		c.Fatal("Unable to insert test worker", err)
	}
}

func (s *SleepSchedulerTestSuite) TearDownTest(c *check.C) {
	log.Info("SleepSchedulerTestSuite tearing down test, dropping database.")
	s.db.DropDatabase()
}

func (s *SleepSchedulerTestSuite) updateWorker(c *check.C, updates bson.M) {
	assert.Nil(c, s.db.C("flamenco_workers").UpdateId(s.worker.ID, updates))
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

	// Nothing scheduled for this day, so no change should be requested.
	s.scheduleTest(c, "", now, ScheduleInfo{
		DaysOfWeek: "mo tu we",
	})
}

func (s *SleepSchedulerTestSuite) TestTodayWithoutTime(c *check.C) {
	now := time.Date(2018, 12, 12, 17, 5, 43, 0, time.Local)
	assert.Equal(c, now.Weekday(), time.Wednesday)

	// No time in schedule, but today is sleeping day, so we sleep all day.
	s.scheduleTest(c, workerStatusAsleep, now, ScheduleInfo{
		DaysOfWeek: "mo tu we",
	})
}

func (s *SleepSchedulerTestSuite) TestTodayTooEarlyToSleep(c *check.C) {
	// it's early in the morning, too early to sleep.
	now := time.Date(2018, 12, 12, 7, 5, 43, 0, time.Local)
	assert.Equal(c, now.Weekday(), time.Wednesday)

	schedule := ScheduleInfo{
		DaysOfWeek: "mo tu we",
		TimeStart:  &TimeOfDay{8, 0},
		TimeEnd:    &TimeOfDay{17, 0},
	}
	s.scheduleTest(c, workerStatusAwake, now, schedule)

	// at exactly TimeStart we should sleep, though.
	now = time.Date(2018, 12, 12, 8, 0, 0, 0, time.Local)
	s.scheduleTest(c, workerStatusAsleep, now, schedule)
}

func (s *SleepSchedulerTestSuite) TestTodayTooLateToSleep(c *check.C) {
	// it's late at night, too late to sleep.
	now := time.Date(2018, 12, 12, 23, 5, 43, 0, time.Local)
	assert.Equal(c, now.Weekday(), time.Wednesday)

	schedule := ScheduleInfo{
		DaysOfWeek: "mo tu we",
		TimeStart:  &TimeOfDay{8, 0},
		TimeEnd:    &TimeOfDay{17, 0},
	}
	s.scheduleTest(c, workerStatusAwake, now, schedule)

	// at exactly TimeEnd we should be awake, though.
	now = time.Date(2018, 12, 12, 17, 0, 0, 0, time.Local)
	s.scheduleTest(c, workerStatusAwake, now, schedule)
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

func (s *SleepSchedulerTestSuite) TestScheduleInfoNextCheck(c *check.C) {
	var now time.Time
	timeStart := TimeOfDay{8, 0}
	timeEnd := TimeOfDay{17, 0}
	nextMidnight := TimeOfDay{24, 0}
	schedule := ScheduleInfo{
		TimeStart: &timeStart,
		TimeEnd:   &timeEnd,
	}

	// now < timeStart < timeEnd → the next check should be on timeStart.
	now = time.Date(2018, 12, 12, 7, 59, 43, 0, time.Local)
	assert.Equal(c, timeStart.OnDate(now), *schedule.calculateNextCheck(now))

	// timeStart < now < timeEnd → the next check should be on timeEnd.
	now = time.Date(2018, 12, 12, 8, 0, 43, 0, time.Local)
	assert.Equal(c, timeEnd.OnDate(now), *schedule.calculateNextCheck(now))

	// timeStart < timeEnd < now → the next check should be on 00:00 a day later.
	now = time.Date(2018, 12, 12, 17, 0, 43, 0, time.Local)
	assert.Equal(c, nextMidnight.OnDate(now), *schedule.calculateNextCheck(now))

	// now < timeEnd → the next check should be on timeEnd
	now = time.Date(2018, 12, 12, 7, 59, 43, 0, time.Local)
	schedule.TimeStart = nil
	assert.Equal(c, timeEnd.OnDate(now), *schedule.calculateNextCheck(now))

	// No start/end time → next check should be on midnight the next day.
	schedule.TimeEnd = nil
	assert.Equal(c, nextMidnight.OnDate(now), *schedule.calculateNextCheck(now))

	// timeStart < now → the next check should be on 00:00 a day later.
	now = time.Date(2018, 12, 12, 8, 0, 43, 0, time.Local)
	schedule.TimeStart = &timeStart
	assert.Equal(c, nextMidnight.OnDate(now), *schedule.calculateNextCheck(now))
}

func (s *SleepSchedulerTestSuite) TestRefresh(c *check.C) {
	now := time.Date(2018, 12, 12, 8, 0, 43, 0, time.Local)
	assert.Equal(c, now.Weekday(), time.Wednesday)

	defer func() { s.ss.now = time.Now }()
	s.ss.now = func() time.Time {
		return now
	}

	// Set a typical working day schedule.
	timeStart := TimeOfDay{9, 0}
	timeEnd := TimeOfDay{18, 0}
	schedule := ScheduleInfo{
		ScheduleActive: true,
		DaysOfWeek:     "mo tu we th fr",
		TimeStart:      &timeStart,
		TimeEnd:        &timeEnd,
	}
	s.updateWorker(c, bson.M{"$set": bson.M{"sleep_schedule": schedule}})
	s.ss.RefreshAllWorkers()

	// We're before timeStart, so the worker should be active.
	check := func(expectedStatus, expectedStatusRequested string, expectedNextCheck time.Time) {
		found, err := FindWorkerByID(s.worker.ID, s.db)
		assert.Nil(c, err)
		assert.Equal(c, expectedStatus, found.Status)
		assert.Equal(c, expectedStatusRequested, found.StatusRequested)
		assert.Equal(c, expectedNextCheck, *found.SleepSchedule.NextCheck)
	}
	check(workerStatusAwake, "", timeStart.OnDate(now))

	// timeStart ~ now; almost equal, just a few nanoseconds after.
	// Having it equal will keep the 'next check' to 'now', because 'now' is
	// the moment we have to do the status change.
	log.Info("checking now ~ timeStart")
	now = time.Date(2018, 12, 12, 9, 0, 0, 10, time.Local)
	s.ss.RefreshAllWorkers()
	check(workerStatusAwake, workerStatusAsleep, timeEnd.OnDate(now))

	// Mimick that the worker has gone to sleep, and then was manually set awake.
	s.updateWorker(c, bson.M{"$set": bson.M{
		"status":           workerStatusAwake,
		"status_requested": "",
	}})
	log.Info("checking timeStart < now < timeEnd with worker awake")
	now = time.Date(2018, 12, 12, 10, 0, 0, 0, time.Local)
	s.ss.RefreshAllWorkers()
	check(workerStatusAwake, "", timeEnd.OnDate(now))

	// Mimick that the worker was manually set to sleep.
	s.updateWorker(c, bson.M{"$set": bson.M{
		"status":           workerStatusAsleep,
		"status_requested": "",
	}})
	log.Info("checking timeStart < now < timeEnd with worker awake")
	now = time.Date(2018, 12, 12, 11, 0, 0, 0, time.Local)
	s.ss.RefreshAllWorkers()
	check(workerStatusAsleep, "", timeEnd.OnDate(now))

	// Check what happens when time passes beyond the sleep period.
	log.Info("checking timeStart < timeEnd < now")
	now = time.Date(2018, 12, 12, 20, 0, 0, 0, time.Local)
	s.ss.RefreshAllWorkers()
	check(workerStatusAsleep, workerStatusAwake, TimeOfDay{24, 0}.OnDate(now))
}
