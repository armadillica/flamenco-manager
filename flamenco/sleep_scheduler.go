package flamenco

import (
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// Time period for checking the schedule of every worker.
const sleepSchedulerCheckInterval = 1 * time.Minute

// SleepScheduler manages wake/sleep cycles of Workers.
type SleepScheduler struct {
	closable
	session *mgo.Session
	now     func() time.Time // for mocking the time.Now() function
}

// CreateSleepScheduler creates a new SleepScheduler.
func CreateSleepScheduler(session *mgo.Session) *SleepScheduler {
	return &SleepScheduler{
		makeClosable(),
		session,
		time.Now,
	}
}

// Go starts a new goroutine to perform the periodic checking of the schedule.
func (ss *SleepScheduler) Go() {
	ss.closableAdd(1)
	go func() {
		defer ss.closableDone()
		defer log.Info("SleepScheduler: shutting down.")

		timer := Timer("SleepScheduler", sleepSchedulerCheckInterval, 10*time.Second, &ss.closable)
		for range timer {
			ss.RefreshAllWorkers()
		}
	}()
}

// Close gracefully shuts down the sleep scheduler goroutine.
func (ss *SleepScheduler) Close() {
	log.Debug("SleepScheduler: Close() called.")
	ss.closableCloseAndWait()
	log.Debug("SleepScheduler: shutdown complete.")
}

// RefreshAllWorkers updates the status of all workers for which a schedule is active.
func (ss *SleepScheduler) RefreshAllWorkers() {
	// This function could be called from different goroutines.
	session := ss.session.Copy()
	db := session.DB("")
	defer session.Close()

	coll := db.C("flamenco_workers")
	queryDoc := bson.M{
		"sleep_schedule.schedule_active": true,
		// If 'next_check' is not after 'now', it means it either doesn't exist
		// or it's before 'now'.
		"sleep_schedule.next_check": bson.M{"$not": bson.M{"$gt": ss.now()}},
	}
	query := coll.Find(queryDoc)
	count, err := query.Count()
	if err != nil {
		log.WithError(err).Error("SleepScheduler: unable to count workers to refresh")
		return
	}
	if count == 0 {
		log.Debug("SleepScheduler: no workers to refresh")
		return
	}
	log.WithField("workers_to_refresh", count).Info("SleepScheduler: refreshing workers")

	iter := query.Iter()
	worker := Worker{}

	for iter.Next(&worker) {
		ss.refreshWorker(&worker, db)
	}
	if err := iter.Close(); err != nil {
		log.WithError(err).Error("SleepScheduler: error querying MongoDB")
	}
}

// RequestWorkerStatus sets worker.StatusRequested if the scheduler demands a status change.
func (ss *SleepScheduler) RequestWorkerStatus(worker *Worker, db *mgo.Database) {
	scheduled := ss.scheduledWorkerStatus(worker, ss.now())
	if scheduled == "" || worker.StatusRequested == scheduled {
		return
	}
	logger := log.WithFields(log.Fields{
		"worker":           worker.Identifier(),
		"current_status":   worker.Status,
		"scheduled_status": scheduled,
	})
	if worker.StatusRequested != "" {
		logger.WithField("old_status_requested", worker.StatusRequested).Info("overruling previously requested status with scheduled status")
	} else {
		logger.Info("requesting worker to switch to scheduled status")
	}
	if err := worker.RequestStatusChange(scheduled, Immediate, db); err != nil {
		logger.WithError(err).Error("unable to store status change in database")
	}
}

func (ss *SleepScheduler) refreshWorker(worker *Worker, db *mgo.Database) {
	// Setting the schedule recalculates the NextCheck property and applies the schedule.
	log.WithFields(log.Fields{
		"worker":         worker.Identifier(),
		"current_status": worker.Status,
	}).Info("checking sleep schedule")
	ss.SetSleepSchedule(worker, worker.SleepSchedule, db)
}

// scheduledWorkerStatus returns the expected worker status at the given timestamp.
// Returns an empty string when the schedule is inactive or the worker already has
// the appropriate status.
func (ss *SleepScheduler) scheduledWorkerStatus(worker *Worker, forTime time.Time) string {
	tod := MakeTimeOfDay(forTime)
	logger := log.WithField("worker", worker.Identifier())
	sched := worker.SleepSchedule
	if !sched.ScheduleActive {
		logger.Debug("worker has disabled sleep schedule")
		return ""
	}

	weekdayName := strings.ToLower(forTime.Weekday().String()[:2])
	logger = logger.WithFields(log.Fields{
		"day_of_week": weekdayName,
		"time_of_day": tod.String(),
	})

	// Little inner function that allows us to return early¸ instead of
	// writing if/else clauses.
	timeBasedStatus := func() string {
		if len(sched.DaysOfWeek) > 0 && !strings.Contains(sched.DaysOfWeek, weekdayName) {
			// There are days configured, but today is not a sleeping day.
			logger.WithField("sleep_days", sched.DaysOfWeek).Debug("today is not a sleep day")
			return ""
		}

		beforeStart := sched.TimeStart != nil && tod.IsBefore(*sched.TimeStart)
		afterEnd := sched.TimeEnd != nil && !tod.IsBefore(*sched.TimeEnd)

		localLog := logger
		if sched.TimeStart != nil {
			localLog = localLog.WithField("sleep_start", sched.TimeStart.String())
		}
		if sched.TimeEnd != nil {
			localLog = localLog.WithField("sleep_end", sched.TimeEnd.String())
		}

		if beforeStart || afterEnd {
			localLog.Debug("outside sleep time")
			return workerStatusAwake
		}

		localLog.Debug("during sleep time")
		return workerStatusAsleep
	}
	scheduledStatus := timeBasedStatus()

	logger = logger.WithFields(log.Fields{
		"scheduled_status": scheduledStatus,
		"worker_status":    worker.Status,
	})
	if scheduledStatus == worker.Status {
		logger.Debug("worker is already in scheduled status")
		return ""
	}
	return scheduledStatus
}

// SetSleepSchedule stores the given schedule as the worker's new sleep schedule and applies it.
// Updates both the Worker object itself and the Mongo database.
// Instantly requests a new status for the worker according to the schedule.
func (ss *SleepScheduler) SetSleepSchedule(worker *Worker, schedule ScheduleInfo, db *mgo.Database) error {
	schedule.DaysOfWeek = ss.cleanupDaysOfWeek(schedule.DaysOfWeek)
	schedule.NextCheck = schedule.calculateNextCheck(ss.now())

	updates := bson.M{"$set": bson.M{"sleep_schedule": schedule}}
	if err := db.C("flamenco_workers").UpdateId(worker.ID, updates); err != nil {
		return err
	}

	worker.SleepSchedule = schedule
	ss.RequestWorkerStatus(worker, db)
	return nil
}

func (ss *SleepScheduler) cleanupDaysOfWeek(daysOfWeek string) string {
	trimmed := strings.TrimSpace(daysOfWeek)
	if trimmed == "" {
		return ""
	}

	daynames := strings.Fields(trimmed)
	for idx, name := range daynames {
		daynames[idx] = strings.ToLower(strings.TrimSpace(name))[:2]
	}
	return strings.Join(daynames, " ")
}

// DeactivateSleepSchedule deactivates the worker's sleep schedule.
func (ss *SleepScheduler) DeactivateSleepSchedule(worker *Worker, db *mgo.Database) error {
	updates := bson.M{"$set": bson.M{"sleep_schedule.schedule_active": false}}
	if err := db.C("flamenco_workers").UpdateId(worker.ID, updates); err != nil {
		return err
	}

	worker.SleepSchedule.ScheduleActive = false
	return nil
}

// Return a timestamp when the next scheck for this schedule is due.
// This ignores the time of day, and just sets a time.
// The returned timestamp can be used for ScheduleInfo.NextCheck.
func (si ScheduleInfo) calculateNextCheck(forTime time.Time) *time.Time {
	calcNext := func(tod TimeOfDay) time.Time {
		nextCheck := tod.OnDate(forTime)
		if nextCheck.Before(forTime) {
			nextCheck = nextCheck.AddDate(0, 0, 1)
		}
		return nextCheck
	}

	if si.TimeStart == nil && si.TimeEnd == nil {
		next := calcNext(TimeOfDay{24, 0})
		return &next
	}

	if si.TimeStart == nil {
		next := calcNext(*si.TimeEnd)
		return &next
	}

	if si.TimeEnd == nil {
		// No end time implies midnight the next day.
		next := calcNext(TimeOfDay{24, 0})
		return &next
	}

	nextChecks := []time.Time{
		calcNext(*si.TimeStart),
		calcNext(*si.TimeEnd),
		calcNext(TimeOfDay{24, 0}),
	}
	next := earliestTime(nextChecks)
	return &next
}

func earliestTime(timestamps []time.Time) time.Time {
	earliest := timestamps[0]
	for _, timestamp := range timestamps[1:] {
		if timestamp.Before(earliest) {
			earliest = timestamp
		}
	}
	return earliest
}
