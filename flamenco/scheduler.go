package flamenco

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	auth "github.com/abbot/go-http-auth"
	log "github.com/sirupsen/logrus"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// TaskScheduler offers tasks to Workers when they ask for them.
type TaskScheduler struct {
	config   *Conf
	upstream *UpstreamConnection
	session  *mgo.Session

	/* Timestamp of the last time we kicked the task downloader because there weren't any
	 * tasks left for workers. */
	lastUpstreamCheck time.Time

	mutex *sync.Mutex
}

// CreateTaskScheduler constructs a new TaskScheduler, including private fields.
func CreateTaskScheduler(config *Conf, upstream *UpstreamConnection, session *mgo.Session) *TaskScheduler {
	return &TaskScheduler{
		config,
		upstream,
		session,
		time.Time{},
		new(sync.Mutex),
	}
}

// ScheduleTask assigns a task to a worker.
func (ts *TaskScheduler) ScheduleTask(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := ts.session.Copy()
	defer mongoSess.Close()
	db := mongoSess.DB("")
	logger := log.WithFields(log.Fields{
		"remote_addr": r.RemoteAddr,
		"worker_id":   r.Username,
	})

	// Fetch the worker's info
	projection := M{"platform": 1, "supported_task_types": 1, "address": 1, "nickname": 1, "status_requested": 1}
	worker, err := FindWorker(r.Username, projection, db)
	if err != nil {
		logger.WithError(err).Warning("ScheduleTask: Unable to find worker")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "Unable to find worker: %s", err)
		return
	}
	logger = logger.WithField("worker", worker.Identifier())
	if worker.StatusRequested != "" {
		logger = logger.WithField("status_requested", worker.StatusRequested)
	}
	worker.Seen(&r.Request, db)

	// If a status change was requested, refuse to schedule a task.
	// The worker should handle this status change first.
	if worker.StatusRequested != "" {
		logger.Warning("ScheduleTask: status change requested for this worker, refusing to give a task.")
		resp := WorkerStatus{worker.StatusRequested}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusLocked)
		encoder := json.NewEncoder(w)
		if err := encoder.Encode(resp); err != nil {
			logger.WithError(err).Warning("ScheduleTask: error encoding worker status request")
		}
		return
	}

	logger.Debug("ScheduleTask: Worker asking for a task")

	// From here on, things should be locked. This prevents multiple workers from
	// getting assigned the same task.
	//
	// TODO: In the future, this should be done smarter, for example by immediately
	// marking the task returned by ts.fetchTaskFromQueueOrManager() in the database
	// as "under consideration" for a worker. This should contain a timestamp, though,
	// so that it can be automatically ignored after a certain time, without having
	// to re-update the database.
	ts.mutex.Lock()
	defer ts.mutex.Unlock()

	var task *Task
	var wasChanged bool
	for attempt := 0; attempt < 1000; attempt++ {
		// Fetch the first available task of a supported task type.
		task = ts.fetchTaskFromQueueOrManager(w, db, worker)
		if task == nil {
			// A response has already been written to 'w'.
			return
		}

		wasChanged = ts.upstream.RefetchTask(task)
		if !wasChanged {
			break
		}

		logger.WithField("task", task.ID.Hex()).Debug("Task was changed, reexamining queue.")
	}
	if wasChanged {
		logger.Error("Infinite loop detected, tried 1000 tasks and they all changed...")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logger = logger.WithField("task_id", task.ID.Hex())
	logger.Info("ScheduleTask: assigning task to worker")

	// Update the task status to "active", pushing it as a task update to the manager too.
	task.Status = "active"
	tupdate := TaskUpdate{TaskID: task.ID, TaskStatus: task.Status}
	localUpdates := bson.M{
		"worker":           worker.Nickname,
		"worker_id":        worker.ID,
		"last_worker_ping": UtcNow(),
	}
	if err := QueueTaskUpdateWithExtra(&tupdate, db, localUpdates); err != nil {
		logger.WithError(err).Error("Unable to queue task update while assigning task")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Update the "Current task" on the Worker as well.
	worker.SetCurrentTask(task.ID, db)

	// Perform variable replacement on the task.
	ReplaceVariables(ts.config, task, worker)

	// Set it to this worker.
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(task); err != nil {
		logger.WithError(err).Warning("ScheduleTask: error encoding & sending task to worker")
		ts.unassignTaskFromWorker(task.ID, worker, err, db)
		return
	}

	logger.Info("ScheduleTask: assigned task to worker")

	// Push a task log line stating we've assigned this task to the given worker.
	// This is done here, instead of by the worker, so that it's logged even if the worker fails.
	msg := fmt.Sprintf("Manager assigned task %s to worker %s", task.ID.Hex(), worker.Identifier())
	LogTaskActivity(worker, task.ID, msg, time.Now().Format(IsoFormat)+": "+msg, db)
}

func (ts *TaskScheduler) unassignTaskFromWorker(taskID bson.ObjectId, worker *Worker, reason error, db *mgo.Database) {
	wIdent := worker.Identifier()
	logger := log.WithFields(log.Fields{
		"task_id": taskID.Hex(),
		"worker":  wIdent,
	})

	logger.Warning("unassignTaskFromWorker: un-assigning task from worker")

	tupdate := TaskUpdate{
		TaskID:     taskID,
		TaskStatus: "claimed-by-manager",
		Worker:     "-", // no longer assigned to any worker
		Activity:   fmt.Sprintf("Re-queued task after unassigning from worker %s ", wIdent),
		Log: fmt.Sprintf("%s: Manager re-queued task after there was an error sending it to worker %s:\n%s",
			time.Now().Format(IsoFormat), wIdent, reason),
	}

	if err := QueueTaskUpdate(&tupdate, db); err != nil {
		logger.WithError(err).Error("unassignTaskFromWorker: unable to update MongoDB for task just unassigned from worker")
		return
	}
}

/**
 * Fetches a task from either the queue, or if it is empty, from the manager.
 */
func (ts *TaskScheduler) fetchTaskFromQueueOrManager(
	w http.ResponseWriter, db *mgo.Database, worker *Worker) *Task {

	logFields := log.Fields{
		"worker": worker.Identifier(),
	}

	if len(worker.SupportedTaskTypes) == 0 {
		log.WithFields(logFields).Warning("TaskScheduler: worker has no supported task types")
		w.WriteHeader(http.StatusNotAcceptable)
		fmt.Fprintln(w, "You do not support any task types.")
		return nil
	}

	tasksColl := db.C("flamenco_tasks")

	// First check for any active tasks already assigned to the worker.
	alreadyAssignedTask := Task{}
	findErr := tasksColl.Find(M{
		"status":    statusActive,
		"worker_id": worker.ID,
	}).One(&alreadyAssignedTask)
	if findErr == nil {
		// We found an already-assigned task. Just return that.
		logFields["task_id"] = alreadyAssignedTask.ID.Hex()
		log.WithFields(logFields).Info("TaskScheduler: worker already had task assigned, returning that")
		return &alreadyAssignedTask
	} else if findErr != mgo.ErrNotFound {
		// Something went wrong, and it wasn't just an "not found" error (which we actually expect).
		// In this case we log the error but fall through to the regular task scheduling query.
		log.WithFields(logFields).WithError(findErr).Error("TaskScheduler: unable to query for active tasks assigned to worker")
	}

	// Perform the monster MongoDB aggregation query to schedule a task.
	result := aggregationPipelineResult{}
	pipe := tasksColl.Pipe([]M{
		// 1: Select only tasks that have a runnable status & acceptable task type.
		M{"$match": M{
			"status":    M{"$in": []string{statusQueued, statusClaimedByManager}},
			"task_type": M{"$in": worker.SupportedTaskTypes},
		}},
		// 2: Unwind the parents array, so that we can do a lookup in the next stage.
		M{"$unwind": M{
			"path": "$parents",
			"preserveNullAndEmptyArrays": true,
		}},
		// 3: Look up the parent document for each unwound task.
		// This produces 1-length "parent_doc" arrays.
		M{"$lookup": M{
			"from":         "flamenco_tasks",
			"localField":   "parents",
			"foreignField": "_id",
			"as":           "parent_doc",
		}},
		// 4: Unwind again, to turn the 1-length "parent_doc" arrays into a subdocument.
		M{"$unwind": M{
			"path": "$parent_doc",
			"preserveNullAndEmptyArrays": true,
		}},
		// 5: Group by task ID to undo the unwind, and create an array parent_statuses
		// with booleans indicating whether the parent status is "completed".
		M{"$group": M{
			"_id": "$_id",
			"parent_statuses": M{"$push": M{
				"$eq": []interface{}{
					statusCompleted,
					M{"$ifNull": []string{"$parent_doc.status", statusCompleted}}}}},
			// This allows us to keep all dynamic properties of the original task document:
			"task": M{"$first": "$$ROOT"},
		}},
		// 6: Turn the list of "parent_statuses" booleans into a single boolean
		M{"$project": M{
			"_id":               0,
			"parents_completed": M{"$allElementsTrue": []string{"$parent_statuses"}},
			"task":              1,
		}},
		// 7: Select only those tasks for which the parents have completed.
		M{"$match": M{
			"parents_completed": true,
		}},
		// 8: just keep the task info, the "parents_runnable" is no longer needed.
		M{"$project": M{"task": 1}},
		// 9: Sort by priority, with highest prio first. If prio is equal, use oldest task.
		M{"$sort": bson.D{
			{Name: "task.job_priority", Value: -1},
			{Name: "task.priority", Value: -1},
			{Name: "task._id", Value: 1},
		}},
		// 10: Only return one task.
		M{"$limit": 1},
	})

	err := pipe.One(&result)
	if err == mgo.ErrNotFound {
		log.WithFields(logFields).Debug("TaskScheduler: no more tasks available for worker")
		ts.maybeKickTaskDownloader()
		w.WriteHeader(204)
		return nil
	}
	if err != nil {
		log.WithFields(logFields).WithError(err).Error("TaskScheduler: Error fetching task for worker")
		w.WriteHeader(500)
		return nil
	}

	return result.Task
}

func (ts *TaskScheduler) maybeKickTaskDownloader() {
	dtrt := ts.config.DownloadTaskRecheckThrottle
	if dtrt < 0 || time.Now().Sub(ts.lastUpstreamCheck) <= dtrt {
		return
	}

	log.Debug("TaskScheduler: kicking task downloader")
	ts.lastUpstreamCheck = time.Now()
	ts.upstream.KickDownloader(false)
}
