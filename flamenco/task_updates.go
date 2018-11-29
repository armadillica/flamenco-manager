// Package flamenco receives task updates from workers, queues them, and forwards them to the Flamenco Server.
package flamenco

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	auth "github.com/abbot/go-http-auth"
	log "github.com/sirupsen/logrus"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	queueMgoCollection      = "task_update_queue"
	taskQueueInspectPeriod  = 1 * time.Second
	taskQueueRetainLogLines = 10 // How many lines of logging are sent to the server.
)

// In the specific case where the Server asks us to cancel a task we know nothing about,
// we cannot look up the Job ID, which means that we cannot write to the task's log file
// on disk. As an indicator that we do not know the job ID, we use this one. Behind the
// scenes it's actually just an empty string, so it's never used as an actual job ID.
var unknownJobID bson.ObjectId

// TaskUpdatePusher pushes queued task updates to the Flamenco Server.
type TaskUpdatePusher struct {
	closable
	config   *Conf
	upstream *UpstreamConnection
	session  *mgo.Session
	queue    *TaskUpdateQueue
}

// TaskUpdateQueue queues task updates for later pushing, and writes log files to disk.
type TaskUpdateQueue struct {
	config *Conf
}

// CreateTaskUpdateQueue creates a new TaskUpdateQueue.
func CreateTaskUpdateQueue(config *Conf) *TaskUpdateQueue {
	tuq := TaskUpdateQueue{config}
	return &tuq
}

// QueueTaskUpdateFromWorker receives a task update from a worker, and queues it for sending to Flamenco Server.
func (tuq *TaskUpdateQueue) QueueTaskUpdateFromWorker(w http.ResponseWriter, r *auth.AuthenticatedRequest,
	db *mgo.Database, taskID bson.ObjectId) {

	logFields := log.Fields{
		"remote_addr": r.RemoteAddr,
		"worker_id":   r.Username,
	}

	// Get the worker
	worker, err := FindWorker(r.Username, bson.M{"address": 1, "nickname": 1}, db)
	if err != nil {
		log.WithFields(logFields).WithError(err).Warning("QueueTaskUpdate: Unable to find worker")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "Unable to find worker address: %s", err)
		return
	}
	worker.Seen(&r.Request, db)
	logFields["task_id"] = taskID.Hex()
	logFields["worker"] = worker.Identifier()

	// Parse the task JSON
	tupdate := TaskUpdate{}
	defer r.Body.Close()
	if err := DecodeJSON(w, r.Body, &tupdate, fmt.Sprintf("%s QueueTaskUpdate:", worker.Identifier())); err != nil {
		return
	}
	tupdate.TaskID = taskID
	tupdate.Worker = worker.Identifier()
	logFields["task_status"] = tupdate.TaskStatus
	log.WithFields(logFields).Info("QueueTaskUpdateFromWorker: Received task update")

	// Check that this worker is allowed to update this task.
	task := Task{}
	if err := db.C("flamenco_tasks").FindId(taskID).One(&task); err != nil {
		log.WithFields(logFields).Warning("QueueTaskUpdateFromWorker: unable to find task")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Task %s is unknown.", taskID.Hex())
		return
	}
	logFields["current_task_status"] = task.Status
	if task.WorkerID != nil {
		logFields["current_task_worker_id"] = task.WorkerID.Hex()
	}

	if task.WorkerID != nil && *task.WorkerID != worker.ID {
		log.WithFields(logFields).Warning("QueueTaskUpdateFromWorker: task update rejected, task belongs to other worker")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, "Task %s is assigned to another worker.", taskID.Hex())
		return
	}

	WorkerPingedTask(worker.ID, tupdate.TaskID, tupdate.TaskStatus, db)

	if !IsRunnableTaskStatus(task.Status) {
		// These statuses can never be overwritten by a worker.
		tupdate.TaskStatus = ""
		tupdate.Activity = ""
		log.WithFields(logFields).Debug("QueueTaskUpdateFromWorker: task has non-runnable status, ignoring new task status & activity")
	}

	tupdate.isManagerLocal = task.isManagerLocalTask()
	if err := tuq.QueueTaskUpdate(&task, &tupdate, db); err != nil {
		log.WithFields(logFields).WithError(err).Warning("QueueTaskUpdateFromWorker: unable to update task")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Unable to store update: %s\n", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// QueueTaskUpdate queues the task update, without any extra updates.
func (tuq *TaskUpdateQueue) QueueTaskUpdate(task *Task, tupdate *TaskUpdate, db *mgo.Database) error {
	return tuq.QueueTaskUpdateWithExtra(task, tupdate, db, bson.M{})
}

// QueueTaskUpdateWithExtra does the same as QueueTaskUpdate(), but with extra updates to
// the local flamenco_tasks collection.
func (tuq *TaskUpdateQueue) QueueTaskUpdateWithExtra(task *Task, tupdate *TaskUpdate, db *mgo.Database, extraUpdates bson.M) error {
	// For ensuring the ordering of updates. time.Time has nanosecond precision.
	tupdate.ReceivedOnManager = time.Now().UTC()
	tupdate.ID = bson.NewObjectId()

	// Store the log into a plain-text file for the task.
	if err := tuq.writeTaskLog(task, tupdate.Log); err != nil {
		return err
	}
	tupdate.Log = trimLogForTaskUpdate(tupdate.Log)

	// Store the update in the queue for sending to the Flamenco Server later.
	if !tupdate.isManagerLocal {
		taskUpdateQueue := db.C(queueMgoCollection)
		if err := taskUpdateQueue.Insert(tupdate); err != nil {
			return fmt.Errorf("QueueTaskUpdate: error inserting task update in queue: %s", err)
		}
	}

	// Locally apply the change to our cached version of the task too, if it is a valid transition.
	// This prevents a task being reported active on the worker from overwriting the
	// cancel-requested state we received from the Server.
	taskColl := db.C("flamenco_tasks")
	updates := extraUpdates
	updates["last_updated"] = tupdate.ReceivedOnManager

	logFields := log.Fields{
		"task_status": tupdate.TaskStatus,
		"task_id":     tupdate.TaskID.Hex(),
	}

	if tupdate.TaskStatus != "" {
		// Before blindly applying the task status, first check if the transition is valid.
		if TaskStatusTransitionValid(taskColl, tupdate.TaskID, tupdate.TaskStatus) {
			updates["status"] = tupdate.TaskStatus
		} else {
			log.WithFields(logFields).Warning("QueueTaskUpdate: not locally applying task status")
		}
	}
	if tupdate.Activity != "" {
		updates["activity"] = tupdate.Activity
	}
	if tupdate.Log != "" {
		updates["log"] = tupdate.Log
	}
	if len(updates) > 0 {
		log.WithFields(logFields).WithField("updates", updates).Debug("QueueTaskUpdate: updating task")
		if err := taskColl.UpdateId(tupdate.TaskID, bson.M{"$set": updates}); err != nil {
			if err != mgo.ErrNotFound {
				return fmt.Errorf("QueueTaskUpdate: error updating local task cache: %s", err)
			}
			log.WithFields(logFields).Warning("QueueTaskUpdate: cannot find task to update locally")
		}
	} else {
		log.WithFields(logFields).Debug("QueueTaskUpdate: nothing to do locally")
	}

	return nil
}

// LogTaskActivity creates and queues a TaskUpdate to store activity and a log line.
func (tuq *TaskUpdateQueue) LogTaskActivity(worker *Worker, task *Task, activity, logLine string, db *mgo.Database) {
	tupdate := TaskUpdate{
		TaskID:         task.ID,
		Activity:       activity,
		Log:            logLine,
		isManagerLocal: task.isManagerLocalTask(),
	}
	if err := tuq.QueueTaskUpdate(task, &tupdate, db); err != nil {
		logFields := log.Fields{
			"task_id":    task.ID.Hex(),
			log.ErrorKey: err,
		}
		if worker != nil {
			logFields["worker"] = worker.Identifier()
		}
		log.WithFields(logFields).Error("LogTaskActivity: Unable to queue task update")
	}
}

func trimLogForTaskUpdate(logText string) string {
	if logText == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(logText), "\n")
	fromLine := 0
	if len(lines) > taskQueueRetainLogLines {
		fromLine = len(lines) - taskQueueRetainLogLines
	}

	return strings.Join(lines[fromLine:], "\n") + "\n"
}

func (tuq *TaskUpdateQueue) writeTaskLog(task *Task, logText string) error {
	logger := log.WithField("task_id", task.ID.Hex())
	if task.Job == unknownJobID {
		logger.Debug("not saving log, task as unknown job ID")
		return nil
	}

	dirpath, filename := tuq.taskLogPath(task.Job, task.ID)
	filepath := path.Join(dirpath, filename)
	logger = logger.WithField("filepath", filepath)

	if err := os.MkdirAll(dirpath, 0755); err != nil {
		logger.WithError(err).Error("unable to create directory for log file")
		return err
	}

	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.WithError(err).Error("unable to open log file for append/create/write")
		return err
	}

	if n, err := file.WriteString(logText); n < len(logText) || err != nil {
		logger.WithFields(log.Fields{
			"written":      n,
			"total_length": len(logText),
			log.ErrorKey:   err,
		}).Error("could only write partial log file")
		return err
	}

	if err := file.Close(); err != nil {
		logger.WithError(err).Error("error closing log file")
		return err
	}

	logger.Debug("successfully appended to log file")
	return nil
}

// taskLogPath returns the directory and the filename suitable to write a log file.
func (tuq *TaskUpdateQueue) taskLogPath(jobID, taskID bson.ObjectId) (string, string) {
	jobHex := jobID.Hex()
	dirpath := path.Join(tuq.config.TaskLogsPath, "job-"+jobHex[:4], jobHex)
	filename := "task-" + taskID.Hex() + ".txt"
	return dirpath, filename
}

// TaskStatusTransitionValid performs a query on the database to determine the current status,
// then checks whether the new status is acceptable.
func TaskStatusTransitionValid(taskColl *mgo.Collection, taskID bson.ObjectId, newStatus string) bool {
	/* The only actual test we do is when the transition is from cancel-requested
	   to something else. If the new status is valid for cancel-requeted, we don't
	   even need to go to the database to fetch the current status. */
	if validForCancelRequested(newStatus) {
		return true
	}

	taskCurr := Task{}
	if err := taskColl.FindId(taskID).Select(bson.M{"status": 1}).One(&taskCurr); err != nil {
		log.WithFields(log.Fields{
			"task_id":    taskID.Hex(),
			"new_status": newStatus,
			log.ErrorKey: err,
		}).Warning("Unable to find task, not accepting status update")
		return false
	}

	// We already know the new status is not valid for cancel-requested.
	// All other statuses are fine, though.
	return taskCurr.Status != "cancel-requested"
}

func validForCancelRequested(newStatus string) bool {
	// Valid statuses to which a task can go after being cancel-requested
	validStatuses := map[string]bool{
		statusCanceled:  true, // the expected case
		statusFailed:    true, // it may have failed on the worker before it could be canceled
		statusCompleted: true, // it may have completed on the worker before it could be canceled
	}

	valid, found := validStatuses[newStatus]
	return valid && found
}

// CreateTaskUpdatePusher creates a new task update pusher that runs in a separate goroutine.
func CreateTaskUpdatePusher(config *Conf, upstream *UpstreamConnection, session *mgo.Session, queue *TaskUpdateQueue) *TaskUpdatePusher {
	return &TaskUpdatePusher{
		makeClosable(),
		config,
		upstream,
		session,
		queue,
	}
}

// Close closes the task update pusher by stopping all timers & goroutines.
func (pusher *TaskUpdatePusher) Close() {
	log.Info("TaskUpdatePusher: shutting down, waiting for shutdown to complete.")
	pusher.closableCloseAndWait()
	log.Info("TaskUpdatePusher: shutdown complete.")
}

// Go starts the goroutine.
func (pusher *TaskUpdatePusher) Go() {
	log.Info("TaskUpdatePusher: Starting")
	pusher.closableAdd(1)
	go func() {
		defer pusher.closableDone()

		mongoSess := pusher.session.Copy()
		defer mongoSess.Close()

		var lastPush time.Time
		db := mongoSess.DB("")
		queue := db.C(queueMgoCollection)

		// Investigate the queue periodically.
		timerChan := Timer("TaskUpdatePusherTimer",
			taskQueueInspectPeriod, 0, &pusher.closable)

		for _ = range timerChan {
			// log.Info("TaskUpdatePusher: checking task update queue")
			updateCount, err := Count(queue)
			if err != nil {
				log.WithError(err).Warning("TaskUpdatePusher: error checking queue")
				continue
			}

			timeSinceLastPush := time.Now().Sub(lastPush)
			mayRegularPush := updateCount > 0 &&
				(updateCount >= pusher.config.TaskUpdatePushMaxCount ||
					timeSinceLastPush >= pusher.config.TaskUpdatePushMaxInterval)
			mayEmptyPush := timeSinceLastPush >= pusher.config.CancelTaskFetchInterval
			if !mayRegularPush && !mayEmptyPush {
				continue
			}

			// Time to push!
			if updateCount > 0 {
				log.WithField("update_count", updateCount).Debug("TaskUpdatePusher: updates are queued")
			}
			if err := pusher.push(db); err != nil {
				log.WithError(err).Warning("TaskUpdatePusher: unable to push to upstream Flamenco Server")
				continue
			}

			// Only remember we've pushed after it was succesful.
			lastPush = time.Now()
		}
	}()
}

/**
 * Push task updates to the queue, and handle the response.
 * This response can include a list of task IDs to cancel.
 *
 * NOTE: this function assumes there is only one thread/process doing the pushing,
 * and that we can safely leave documents in the queue until they have been pushed. */
func (pusher *TaskUpdatePusher) push(db *mgo.Database) error {
	var result []TaskUpdate

	queue := db.C(queueMgoCollection)

	// Figure out what to send.
	query := queue.Find(bson.M{}).Limit(pusher.config.TaskUpdatePushMaxCount)
	if err := query.All(&result); err != nil {
		return err
	}

	logFields := log.Fields{
		"updates_to_push": len(result),
	}
	// Perform the sending.
	if len(result) > 0 {
		log.WithFields(logFields).Info("TaskUpdatePusher: pushing updates to upstream Flamenco Server")
	} else {
		log.WithFields(logFields).Debug("TaskUpdatePusher: pushing updates to upstream Flamenco Server")
	}
	response, err := pusher.upstream.SendTaskUpdates(&result)
	if err != nil {
		// TODO Sybren: implement some exponential backoff when things fail to get sent.
		return err
	}
	logFields["updates_accepted"] = len(response.HandledUpdateIds)
	if len(response.HandledUpdateIds) != len(result) {
		log.WithFields(logFields).Warning("TaskUpdatePusher: server accepted only a subset of our updates")
	}

	// If succesful, remove the accepted updates from the queue.
	/* If there is an error, don't return just yet - we also want to cancel any task
	   that needs cancelling. */
	var errUnqueue error
	if len(response.HandledUpdateIds) > 0 {
		_, errUnqueue = queue.RemoveAll(bson.M{"_id": bson.M{"$in": response.HandledUpdateIds}})
	}
	errCancel := pusher.handleIncomingCancelRequests(response.CancelTasksIds, db)

	if errUnqueue != nil {
		log.WithFields(logFields).WithError(errUnqueue).Warning(
			"TaskUpdatePusher: This is awkward; we have already sent the task updates " +
				"upstream, but now we cannot un-queue them. Expect duplicates.")
		return errUnqueue
	}

	return errCancel
}

/**
 * Handles the canceling of tasks, as mentioned in the task batch update response.
 */
func (pusher *TaskUpdatePusher) handleIncomingCancelRequests(cancelTaskIDs []bson.ObjectId, db *mgo.Database) error {
	if len(cancelTaskIDs) == 0 {
		return nil
	}

	logFields := log.Fields{
		"tasks_to_cancel": len(cancelTaskIDs),
	}
	log.WithFields(logFields).Info("TaskUpdatePusher: canceling tasks")
	tasksColl := db.C("flamenco_tasks")

	// Fetch all to-be-canceled tasks
	var tasksToCancel []Task
	err := tasksColl.Find(bson.M{
		"_id": bson.M{"$in": cancelTaskIDs},
	}).Select(bson.M{
		"_id":    1,
		"job":    1,
		"status": 1,
	}).All(&tasksToCancel)
	if err != nil {
		log.WithFields(logFields).WithError(err).Error("TaskUpdatePusher: unable to fetch tasks")
		return err
	}

	// Remember which tasks we actually have seen, so we know which ones we don't have cached.
	canceledCount := 0
	seenTasks := map[bson.ObjectId]bool{}
	goToCancelRequested := make([]bson.ObjectId, 0, len(cancelTaskIDs))

	queueTaskCancel := func(task *Task) {
		msg := "Manager cancelled task by request of Flamenco Server"
		pusher.queue.LogTaskActivity(nil, task, msg, time.Now().Format(IsoFormat)+": "+msg, db)

		tupdate := TaskUpdate{
			TaskID:     task.ID,
			TaskStatus: statusCanceled,
		}
		if updateErr := pusher.queue.QueueTaskUpdate(task, &tupdate, db); updateErr != nil {
			log.WithFields(logFields).WithFields(log.Fields{
				"task_id":    task.ID.Hex(),
				log.ErrorKey: updateErr,
			}).Error("TaskUpdatePusher: Unable to queue task update for canceled task, " +
				"expect the task to hang in cancel-requested state.")
		} else {
			canceledCount++
		}
	}

	for _, taskToCancel := range tasksToCancel {
		seenTasks[taskToCancel.ID] = true

		if taskToCancel.Status == statusActive {
			// This needs to be canceled through the worker, and thus go to cancel-requested.
			goToCancelRequested = append(goToCancelRequested, taskToCancel.ID)
		} else {
			queueTaskCancel(&taskToCancel)
		}
	}

	// Mark tasks as cancel-requested.
	updateInfo, err := tasksColl.UpdateAll(
		bson.M{"_id": bson.M{"$in": goToCancelRequested}},
		bson.M{"$set": bson.M{"status": statusCancelRequested}},
	)
	if err != nil {
		logFields["tasks_cancel_requested"] = 0
		log.WithFields(logFields).WithError(err).Warning("TaskUpdatePusher: unable to mark tasks as cancel-requested")
	} else {
		logFields["tasks_cancel_requested"] = updateInfo.Matched
		log.WithFields(logFields).WithFields(log.Fields{
			"task_ids": goToCancelRequested,
		}).Debug("TaskUpdatePusher: marked tasks as cancel-requested")
	}

	// Just push a "canceled" update to the Server about tasks we know nothing about.
	for _, taskID := range cancelTaskIDs {
		seen, _ := seenTasks[taskID]
		if seen {
			continue
		}
		log.WithFields(logFields).WithField("task_id", taskID.Hex()).Warning("TaskUpdatePusher: unknown task")
		fakeTask := Task{
			ID:  taskID,
			Job: unknownJobID,
		}
		queueTaskCancel(&fakeTask)
	}
	logFields["tasks_canceled"] = canceledCount
	log.WithFields(logFields).Info("TaskUpdatePusher: marked tasks as canceled")

	if updateInfo.Matched+canceledCount < len(cancelTaskIDs) {
		logFields["unable_to_cancel"] = len(cancelTaskIDs) - (updateInfo.Matched + canceledCount)
		log.WithFields(logFields).Warning("TaskUpdatePusher: I was unable to cancel some tasks for some reason.")
	}

	return err
}
