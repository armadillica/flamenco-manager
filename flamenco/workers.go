package flamenco

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	auth "github.com/abbot/go-http-auth"
	log "github.com/sirupsen/logrus"

	"golang.org/x/crypto/bcrypt"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// Set of worker statuses that can be requested with RequestStatusChange()
var workerStatusRequestable = map[string]bool{
	workerStatusAsleep:   true,
	workerStatusAwake:    true,
	workerStatusShutdown: true,
}

// Set of worker statuses that allow the worker to keep running tasks.
var workerStatusRunnable = map[string]bool{
	workerStatusAwake:   true,
	workerStatusTesting: true,
	"":                  true, // no status change was requested
}

// Identifier returns the worker's address, with the nickname in parentheses (if set).
//
// Make sure that you include the nickname in the projection when you fetch
// the worker from MongoDB.
func (worker *Worker) Identifier() string {
	if len(worker.Nickname) > 0 {
		return fmt.Sprintf("%s (%s)", worker.Address, worker.Nickname)
	}
	return worker.Address
}

// SetStatus sets the worker's status, and updates the database too.
// Use SetAwake() instead of calling this function with status="awake".
func (worker *Worker) SetStatus(status string, db *mgo.Database) error {
	log.WithFields(log.Fields{
		"worker":     worker.Identifier(),
		"old_status": worker.Status,
		"new_status": status,
	}).Debug("setting worker status")

	worker.Status = status
	updates := M{"status": status}
	return db.C("flamenco_workers").UpdateId(worker.ID, M{"$set": updates})
}

// SetAwake sets the worker status to Awake, but only if's not already awake or testing.
func (worker *Worker) SetAwake(db *mgo.Database) error {
	if worker.Status == workerStatusAwake || worker.Status == workerStatusTesting {
		return nil
	}

	return worker.SetStatus(workerStatusAwake, db)
}

// SetCurrentTask sets the worker's current task, and updates the database too.
func (worker *Worker) SetCurrentTask(taskID bson.ObjectId, db *mgo.Database) error {
	worker.CurrentTask = &taskID
	updates := M{"current_task": taskID}
	return db.C("flamenco_workers").UpdateId(worker.ID, M{"$set": updates})
}

// RequestStatusChange stores the new requested status in MongoDB, so that it gets picked up
// by the worker the next time it asks for it.
func (worker *Worker) RequestStatusChange(newStatus string, db *mgo.Database) error {
	if !workerStatusRequestable[newStatus] {
		return fmt.Errorf("RequestStatusChange(%q): status cannot be requested", newStatus)
	}

	worker.StatusRequested = newStatus
	updates := M{"status_requested": newStatus}
	return db.C("flamenco_workers").UpdateId(worker.ID, M{"$set": updates})
}

// AckStatusChange acknowledges the requested status change by moving it to the actual status.
// Only the "shutdown" status should not be acknowledged, but just result in a signoff and thus
// directly go to "offline" state.
func (worker *Worker) AckStatusChange(newStatus string, db *mgo.Database) error {
	logger := log.WithFields(log.Fields{
		"old_status": worker.Status,
		"ack_status": newStatus,
		"worker":     worker.Identifier(),
	})

	if newStatus == workerStatusShutdown {
		logger.Warning("worker tries to acknowledge a status that should not be acknowledged; ignoring")
		return nil
	}
	logger.Debug("worker ACKd status change")

	worker.Status = newStatus
	worker.StatusRequested = ""

	return db.C("flamenco_workers").UpdateId(worker.ID, M{
		"$set":   M{"status": newStatus},
		"$unset": M{"status_requested": true},
	})
}

// AckTimeout acknowledges the timeout and just sets the worker to "offline".
func (worker *Worker) AckTimeout(db *mgo.Database) error {
	if worker.Status != workerStatusTimeout {
		return fmt.Errorf("Worker status is not %q", workerStatusTimeout)
	}
	return worker.AckStatusChange(workerStatusOffline, db)
}

// Timeout marks the worker as timed out.
func (worker *Worker) Timeout(db *mgo.Database) {
	logFields := log.Fields{
		"worker":    worker.Identifier(),
		"worker_id": worker.ID.Hex(),
	}
	log.WithFields(logFields).Warning("Worker timed out")

	err := worker.SetStatus(workerStatusTimeout, db)
	if err != nil {
		log.WithFields(logFields).WithError(err).Error("Unable to set worker status")
	}
}

// TimeoutOnTask marks the worker as timed out on a given task.
// The task is just used for logging.
func (worker *Worker) TimeoutOnTask(task *Task, db *mgo.Database) {
	logFields := log.Fields{
		"worker":    worker.Identifier(),
		"worker_id": worker.ID.Hex(),
		"task_id":   task.ID.Hex(),
	}
	log.WithFields(logFields).Warning("Worker timed out on task")

	err := worker.SetStatus(workerStatusTimeout, db)
	if err != nil {
		log.WithFields(logFields).WithError(err).Error("Unable to set worker status")
	}
}

// Seen registers that we have seen this worker at a certain address and with certain software.
func (worker *Worker) Seen(r *http.Request, db *mgo.Database) {
	if err := worker.SeenEx(r, db, bson.M{}, bson.M{}); err != nil {
		log.WithFields(log.Fields{
			"worker_id":  worker.ID,
			"worker":     worker.Identifier(),
			log.ErrorKey: err,
		}).Error("Worker.Seen: unable to update worker")
	}
}

// SeenEx is same as Seen(), but allows for extra updates on the worker in the database, and returns err
func (worker *Worker) SeenEx(r *http.Request, db *mgo.Database, set bson.M, unset bson.M) error {
	worker.LastActivity = UtcNow()

	logFields := log.Fields{
		"remote_addr": r.RemoteAddr,
	}

	set["last_activity"] = worker.LastActivity

	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.WithFields(logFields).WithError(err).Warning("Unable to split remote address into host/port; using the whole thing instead")
		remoteHost = r.RemoteAddr
	}
	if worker.Address != remoteHost {
		worker.Address = remoteHost
		set["address"] = remoteHost
	}

	var userAgent string = r.Header.Get("User-Agent")
	if worker.Software != userAgent {
		set["software"] = userAgent
	}

	updates := bson.M{"$set": set}
	if len(unset) > 0 {
		updates["$unset"] = unset
	}
	log.WithFields(logFields).WithField("updates", updates).Debug("Updating worker")

	return db.C("flamenco_workers").UpdateId(worker.ID, updates)
}

// RegisterWorker creates a new Worker in the DB, based on the WorkerRegistration document received.
func RegisterWorker(w http.ResponseWriter, r *http.Request, db *mgo.Database) {
	var err error

	logFields := log.Fields{
		"remote_addr": r.RemoteAddr,
	}
	log.WithFields(logFields).Debug("Worker registering")

	// Parse the given worker information.
	winfo := WorkerRegistration{}
	if err = DecodeJSON(w, r.Body, &winfo, fmt.Sprintf("%s RegisterWorker:", r.RemoteAddr)); err != nil {
		return
	}

	// Store it in MongoDB after hashing the password and assigning an ID.
	worker := Worker{}
	worker.Secret = winfo.Secret
	worker.Platform = winfo.Platform
	worker.SupportedTaskTypes = winfo.SupportedTaskTypes
	worker.Nickname = winfo.Nickname
	worker.Address = r.RemoteAddr
	logFields["worker"] = worker.Identifier()

	if err = StoreNewWorker(&worker, db); err != nil {
		log.WithFields(logFields).WithError(err).Error("RegisterWorker: Unable to store worker")

		w.WriteHeader(500)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "Unable to store worker")

		return
	}
	logFields["worker_id"] = worker.ID.Hex()
	log.WithFields(logFields).Info("Worker registered")

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(worker); err != nil {
		log.WithFields(logFields).WithError(err).Error("RegisterWorker: unable to send registration response to worker")
		return
	}
}

// StoreNewWorker saves the given worker in the database.
func StoreNewWorker(winfo *Worker, db *mgo.Database) error {
	var err error

	// Store it in MongoDB after hashing the password and assigning an ID.
	winfo.ID = bson.NewObjectId()
	winfo.HashedSecret, err = bcrypt.GenerateFromPassword([]byte(winfo.Secret), bcrypt.MinCost)
	if err != nil {
		log.WithError(err).Error("Unable to hash password")
		return err
	}

	workersColl := db.C("flamenco_workers")
	if err = workersColl.Insert(winfo); err != nil {
		log.WithError(err).Error("Unable to insert worker in DB")
		return err
	}

	return nil
}

// forgetWorker actually erases the worker from the database.
// References to this worker are not erased, so will become invalid.
func forgetWorker(winfo *Worker, db *mgo.Database) error {
	log.WithFields(log.Fields{
		"worker": winfo.Identifier(),
		"id":     winfo.ID.Hex(),
	}).Warning("forgetting worker; erasing worker from database")

	workersColl := db.C("flamenco_workers")
	return workersColl.Remove(bson.M{"_id": winfo.ID})
}

// WorkerSecret returns the hashed secret of the worker.
func WorkerSecret(user string, db *mgo.Database) string {
	projection := bson.M{"hashed_secret": 1}
	worker, err := FindWorker(user, projection, db)

	if err != nil {
		log.WithError(err).Warning("Error fetching hashed password")
		return ""
	}

	return string(worker.HashedSecret)
}

// FindWorker returns the worker given its ID in string form.
func FindWorker(workerID string, projection interface{}, db *mgo.Database) (*Worker, error) {
	worker := Worker{}

	if !bson.IsObjectIdHex(workerID) {
		return &worker, errors.New("Invalid ObjectID")
	}
	coll := db.C("flamenco_workers")
	err := coll.FindId(bson.ObjectIdHex(workerID)).Select(projection).One(&worker)

	return &worker, err
}

// FindWorkerByID returns the entire worker, no projections.
func FindWorkerByID(workerID bson.ObjectId, db *mgo.Database) (*Worker, error) {
	worker := Worker{}
	err := db.C("flamenco_workers").FindId(workerID).One(&worker)
	return &worker, err
}

func findWorkerForHTTP(w http.ResponseWriter, r *auth.AuthenticatedRequest, db *mgo.Database) (*Worker, log.Fields) {
	logFields := log.Fields{
		"remote_addr": r.RemoteAddr,
		"worker_id":   r.Username,
	}
	logger := log.WithFields(logFields)

	// Get the worker
	worker, err := FindWorker(r.Username, M{}, db)
	if err != nil {
		logger.WithField("url", r.URL).WithError(err).Warning("unable to find worker")
		w.WriteHeader(http.StatusForbidden)
		return nil, nil
	}
	logFields["worker"] = worker.Identifier()

	return worker, logFields
}

// WorkerCount returns the number of registered workers.
func WorkerCount(db *mgo.Database) int {
	count, err := Count(db.C("flamenco_workers"))
	if err != nil {
		return -1
	}
	return count
}

// WorkerMayRunTask tells the worker whether it's allowed to keep running the given task.
func WorkerMayRunTask(w http.ResponseWriter, r *auth.AuthenticatedRequest,
	db *mgo.Database, taskID bson.ObjectId) {

	worker, logFields := findWorkerForHTTP(w, r, db)
	logFields["task_id"] = taskID.Hex()

	if worker.StatusRequested != "" {
		logFields["worker_status_requested"] = worker.StatusRequested
	}
	worker.SetAwake(db)
	worker.Seen(&r.Request, db)
	log.WithFields(logFields).Debug("WorkerMayRunTask: asking if it is allowed to keep running task")

	response := MayKeepRunningResponse{
		StatusRequested: worker.StatusRequested,
	}

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
	} else if !workerStatusRunnable[worker.StatusRequested] {
		log.WithFields(logFields).Warning("WorkerMayRunTask: worker was requested to go to non-active status; will stop its current task")
		response.Reason = fmt.Sprintf("worker status change to %s requested", worker.StatusRequested)
	} else {
		response.MayKeepRunning = true
		WorkerPingedTask(worker.ID, taskID, "", db)
	}

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
	runnableStatuses := map[string]bool{
		"active":             true,
		"claimed-by-manager": true,
		"queued":             true,
	}

	runnable, found := runnableStatuses[status]
	return runnable && found
}

// WorkerPingedTask marks the task as pinged by the worker.
// If worker_id is not nil, sets the worker_id field of the task. Otherwise doesn't
// touch that field and only updates last_worker_ping.
func WorkerPingedTask(workerID bson.ObjectId, taskID bson.ObjectId, taskStatus string, db *mgo.Database) {
	tasksColl := db.C("flamenco_tasks")
	workersColl := db.C("flamenco_workers")
	logFields := log.Fields{
		"worker_id":   workerID.Hex(),
		"task_id":     taskID.Hex(),
		"task_status": taskStatus,
	}

	now := UtcNow()
	updates := bson.M{
		"last_worker_ping": now,
		"worker_id":        workerID,
	}

	log.WithFields(logFields).WithField("updates", updates).Debug("WorkerPingedTask: updating task")
	if err := tasksColl.UpdateId(taskID, bson.M{"$set": updates}); err != nil {
		log.WithFields(logFields).WithError(err).Error("WorkerPingedTask: unable to update last_worker_ping on task")
		return
	}

	// Also update this worker to reflect the last time it pinged a task.
	updates = bson.M{
		"current_task_updated": now,
	}
	if len(taskStatus) > 0 {
		updates["current_task_status"] = taskStatus
	}
	logFields["updates"] = updates
	log.WithFields(logFields).Debug("WorkerPingedTask: updating worker")
	if err := workersColl.UpdateId(workerID, bson.M{"$set": updates}); err != nil {
		log.WithFields(logFields).WithError(err).Error("WorkerPingedTask: unable to update current_task_updated on task")
		return
	}
}

// WorkerSignOff re-queues all active tasks (should be only one) that are assigned to this worker.
func WorkerSignOff(w http.ResponseWriter, r *auth.AuthenticatedRequest, db *mgo.Database) {
	worker, logFields := findWorkerForHTTP(w, r, db)
	workerIdent := worker.Identifier()

	log.WithFields(logFields).Warning("Worker signing off")

	// Update the tasks assigned to the worker.
	var tasks []Task
	query := bson.M{
		"worker_id": worker.ID,
		"status":    "active",
	}
	sentHeader := false
	if err := db.C("flamenco_tasks").Find(query).All(&tasks); err != nil {
		log.WithFields(logFields).WithError(err).Warning("WorkerSignOff: unable to find active tasks of worker")
		w.WriteHeader(http.StatusInternalServerError)
		sentHeader = true
	} else {
		tupdate := TaskUpdate{
			TaskStatus: "claimed-by-manager",
			Worker:     "-", // no longer assigned to any worker
			Activity:   fmt.Sprintf("Re-queued task after worker %s signed off", workerIdent),
			Log: fmt.Sprintf("%s: Manager re-queued task after worker %s signed off",
				time.Now(), workerIdent),
		}

		for _, task := range tasks {
			tupdate.TaskID = task.ID
			tupdate.isManagerLocal = task.isManagerLocalTask()
			if err := QueueTaskUpdate(&tupdate, db); err != nil {
				if !sentHeader {
					w.WriteHeader(http.StatusInternalServerError)
					sentHeader = true
				}
				fmt.Fprintf(w, "Error updating task %s: %s\n", task.ID.Hex(), err)
				log.WithFields(logFields).WithError(err).Error("WorkerSignOff: unable to update task for worker")
			}
		}
	}

	// Update the worker itself, to show it's down in the DB too.
	if worker.Status == workerStatusAsleep {
		// Make sure that the worker remains asleep, even after signing on again.
		defer worker.RequestStatusChange(workerStatusAsleep, db)
	}

	// Signing off is seen as acknowledgement of the "shutdown" status.
	if err := worker.AckStatusChange(workerStatusOffline, db); err != nil {
		if !sentHeader {
			w.WriteHeader(http.StatusInternalServerError)
		}
		log.WithFields(logFields).WithError(err).Error("WorkerSignOff: unable to update worker")
	} else {
		if !sentHeader {
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

// WorkerSignOn allows a Worker to register a new list of supported task types.
// It also clears the worker's "current" task from the dashboard, so that it's clear that the
// now-active worker is not actually working on that task.
func WorkerSignOn(w http.ResponseWriter, r *auth.AuthenticatedRequest, db *mgo.Database) {
	worker, logFields := findWorkerForHTTP(w, r, db)
	log.WithFields(logFields).Info("Worker signed on")

	// Parse the given worker information.
	winfo := WorkerSignonDoc{}
	if err := DecodeJSON(w, r.Body, &winfo, fmt.Sprintf("%s WorkerSignOn:", r.RemoteAddr)); err != nil {
		return
	}

	// Only update those fields that were actually given.
	updateSet := bson.M{}
	if winfo.Nickname != "" && winfo.Nickname != worker.Nickname {
		logFields["new_nickname"] = winfo.Nickname
		log.WithFields(logFields).Info("Worker changed nickname")
		updateSet["nickname"] = winfo.Nickname
	}
	if len(winfo.SupportedTaskTypes) > 0 {
		updateSet["supported_task_types"] = winfo.SupportedTaskTypes
	}

	updateUnset := bson.M{
		"current_task": 1,
	}
	worker.CurrentTask = nil
	worker.SetStatus(workerStatusStarting, db)

	if err := worker.SeenEx(&r.Request, db, updateSet, updateUnset); err != nil {
		log.WithFields(logFields).WithError(err).Error("WorkerSignOn: Unable to update worker")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// WorkerAckStatusChange allows a Worker to acknowledge a requested status change.
func WorkerAckStatusChange(w http.ResponseWriter, r *auth.AuthenticatedRequest, db *mgo.Database, ackStatus string) {
	worker, logFields := findWorkerForHTTP(w, r, db)
	logFields["ack_status"] = ackStatus
	logFields["previous_status"] = worker.Status
	logFields["status_requested"] = worker.StatusRequested
	logger := log.WithFields(logFields)

	if ackStatus != worker.StatusRequested {
		logger.Warning("WorkerAckStatusChange: acknowledged status is not the same as requested status. Accepting anyway.")
	} else {
		logger.Info("WorkerAckStatusChange: worker acknowledged requested status")
	}
	err := worker.AckStatusChange(ackStatus, db)
	if err != nil {
		logger.WithError(err).Error("WorkerAckStatusChange: unable to set status change from worker ACK")
	}

	w.WriteHeader(http.StatusNoContent)
}

// WorkerGetStatusChange allows a Worker to fetch any pending status change.
func WorkerGetStatusChange(w http.ResponseWriter, r *auth.AuthenticatedRequest, db *mgo.Database) {
	worker, logFields := findWorkerForHTTP(w, r, db)
	logFields["status"] = worker.Status
	logFields["status_requested"] = worker.StatusRequested

	worker.Seen(&r.Request, db)

	if worker.StatusRequested == "" {
		w.WriteHeader(http.StatusNoContent)
		log.WithFields(logFields).Debug("no worker status change requested")
		return
	}

	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	err := enc.Encode(WorkerStatus{worker.StatusRequested})
	if err != nil {
		log.WithFields(logFields).WithError(err).Error("unable to marshal WorkerStatus response")
		return
	}

	log.WithFields(logFields).Info("sent requested status change to worker")
}
