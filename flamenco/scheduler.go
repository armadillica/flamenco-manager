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

// Tasks with a status in this list will be allowed to be assigned to a worker by the scheduler.
var schedulableTaskStatuses = []string{statusQueued, statusClaimedByManager, statusSoftFailed}

// TaskScheduler offers tasks to Workers when they ask for them.
type TaskScheduler struct {
	config    *Conf
	upstream  *UpstreamConnection
	session   *mgo.Session
	queue     *TaskUpdateQueue
	blacklist *WorkerBlacklist

	/* Timestamp of the last time we kicked the task downloader because there weren't any
	 * tasks left for workers. */
	lastUpstreamCheck time.Time

	mutex *sync.Mutex
}

// CreateTaskScheduler constructs a new TaskScheduler, including private fields.
func CreateTaskScheduler(config *Conf,
	upstream *UpstreamConnection,
	session *mgo.Session,
	queue *TaskUpdateQueue,
	blacklist *WorkerBlacklist,
) *TaskScheduler {
	return &TaskScheduler{
		config,
		upstream,
		session,
		queue,
		blacklist,
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
	projection := M{"hashed_secret": 0}
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

	worker.SetAwake(db)
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

		// If this is a manager-local task, we should not check with Flamenco Server.
		if task.isManagerLocalTask() {
			logger.WithField("task", task.ID.Hex()).Debug("debug task, not checking upstream")
			break
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
	if err := ts.assignTaskToWorker(task, worker, db, logger); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Perform variable replacement on the task.
	ReplaceVariables(ts.config, task, worker)

	// Send it to this worker.
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(task); err != nil {
		logger.WithError(err).Warning("ScheduleTask: error encoding & sending task to worker")
		ts.unassignTaskFromWorker(task, worker, err, db)
		return
	}

	logger.WithField("previously_failed_by_num_workers", len(task.FailedByWorkers)).Info("ScheduleTask: assigned task to worker")

	// Push a task log line stating we've assigned this task to the given worker.
	// This is done here, instead of by the worker, so that it's logged even if the worker fails.
	msg := fmt.Sprintf("Manager assigned task %s to worker %s", task.ID.Hex(), worker.Identifier())
	ts.queue.LogTaskActivity(worker, task, msg, time.Now().Format(IsoFormat)+": "+msg, db)
}

func (ts *TaskScheduler) assignTaskToWorker(task *Task, worker *Worker, db *mgo.Database, logger *log.Entry) error {
	logger.Info("assignTaskToWorker: assigning task to worker")

	// Update the task status to "active", pushing it as a task update to the Server too.
	if task.Status != statusActive {
		logger.WithFields(log.Fields{
			"old_status": task.Status,
			"new_status": statusActive,
		}).Info("assignTaskToWorker: updating task status")

		// Poke the task update queue to trigger log rotation.
		// TODO(Sybren): move the mutation of tasks to a central place so that
		//  such triggers are always handled properly.
		ts.queue.onTaskStatusMayHaveChanged(task, statusActive, db)
		task.Status = statusActive
	} else {
		logger.Info("assignTaskToWorker: task already active")
	}
	tupdate := TaskUpdate{
		TaskID:         task.ID,
		TaskStatus:     task.Status,
		isManagerLocal: task.isManagerLocalTask(),
	}
	localUpdates := bson.M{
		"$set": bson.M{
			"worker":           worker.Nickname,
			"worker_id":        worker.ID,
			"last_worker_ping": UtcNow(),
		},
	}
	if err := ts.queue.QueueTaskUpdateWithExtra(task, &tupdate, db, localUpdates); err != nil {
		logger.WithError(err).Error("Unable to queue task update while assigning task")
		return err
	}

	// Update the "Current task" on the Worker as well.
	worker.SetCurrentTask(task.ID, db)

	return nil
}

func (ts *TaskScheduler) unassignTaskFromWorker(task *Task, worker *Worker, reason error, db *mgo.Database) {
	wIdent := worker.Identifier()
	logger := log.WithFields(log.Fields{
		"task_id": task.ID.Hex(),
		"worker":  wIdent,
	})

	logger.Warning("unassignTaskFromWorker: un-assigning task from worker")

	tupdate := TaskUpdate{
		isManagerLocal: task.isManagerLocalTask(),
		TaskID:         task.ID,
		TaskStatus:     "claimed-by-manager",
		Worker:         "-", // no longer assigned to any worker
		Activity:       fmt.Sprintf("Re-queued task after unassigning from worker %s ", wIdent),
		Log: fmt.Sprintf("%s: Manager re-queued task after there was an error sending it to worker %s:\n%s",
			time.Now().Format(IsoFormat), wIdent, reason),
	}

	if err := ts.queue.QueueTaskUpdate(task, &tupdate, db); err != nil {
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
	// Note that this task type could be blacklisted, but since it's active that is unlikely.
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

	blacklist := ts.blacklist.BlacklistForWorker(worker.ID)

	// Perform the monster MongoDB aggregation query to schedule a task.
	result := aggregationPipelineResult{}
	query := []M{
		// Select only tasks that have a runnable status & acceptable task type.
		M{"$match": M{
			"status":    M{"$in": schedulableTaskStatuses},
			"task_type": M{"$in": worker.SupportedTaskTypes},
		}},
		// Filter out any task type that's blacklisted.
		M{"$match": blacklist},
		// Unwind the parents array, so that we can do a lookup in the next stage.
		M{"$unwind": M{
			"path":                       "$parents",
			"preserveNullAndEmptyArrays": true,
		}},
		// Look up the parent document for each unwound task.
		// This produces 1-length "parent_doc" arrays.
		M{"$lookup": M{
			"from":         "flamenco_tasks",
			"localField":   "parents",
			"foreignField": "_id",
			"as":           "parent_doc",
		}},
		// Unwind again, to turn the 1-length "parent_doc" arrays into a subdocument.
		M{"$unwind": M{
			"path":                       "$parent_doc",
			"preserveNullAndEmptyArrays": true,
		}},
		// Group by task ID to undo the unwind, and create an array parent_statuses
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
		// Turn the list of "parent_statuses" booleans into a single boolean
		M{"$project": M{
			"_id":               0,
			"parents_completed": M{"$allElementsTrue": []string{"$parent_statuses"}},
			"task":              1,
		}},
		// Select only those tasks for which the parents have completed.
		M{"$match": M{
			"parents_completed": true,
		}},
		// Just keep the task info, the "parents_runnable" is no longer needed.
		M{"$project": M{"task": 1}},
		// Skip any task this worker failed in the past.
		M{"$match": M{
			"task.failed_by_workers.id": M{"$ne": worker.ID},
		}},
		// Sort by priority, with highest prio first. If prio is equal, use oldest task.
		M{"$sort": bson.D{
			{Name: "task.job_priority", Value: -1},
			{Name: "task.priority", Value: -1},
			{Name: "task._id", Value: 1},
		}},
		// Only return one task.
		M{"$limit": 1},
	}

	// Just for debugging during development.
	// queryAsJSON, jsonErr := json.MarshalIndent(query, "", "    ")
	// if jsonErr != nil {
	// 	panic(jsonErr)
	// } else {
	// 	fmt.Printf("JSON-encoded query:\n%s\n", string(queryAsJSON))
	// }

	pipe := tasksColl.Pipe(query)

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

// ReturnTaskFromWorker is the HTTP interface for workers to return a specific task to the queue.
func (ts *TaskScheduler) ReturnTaskFromWorker(w http.ResponseWriter, r *auth.AuthenticatedRequest,
	db *mgo.Database, taskID bson.ObjectId) {

	worker, logFields := findWorkerForHTTP(w, r, db)
	logFields["task_id"] = taskID.Hex()
	logger := log.WithFields(logFields)

	task := Task{}
	if err := db.C("flamenco_tasks").FindId(taskID).One(&task); err != nil {
		if err == mgo.ErrNotFound {
			w.WriteHeader(http.StatusNotFound)
		} else {
			logger.WithError(err).Error("ReturnTaskFromWorker: Unable to find task")
			w.WriteHeader(http.StatusInternalServerError)
		}
		fmt.Printf("Task %s not found", taskID.Hex())
		return
	}

	logger.Info("worker is returning task to the queue")

	worker.Seen(&r.Request, db)
	if err := ts.ReturnTask(worker, logFields, db, &task, "returned by worker"); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Unable to return task: %s", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReturnTask lets a Worker return its tasks to the queue, for execution by another worker.
func (ts *TaskScheduler) ReturnTask(worker *Worker, logFields log.Fields,
	db *mgo.Database, task *Task, reasonForReturn string) error {
	logger := log.WithFields(logFields).WithField("reason", reasonForReturn)
	logger.Info("worker returns task to the queue")

	// Lock the task scheduler so that tasks don't get reassigned while we perform our checks.
	ts.mutex.Lock()
	defer ts.mutex.Unlock()

	// Get the task and check whether it's assigned to this worker at all.
	if task.WorkerID != nil && *task.WorkerID != worker.ID {
		logger.WithField("other_worker", task.WorkerID.Hex()).Info("ReturnTask: task was assigned to other worker")
		return nil
	}

	// If the task isn't active, "returning it" has no meaning.
	if task.Status != statusActive {
		logger.WithField("task_status", task.Status).Info("ReturnTask: task is not active, so not doing anything")
		return nil
	}

	// Queue the actual task update to re-queue it.
	wIdent := worker.Identifier()
	task.Status = statusClaimedByManager
	tupdate := TaskUpdate{
		isManagerLocal: task.isManagerLocalTask(),
		TaskID:         task.ID,
		TaskStatus:     task.Status,
		Worker:         "-", // no longer assigned to any worker
		Activity:       fmt.Sprintf("Re-queued task after unassigning from worker %s: %s", wIdent, reasonForReturn),
		Log: fmt.Sprintf("%s: Manager re-queued task after unassigning it from worker %s: %s",
			time.Now().Format(IsoFormat), wIdent, reasonForReturn),
	}
	if err := ts.queue.QueueTaskUpdate(task, &tupdate, db); err != nil {
		log.WithError(err).Error("unassignTaskFromWorker: unable to update MongoDB for task just unassigned from worker")
		return err
	}

	return nil
}

// WorkerMayRunTask tells the worker whether it's allowed to keep running the given task.
func (ts *TaskScheduler) WorkerMayRunTask(w http.ResponseWriter, r *auth.AuthenticatedRequest,
	db *mgo.Database, taskID bson.ObjectId) {

	worker, logFields := findWorkerForHTTP(w, r, db)
	statusRequested := ""

	logFields["task_id"] = taskID.Hex()
	if worker.StatusRequested != "" && worker.LazyStatusRequest == Immediate {
		statusRequested = worker.StatusRequested
	}
	if worker.StatusRequested != "" {
		logFields["worker_status_requested"] = worker.StatusRequested
		logFields["lazy_status_request"] = worker.LazyStatusRequest
	}
	worker.SetAwake(db)
	worker.Seen(&r.Request, db)
	log.WithFields(logFields).Debug("WorkerMayRunTask: asking if it is allowed to keep running task")

	response := MayKeepRunningResponse{
		StatusRequested: worker.StatusRequested,
	}

	func() {
		// Lock the task scheduler so that tasks don't get reassigned while we perform our checks.
		ts.mutex.Lock()
		defer ts.mutex.Unlock()

		// Get the task and check its status.
		task := Task{}
		if err := db.C("flamenco_tasks").FindId(taskID).One(&task); err != nil {
			log.WithFields(logFields).Warning("WorkerMayRunTask: unable to find task")
			response.Reason = fmt.Sprintf("unable to find task %s", taskID.Hex())
		} else if task.WorkerID != nil && *task.WorkerID != worker.ID {
			logFields["other_worker"] = task.WorkerID.Hex()
			log.WithFields(logFields).Warning("WorkerMayRunTask: task was assigned to other worker")
			response.Reason = fmt.Sprintf("task %s reassigned to another worker", taskID.Hex())
		} else if !IsRunnableTaskStatus(task.Status) {
			logFields["task_status"] = task.Status
			log.WithFields(logFields).Warning("WorkerMayRunTask: task is in not-runnable status, worker will stop")
			response.Reason = fmt.Sprintf("task %s in non-runnable status %s", taskID.Hex(), task.Status)
		} else if !workerStatusRunnable[statusRequested] {
			log.WithFields(logFields).Warning("WorkerMayRunTask: worker was requested to go to non-active status; will stop its current task")
			response.Reason = fmt.Sprintf("worker status change to %s requested", statusRequested)
		} else {
			response.MayKeepRunning = true
			WorkerPingedTask(worker.ID, taskID, "", db)
		}
	}()

	// Send the response
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(response); err != nil {
		log.WithFields(logFields).WithError(err).Warning("WorkerMayRunTask: unable to send response to worker")
		return
	}
}

// IsRunnableTaskStatus returns whether the given status is considered "runnable".
func IsRunnableTaskStatus(status string) bool {
	// 'queued', 'claimed-by-manager', and 'soft-failed' aren't considered runnable,
	// as those statuses indicate the task wasn't assigned to a Worker by the scheduler.
	runnableStatuses := map[string]bool{
		statusActive: true,
	}

	runnable, found := runnableStatuses[status]
	return runnable && found
}
