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
}

// CreateSleepScheduler creates a new SleepScheduler.
func CreateSleepScheduler(session *mgo.Session) *SleepScheduler {
	return &SleepScheduler{
		makeClosable(),
		session,
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
	iter := coll.Find(bson.M{"sleep_schedule.schedule_active": true}).Iter()
	worker := Worker{}

	for iter.Next(&worker) {
		ss.RequestWorkerStatus(&worker, db)
	}
	if err := iter.Close(); err != nil {
		log.WithError(err).Error("SleepScheduler: error querying MongoDB")
	}
}

// RequestWorkerStatus sets worker.StatusRequested if the scheduler demands a status change.
func (ss *SleepScheduler) RequestWorkerStatus(worker *Worker, db *mgo.Database) {
	scheduled := ss.scheduledWorkerStatus(worker, time.Now())
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
	if err := worker.RequestStatusChange(scheduled, db); err != nil {
		logger.WithError(err).Error("unable to store status change in database")
	}
}

// scheduledWorkerStatus returns the expected worker status at the given timestamp.
// Returns an empty string when the schedule is inactive or the worker already has
// the appropriate status.
func (ss *SleepScheduler) scheduledWorkerStatus(worker *Worker, forTime time.Time) string {
	now := MakeTimeOfDay(forTime)
	logger := log.WithField("worker", worker.Identifier())
	sched := worker.SleepSchedule
	if !sched.ScheduleActive {
		logger.Debug("worker has disabled sleep schedule")
		return ""
	}

	weekdayName := strings.ToLower(forTime.Weekday().String()[:2])
	logger = logger.WithFields(log.Fields{
		"day_of_week": weekdayName,
		"time_of_day": now.String(),
	})

	// Little inner function that allows us to return early¸ instead of
	// writing if/else clauses.
	timeBasedStatus := func() string {
		if len(sched.DaysOfWeek) > 0 && !strings.Contains(sched.DaysOfWeek, weekdayName) {
			// There are days configured, but today is not a sleeping day.
			logger.WithField("sleep_days", sched.DaysOfWeek).Debug("today is not a sleep day")
			return workerStatusAwake
		}

		beforeStart := sched.TimeStart != nil && now.IsBefore(*sched.TimeStart)
		afterEnd := sched.TimeEnd != nil && now.IsAfter(*sched.TimeEnd)

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

// SetSleepSchedule stores the given schedule as the worker's new sleep schedule.
// Updates both the Worker object itself and the Mongo database.
func (ss *SleepScheduler) SetSleepSchedule(worker *Worker, schedule ScheduleInfo, db *mgo.Database) error {
	schedule.DaysOfWeek = ss.cleanupDaysOfWeek(schedule.DaysOfWeek)

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
