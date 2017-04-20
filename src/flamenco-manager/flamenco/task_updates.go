/*
 * Receives task updates from workers, queues them, and forwards them to the Flamenco Server.
 */
package flamenco

import (
	"fmt"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	auth "github.com/abbot/go-http-auth"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const queueMgoCollection = "task_update_queue"
const taskQueueInspectPeriod = 1 * time.Second

type TaskUpdatePusher struct {
	closable
	config   *Conf
	upstream *UpstreamConnection
	session  *mgo.Session
}

/**
 * Receives a task update from a worker, and queues it for sending to Flamenco Server.
 */
func QueueTaskUpdateFromWorker(w http.ResponseWriter, r *auth.AuthenticatedRequest,
	db *mgo.Database, taskID bson.ObjectId) {

	// Get the worker
	worker, err := FindWorker(r.Username, bson.M{"address": 1, "nickname": 1}, db)
	if err != nil {
		log.Warningf("QueueTaskUpdate: Unable to find worker %s at address: %s",
			r.Username, r.RemoteAddr, err)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "Unable to find worker address: %s", err)
		return
	}
	worker.Seen(&r.Request, db)
	log.Infof("QueueTaskUpdateFromWorker: Received task update for task %s from %s",
		taskID.Hex(), worker.Identifier())

	// Parse the task JSON
	tupdate := TaskUpdate{}
	defer r.Body.Close()
	if err := DecodeJson(w, r.Body, &tupdate, fmt.Sprintf("%s QueueTaskUpdate:", worker.Identifier())); err != nil {
		return
	}
	tupdate.TaskID = taskID
	tupdate.Worker = worker.Identifier()

	// Check that this worker is allowed to update this task.
	task := Task{}
	if err := db.C("flamenco_tasks").FindId(taskID).One(&task); err != nil {
		log.Warningf("%s QueueTaskUpdateFromWorker: unable to find task %s for worker %s",
			r.RemoteAddr, taskID.Hex(), worker.Identifier())
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Task %s is unknown.", taskID.Hex())
		return
	}
	if task.WorkerID != nil && *task.WorkerID != worker.ID {
		log.Warningf("%s QueueTaskUpdateFromWorker: task %s update rejected from %s (%s), task is assigned to %s",
			r.RemoteAddr, taskID.Hex(), worker.ID.Hex(), worker.Identifier(), task.WorkerID.Hex())
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, "Task %s is assigned to another worker.", taskID.Hex())
		return
	}

	// Only set the task's worker.ID if it's not already set to the current worker.
	var setWorkerID *bson.ObjectId
	if task.WorkerID == nil {
		setWorkerID = &worker.ID
	}
	WorkerPingedTask(setWorkerID, tupdate.TaskID, db)

	if err := QueueTaskUpdate(&tupdate, db); err != nil {
		log.Warningf("%s: %s", worker.Identifier(), err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Unable to store update: %s\n", err)
		return
	}

	w.WriteHeader(204)
}

func QueueTaskUpdate(tupdate *TaskUpdate, db *mgo.Database) error {
	return QueueTaskUpdateWithExtra(tupdate, db, bson.M{})
}

/* Same as QueueTaskUpdate(), but with extra updates to be performed on the local flamenco_tasks
 * collection.
 */
func QueueTaskUpdateWithExtra(tupdate *TaskUpdate, db *mgo.Database, extraUpdates bson.M) error {
	// For ensuring the ordering of updates. time.Time has nanosecond precision.
	tupdate.ReceivedOnManager = time.Now().UTC()
	tupdate.ID = bson.NewObjectId()

	// Store the update in the queue for sending to the Flamenco Server later.
	taskUpdateQueue := db.C(queueMgoCollection)
	if err := taskUpdateQueue.Insert(&tupdate); err != nil {
		return fmt.Errorf("QueueTaskUpdate: error inserting task update in queue: %s", err)
	}

	// Locally apply the change to our cached version of the task too, if it is a valid transition.
	// This prevents a task being reported active on the worker from overwriting the
	// cancel-requested state we received from the Server.
	taskColl := db.C("flamenco_tasks")
	updates := extraUpdates
	updates["last_updated"] = tupdate.ReceivedOnManager

	if tupdate.TaskStatus != "" {
		// Before blindly applying the task status, first check if the transition is valid.
		if TaskStatusTransitionValid(taskColl, tupdate.TaskID, tupdate.TaskStatus) {
			updates["status"] = tupdate.TaskStatus
		} else {
			log.Warningf("QueueTaskUpdate: not locally applying status=%s for %s",
				tupdate.TaskStatus, tupdate.TaskID.Hex())
		}
	}
	if tupdate.Activity != "" {
		updates["activity"] = tupdate.Activity
	}
	if len(updates) > 0 {
		log.Debugf("QueueTaskUpdate: applying update %s to task %s", updates, tupdate.TaskID.Hex())
		if err := taskColl.UpdateId(tupdate.TaskID, bson.M{"$set": updates}); err != nil {
			if err != mgo.ErrNotFound {
				return fmt.Errorf("QueueTaskUpdate: error updating local task cache: %s", err)
			} else {
				log.Warningf("QueueTaskUpdate: cannot find task %s to update locally", tupdate.TaskID.Hex())
			}
		}
	} else {
		log.Debugf("QueueTaskUpdate: nothing to do locally for task %s", tupdate.TaskID.Hex())
	}

	return nil
}

/**
 * Performs a query on the database to determine the current status, then checks whether
 * the new status is acceptable.
 */
func TaskStatusTransitionValid(taskColl *mgo.Collection, taskID bson.ObjectId, newStatus string) bool {
	/* The only actual test we do is when the transition is from cancel-requested
	   to something else. If the new status is valid for cancel-requeted, we don't
	   even need to go to the database to fetch the current status. */
	if ValidForCancelRequested(newStatus) {
		return true
	}

	taskCurr := Task{}
	if err := taskColl.FindId(taskID).Select(bson.M{"status": 1}).One(&taskCurr); err != nil {
		log.Warningf("Unable to find task %s - not accepting status update to %s", err, newStatus)
		return false
	}

	// We already know the new status is not valid for cancel-requested.
	// All other statuses are fine, though.
	return taskCurr.Status != "cancel-requested"
}

func ValidForCancelRequested(newStatus string) bool {
	// Valid statuses to which a task can go after being cancel-requested
	validStatuses := map[string]bool{
		statusCanceled:  true, // the expected case
		statusFailed:    true, // it may have failed on the worker before it could be canceled
		statusCompleted: true, // it may have completed on the worker before it could be canceled
	}

	valid, found := validStatuses[newStatus]
	return valid && found
}

func CreateTaskUpdatePusher(config *Conf, upstream *UpstreamConnection, session *mgo.Session) *TaskUpdatePusher {
	return &TaskUpdatePusher{
		makeClosable(),
		config,
		upstream,
		session,
	}
}

/**
 * Closes the task update pusher by stopping all timers & goroutines.
 */
func (self *TaskUpdatePusher) Close() {
	log.Info("TaskUpdatePusher: shutting down, waiting for shutdown to complete.")
	self.closableCloseAndWait()
	log.Info("TaskUpdatePusher: shutdown complete.")
}

func (self *TaskUpdatePusher) Go() {
	log.Info("TaskUpdatePusher: Starting")
	self.closableAdd(1)
	go func() {
		defer self.closableDone()

		mongoSess := self.session.Copy()
		defer mongoSess.Close()

		var lastPush time.Time
		db := mongoSess.DB("")
		queue := db.C(queueMgoCollection)

		// Investigate the queue periodically.
		timerChan := Timer("TaskUpdatePusherTimer",
			taskQueueInspectPeriod, 0, &self.closable)

		for _ = range timerChan {
			// log.Info("TaskUpdatePusher: checking task update queue")
			updateCount, err := Count(queue)
			if err != nil {
				log.Warningf("TaskUpdatePusher: ERROR checking queue: %s", err)
				continue
			}

			timeSinceLastPush := time.Now().Sub(lastPush)
			mayRegularPush := updateCount > 0 &&
				(updateCount >= self.config.TaskUpdatePushMaxCount ||
					timeSinceLastPush >= self.config.TaskUpdatePushMaxInterval)
			mayEmptyPush := timeSinceLastPush >= self.config.CancelTaskFetchInterval
			if !mayRegularPush && !mayEmptyPush {
				continue
			}

			// Time to push!
			if updateCount > 0 {
				log.Debugf("TaskUpdatePusher: %d updates are queued", updateCount)
			}
			if err := self.push(db); err != nil {
				log.Warning("TaskUpdatePusher: unable to push to upstream Flamenco Server: ", err)
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
func (self *TaskUpdatePusher) push(db *mgo.Database) error {
	var result []TaskUpdate

	queue := db.C(queueMgoCollection)

	// Figure out what to send.
	query := queue.Find(bson.M{}).Limit(self.config.TaskUpdatePushMaxCount)
	if err := query.All(&result); err != nil {
		return err
	}

	// Perform the sending.
	if len(result) > 0 {
		log.Infof("TaskUpdatePusher: pushing %d updates to upstream Flamenco Server", len(result))
	} else {
		log.Debugf("TaskUpdatePusher: pushing %d updates to upstream Flamenco Server", len(result))
	}
	response, err := self.upstream.SendTaskUpdates(&result)
	if err != nil {
		// TODO Sybren: implement some exponential backoff when things fail to get sent.
		return err
	}

	if len(response.HandledUpdateIds) != len(result) {
		log.Warningf("TaskUpdatePusher: server accepted %d of %d items.",
			len(response.HandledUpdateIds), len(result))
	}

	// If succesful, remove the accepted updates from the queue.
	/* If there is an error, don't return just yet - we also want to cancel any task
	   that needs cancelling. */
	var errUnqueue error
	if len(response.HandledUpdateIds) > 0 {
		_, errUnqueue = queue.RemoveAll(bson.M{"_id": bson.M{"$in": response.HandledUpdateIds}})
	}
	errCancel := self.handleIncomingCancelRequests(response.CancelTasksIds, db)

	if errUnqueue != nil {
		log.Warningf("TaskUpdatePusher: This is awkward; we have already sent the task updates "+
			"upstream, but now we cannot un-queue them. Expect duplicates: %s", err)
		return errUnqueue
	}

	return errCancel
}

/**
 * Handles the canceling of tasks, as mentioned in the task batch update response.
 */
func (self *TaskUpdatePusher) handleIncomingCancelRequests(cancelTaskIDs []bson.ObjectId, db *mgo.Database) error {
	if len(cancelTaskIDs) == 0 {
		return nil
	}

	log.Infof("TaskUpdatePusher: canceling %d tasks", len(cancelTaskIDs))
	tasksColl := db.C("flamenco_tasks")

	// Fetch all to-be-canceled tasks
	var tasksToCancel []Task
	err := tasksColl.Find(bson.M{
		"_id": bson.M{"$in": cancelTaskIDs},
	}).Select(bson.M{
		"_id":    1,
		"status": 1,
	}).All(&tasksToCancel)
	if err != nil {
		log.Warningf("TaskUpdatePusher: ERROR unable to fetch tasks: %s", err)
		return err
	}

	// Remember which tasks we actually have seen, so we know which ones we don't have cached.
	canceledCount := 0
	seenTasks := map[bson.ObjectId]bool{}
	goToCancelRequested := make([]bson.ObjectId, 0, len(cancelTaskIDs))

	queueTaskCancel := func(taskID bson.ObjectId) {
		tupdate := TaskUpdate{
			TaskID:     taskID,
			TaskStatus: statusCanceled,
		}
		if err := QueueTaskUpdate(&tupdate, db); err != nil {
			log.Warningf("TaskUpdatePusher: Unable to queue task update for canceled task %s, "+
				"expect the task to hang in cancel-requested state.", taskID)
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
			queueTaskCancel(taskToCancel.ID)
		}
	}

	// Mark tasks as cancel-requested.
	updateInfo, err := tasksColl.UpdateAll(
		bson.M{"_id": bson.M{"$in": goToCancelRequested}},
		bson.M{"$set": bson.M{"status": statusCancelRequested}},
	)
	if err != nil {
		log.Warningf("TaskUpdatePusher: unable to mark tasks as cancel-requested: %s", err)
	} else {
		log.Infof("TaskUpdatePusher: marked %d tasks as cancel-requested: %s",
			updateInfo.Matched, goToCancelRequested)
	}

	// Just push a "canceled" update to the Server about tasks we know nothing about.
	for _, taskID := range cancelTaskIDs {
		seen, _ := seenTasks[taskID]
		if seen {
			continue
		}
		log.Warningf("TaskUpdatePusher: unknown task: %s", taskID.Hex())
		queueTaskCancel(taskID)
	}

	log.Infof("TaskUpdatePusher: marked %d tasks as canceled", canceledCount)

	if updateInfo.Matched+canceledCount < len(cancelTaskIDs) {
		log.Warningf("TaskUpdatePusher: I was unable to cancel %d tasks for some reason.",
			len(cancelTaskIDs)-(updateInfo.Matched+canceledCount))
	}

	return err
}

func LogTaskActivity(worker *Worker, taskID bson.ObjectId, activity, logLine string, db *mgo.Database) {
	tupdate := TaskUpdate{
		TaskID:   taskID,
		Activity: activity,
		Log:      logLine,
	}
	if err := QueueTaskUpdate(&tupdate, db); err != nil {
		log.Errorf("LogTaskActivity: Unable to queue task(%s) update for worker %s: %s",
			taskID.Hex(), worker.Identifier(), err)
	}
}
