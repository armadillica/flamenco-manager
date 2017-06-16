package flamenco

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	auth "github.com/abbot/go-http-auth"

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

	// Fetch the worker's info
	projection := bson.M{"platform": 1, "supported_task_types": 1, "address": 1, "nickname": 1}
	worker, err := FindWorker(r.Username, projection, db)
	if err != nil {
		log.Warningf("ScheduleTask: Unable to find worker, requested from %s: %s", r.RemoteAddr, err)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "Unable to find worker: %s", err)
		return
	}
	worker.Seen(&r.Request, db)
	log.Debugf("ScheduleTask: Worker %s asking for a task", worker.Identifier())

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

		log.Debugf("Task %s was changed, reexamining queue.", task.ID.Hex())
	}
	if wasChanged {
		log.Errorf("Infinite loop detected, tried 1000 tasks and they all changed...")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Infof("ScheduleTask: assigning task %s to worker %s",
		task.ID.Hex(), worker.Identifier())

	// Update the task status to "active", pushing it as a task update to the manager too.
	task.Status = "active"
	tupdate := TaskUpdate{TaskID: task.ID, TaskStatus: task.Status}
	localUpdates := bson.M{
		"worker":           worker.Nickname,
		"worker_id":        worker.ID,
		"last_worker_ping": UtcNow(),
	}
	if err := QueueTaskUpdateWithExtra(&tupdate, db, localUpdates); err != nil {
		log.Errorf("Unable to queue task update while assigning task %s to worker %s: %s",
			task.ID.Hex(), worker.Identifier(), err)
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
		log.Warningf("ScheduleTask: error encoding & sending task to worker: %s", err)
		ts.unassignTaskFromWorker(task.ID, worker, err, db)
		return
	}

	log.Infof("ScheduleTask: assigned task %s to worker %s",
		task.ID.Hex(), worker.Identifier())

	// Push a task log line stating we've assigned this task to the given worker.
	// This is done here, instead of by the worker, so that it's logged even if the worker fails.
	msg := fmt.Sprintf("Manager assigned task %s to worker %s", task.ID.Hex(), worker.Identifier())
	LogTaskActivity(worker, task.ID, msg, time.Now().Format(IsoFormat)+": "+msg, db)
}

func (ts *TaskScheduler) unassignTaskFromWorker(taskID bson.ObjectId, worker *Worker, reason error, db *mgo.Database) {
	wIdent := worker.Identifier()

	log.Warningf("unassignTaskFromWorker: un-assigning task %s from worker %s", taskID.Hex(), wIdent)

	tupdate := TaskUpdate{
		TaskID:     taskID,
		TaskStatus: "claimed-by-manager",
		Worker:     "-", // no longer assigned to any worker
		Activity:   fmt.Sprintf("Re-queued task after unassigning from worker %s ", wIdent),
		Log: fmt.Sprintf("%s: Manager re-queued task after there was an error sending it to worker %s:\n%s",
			time.Now().Format(IsoFormat), wIdent, reason),
	}

	if err := QueueTaskUpdate(&tupdate, db); err != nil {
		log.Errorf("unassignTaskFromWorker: unable to update task %s unassigned from worker %s in MongoDB: %s",
			taskID.Hex(), wIdent, err)
		return
	}
}

/**
 * Fetches a task from either the queue, or if it is empty, from the manager.
 */
func (ts *TaskScheduler) fetchTaskFromQueueOrManager(
	w http.ResponseWriter, db *mgo.Database, worker *Worker) *Task {

	if len(worker.SupportedTaskTypes) == 0 {
		log.Warningf("TaskScheduler: worker %s has no supported task types.", worker.Identifier())
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
		log.Infof("TaskScheduler: worker %s already had task %s assigned, returning that.",
			worker.Identifier(), alreadyAssignedTask.ID)
		return &alreadyAssignedTask
	} else if findErr != mgo.ErrNotFound {
		// Something went wrong, and it wasn't just an "not found" error (which we actually expect).
		// In this case we log the error but fall through to the regular task scheduling query.
		log.Errorf("TaskScheduler: unable to query for active tasks assigned to worker %s: %s",
			worker.Identifier(), findErr)
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
		log.Debugf("TaskScheduler: no more tasks available for %s", worker.Identifier())
		ts.maybeKickTaskDownloader()
		w.WriteHeader(204)
		return nil
	}
	if err != nil {
		log.Errorf("TaskScheduler: Error fetching task for %s: %s", worker.Identifier(), err)
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
