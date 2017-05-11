package flamenco

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	auth "github.com/abbot/go-http-auth"

	"golang.org/x/crypto/bcrypt"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

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
func (worker *Worker) SetStatus(status string, db *mgo.Database) error {
	worker.Status = status
	updates := M{"status": status}
	return db.C("flamenco_workers").UpdateId(worker.ID, M{"$set": updates})
}

// SetCurrentTask sets the worker's current task, and updates the database too.
func (worker *Worker) SetCurrentTask(taskID bson.ObjectId, db *mgo.Database) error {
	worker.CurrentTask = &taskID
	updates := M{"current_task": taskID}
	return db.C("flamenco_workers").UpdateId(worker.ID, M{"$set": updates})
}

// TimeoutOnTask marks the worker as timed out on a given task.
// The task is just used for logging.
func (worker *Worker) TimeoutOnTask(task *Task, db *mgo.Database) {
	log.Warningf("Worker %s (%s) timed out on task %s",
		worker.Identifier(), worker.ID.Hex(), task.ID.Hex())

	err := worker.SetStatus("timeout", db)
	if err != nil {
		log.Errorf("Unable to set worker status: %s", err)
	}
}

// Seen registers that we have seen this worker at a certain address and with certain software.
func (worker *Worker) Seen(r *http.Request, db *mgo.Database) {
	if err := worker.SeenEx(r, db, bson.M{}, bson.M{}); err != nil {
		log.Errorf("Worker.Seen: unable to update worker %s in MongoDB: %s", worker.ID, err)
	}
}

// SeenEx is same as Seen(), but allows for extra updates on the worker in the database, and returns err
func (worker *Worker) SeenEx(r *http.Request, db *mgo.Database, set bson.M, unset bson.M) error {
	worker.LastActivity = UtcNow()

	set["last_activity"] = worker.LastActivity
	set["status"] = "awake"

	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Warningf("Unable to split '%s' into host/port; using the whole thing instead: %s",
			r.RemoteAddr, err)
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
	log.Debugf("Updating worker %s: %v", worker.ID, updates)

	return db.C("flamenco_workers").UpdateId(worker.ID, updates)
}

// RegisterWorker creates a new Worker in the DB, based on the WorkerRegistration document received.
func RegisterWorker(w http.ResponseWriter, r *http.Request, db *mgo.Database) {
	var err error

	log.Info(r.RemoteAddr, "Worker registering")

	// Parse the given worker information.
	winfo := WorkerRegistration{}
	if err = DecodeJson(w, r.Body, &winfo, fmt.Sprintf("%s RegisterWorker:", r.RemoteAddr)); err != nil {
		return
	}

	// Store it in MongoDB after hashing the password and assigning an ID.
	worker := Worker{}
	worker.Secret = winfo.Secret
	worker.Platform = winfo.Platform
	worker.SupportedTaskTypes = winfo.SupportedTaskTypes
	worker.Nickname = winfo.Nickname
	worker.Address = r.RemoteAddr

	if err = StoreNewWorker(&worker, db); err != nil {
		log.Errorf(r.RemoteAddr, "Unable to store worker:", err)

		w.WriteHeader(500)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "Unable to store worker")

		return
	}

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(worker); err != nil {
		log.Errorf("RegisterWorker: unable to send registration response to worker at %s: %s",
			r.RemoteAddr, err)
		return
	}
}

// StoreNewWorker saves the given worker in the database.
func StoreNewWorker(winfo *Worker, db *mgo.Database) error {
	var err error

	// Store it in MongoDB after hashing the password and assigning an ID.
	winfo.ID = bson.NewObjectId()
	winfo.HashedSecret, err = bcrypt.GenerateFromPassword([]byte(winfo.Secret), bcrypt.DefaultCost)
	if err != nil {
		log.Error("Unable to hash password:", err)
		return err
	}

	workersColl := db.C("flamenco_workers")
	if err = workersColl.Insert(winfo); err != nil {
		log.Error("Unable to insert worker in DB:", err)
		return err
	}

	return nil
}

// WorkerSecret returns the hashed secret of the worker.
func WorkerSecret(user string, db *mgo.Database) string {
	projection := bson.M{"hashed_secret": 1}
	worker, err := FindWorker(user, projection, db)

	if err != nil {
		log.Warning("Error fetching hashed password: ", err)
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

	// Get the worker
	worker, err := FindWorker(r.Username, M{"_id": 1, "address": 1, "nickname": 1}, db)
	if err != nil {
		log.Warningf("%s WorkerMayRunTask: Unable to find worker: %s",
			r.RemoteAddr, err)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "Unable to find worker: %s", err)
		return
	}
	worker.Seen(&r.Request, db)
	log.Debugf("WorkerMayRunTask: %s asking if it is allowed to keep running task %s",
		worker.Identifier(), taskID.Hex())

	response := MayKeepRunningResponse{}

	// Get the task and check its status.
	task := Task{}
	if err := db.C("flamenco_tasks").FindId(taskID).One(&task); err != nil {
		log.Warningf("%s WorkerMayRunTask: unable to find task %s for worker %s",
			r.RemoteAddr, taskID.Hex(), worker.ID.Hex())
		response.Reason = fmt.Sprintf("unable to find task %s", taskID.Hex())
	} else if task.WorkerID != nil && *task.WorkerID != worker.ID {
		log.Warningf("%s WorkerMayRunTask: task %s was assigned from worker %s to %s",
			r.RemoteAddr, taskID.Hex(), worker.ID.Hex(), task.WorkerID.Hex())
		response.Reason = fmt.Sprintf("task %s reassigned to another worker", taskID.Hex())
	} else if !IsRunnableTaskStatus(task.Status) {
		log.Warningf("%s WorkerMayRunTask: task %s is in not-runnable status %s, worker %s will stop",
			r.RemoteAddr, taskID.Hex(), task.Status, worker.ID.Hex())
		response.Reason = fmt.Sprintf("task %s in non-runnable status %s", taskID.Hex(), task.Status)
	} else {
		response.MayKeepRunning = true
		WorkerPingedTask(worker.ID, taskID, "", db)
	}

	// Send the response
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(response); err != nil {
		log.Warningf("WorkerMayRunTask: unable to send response to worker %s: %s",
			worker.Identifier(), taskID.Hex())
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

	now := UtcNow()
	updates := bson.M{
		"last_worker_ping": now,
		"worker_id":        workerID,
	}

	log.Debugf("WorkerPingedTask: updating task %s by setting %v", taskID, updates)
	if err := tasksColl.UpdateId(taskID, bson.M{"$set": updates}); err != nil {
		log.Errorf("WorkerPingedTask: ERROR unable to update last_worker_ping on task %s: %s",
			taskID.Hex(), err)
		return
	}

	// Also update this worker to reflect the last time it pinged a task.
	updates = bson.M{
		"current_task_updated": now,
	}
	if len(taskStatus) > 0 {
		updates["current_task_status"] = taskStatus
	}
	log.Debugf("WorkerPingedTask: updating worker %s by setting %v", workerID, updates)
	if err := workersColl.UpdateId(workerID, bson.M{"$set": updates}); err != nil {
		log.Errorf("WorkerPingedTask: ERROR unable to update current_task_updated on worker %s: %s",
			workerID.Hex(), err)
		return
	}
}

// WorkerSignOff re-queues all active tasks (should be only one) that are assigned to this worker.
func WorkerSignOff(w http.ResponseWriter, r *auth.AuthenticatedRequest, db *mgo.Database) {
	// Get the worker
	worker, err := FindWorker(r.Username, bson.M{"_id": 1, "address": 1, "nickname": 1}, db)
	if err != nil {
		log.Warningf("%s WorkerSignOff: Unable to find worker: %s", r.RemoteAddr, err)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	workerIdent := worker.Identifier()

	log.Warningf("%s Worker %s signing off", r.RemoteAddr, workerIdent)

	// Update the tasks assigned to the worker.
	var tasks []Task
	query := bson.M{
		"worker_id": worker.ID,
		"status":    "active",
	}
	sentHeader := false
	if err := db.C("flamenco_tasks").Find(query).All(&tasks); err != nil {
		log.Warningf("WorkerSignOff: unable to find active tasks of worker %s in MongoDB: %s",
			workerIdent, err)
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
			if err := QueueTaskUpdate(&tupdate, db); err != nil {
				if !sentHeader {
					w.WriteHeader(http.StatusInternalServerError)
					sentHeader = true
				}
				fmt.Fprintf(w, "Error updating task %s: %s\n", task.ID.Hex(), err)
				log.Errorf("WorkerSignOff: unable to update task %s for worker %s in MongoDB: %s",
					task.ID.Hex(), workerIdent, err)
			}
		}
	}

	// Update the worker itself, to show it's down in the DB too.

	if err := worker.SetStatus("down", db); err != nil {
		if !sentHeader {
			w.WriteHeader(http.StatusInternalServerError)
		}
		log.Errorf("WorkerSignOff: unable to update worker %s in MongoDB: %s", workerIdent, err)
	}
}

// WorkerSignOn allows a Worker to register a new list of supported task types.
// It also clears the worker's "current" task from the dashboard, so that it's clear that the
// now-active worker is not actually working on that task.
func WorkerSignOn(w http.ResponseWriter, r *auth.AuthenticatedRequest, db *mgo.Database) {
	// Get the worker
	worker, err := FindWorker(r.Username, bson.M{"_id": 1, "address": 1, "nickname": 1}, db)
	if err != nil {
		log.Warningf("%s WorkerSignOn: Unable to find worker: %s", r.RemoteAddr, err)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	log.Infof("Worker %s signed on", worker.Identifier())

	// Parse the given worker information.
	winfo := WorkerSignonDoc{}
	if err = DecodeJson(w, r.Body, &winfo, fmt.Sprintf("%s WorkerSignOn:", r.RemoteAddr)); err != nil {
		return
	}

	// Only update those fields that were actually given.
	updateSet := bson.M{}
	if winfo.Nickname != "" && winfo.Nickname != worker.Nickname {
		log.Infof("Worker %s changed nickname to %s", worker.Nickname, winfo.Nickname)
		updateSet["nickname"] = winfo.Nickname
	}
	if len(winfo.SupportedTaskTypes) > 0 {
		updateSet["supported_task_types"] = winfo.SupportedTaskTypes
	}

	updateUnset := bson.M{
		"current_task": 1,
	}
	worker.CurrentTask = nil

	if err = worker.SeenEx(&r.Request, db, updateSet, updateUnset); err != nil {
		log.Errorf("WorkerSignOn: Unable to update worker %s: %s", worker.Identifier(), err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
