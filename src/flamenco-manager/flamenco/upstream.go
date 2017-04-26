/**
 * Periodically fetches new tasks from the Flamenco Server, and sends updates back.
 */
package flamenco

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	log "github.com/Sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// The default task type, in case the task has no task_type field when we get it
// from the Flamenco Server. This is for backward compatibility with Server
// versions older than 2.0.1.
const defaultTaskType = "unknown"

// UpstreamConnection represents a connection to an upstream Flamenco Server.
type UpstreamConnection struct {
	closable
	config  *Conf
	session *mgo.Session

	// Send any boolean here to kick the task downloader into downloading new tasks.
	downloadKick chan chan bool
}

// ConnectUpstream creates a new UpstreamConnection object and starts the task download loop.
func ConnectUpstream(config *Conf, session *mgo.Session) *UpstreamConnection {
	upconn := UpstreamConnection{
		makeClosable(),
		config,
		session,
		make(chan chan bool),
	}
	upconn.downloadTaskLoop()

	return &upconn
}

// Close gracefully closes the upstream connection by stopping all upload/download loops.
func (uc *UpstreamConnection) Close() {
	log.Debugf("UpstreamConnection: shutting down, waiting for shutdown to complete.")
	close(uc.downloadKick) // TODO: maybe move this between closing of done channel and waiting
	uc.closableCloseAndWait()
	log.Info("UpstreamConnection: shutdown complete.")
}

// KickDownloader fetches new tasks from the Flamenco Server.
func (uc *UpstreamConnection) KickDownloader(synchronous bool) {
	if synchronous {
		pingback := make(chan bool)
		uc.downloadKick <- pingback
		log.Info("KickDownloader: Waiting for task downloader to finish.")

		// wait for the download to be complete, or the connection to be shut down.
		uc.closableAdd(1)
		defer uc.closableDone()

		for {
			select {
			case <-pingback:
				log.Debugf("KickDownloader: done.")
				return
			case <-uc.doneChan:
				log.Debugf("KickDownloader: Aborting waiting for task downloader; shutting down.")
				return
			}
		}
	} else {
		log.Debugf("KickDownloader: asynchronous kick, just kicking.")
		uc.downloadKick <- nil
	}
}

func (uc *UpstreamConnection) downloadTaskLoop() {
	timerChan := Timer("downloadTaskLoop",
		uc.config.DownloadTaskSleep,
		0,
		&uc.closable,
	)

	go func() {
		mongoSess := uc.session.Copy()
		defer mongoSess.Close()

		uc.closableAdd(1)
		defer uc.closableDone()
		defer log.Info("downloadTaskLoop: Task download goroutine shutting down.")

		for {
			select {
			case <-uc.doneChan:
				return
			case _, ok := <-timerChan:
				if !ok {
					return
				}
				log.Debugf("downloadTaskLoop: Going to fetch tasks due to periodic timeout.")
				downloadTasksFromUpstream(uc.config, mongoSess)
			case pingbackChan, ok := <-uc.downloadKick:
				if !ok {
					return
				}
				log.Debugf("downloadTaskLoop: Going to fetch tasks due to kick.")
				downloadTasksFromUpstream(uc.config, mongoSess)
				if pingbackChan != nil {
					pingbackChan <- true
				}
			}
		}
	}()
}

/**
 * Downloads a chunkn of tasks from the upstream Flamenco Server.
 */
func downloadTasksFromUpstream(config *Conf, mongoSess *mgo.Session) {
	db := mongoSess.DB("")

	strURL := fmt.Sprintf("/api/flamenco/managers/%s/depsgraph", config.ManagerId)
	relURL, err := url.Parse(strURL)
	if err != nil {
		log.Warningf("Error parsing '%s' as URL; unable to fetch tasks.", strURL)
		return
	}

	getURL := config.Flamenco.ResolveReference(relURL)
	req, err := http.NewRequest("GET", getURL.String(), nil)
	if err != nil {
		log.Warningf("Unable to create GET request: %s", err)
		return
	}
	req.SetBasicAuth(config.ManagerSecret, "")

	// Set If-Modified-Since header on our request.
	settings := GetSettings(db)
	if settings.DepsgraphLastModified != nil {
		log.Infof("Getting tasks from upstream Flamenco %s If-Modified-Since %s", getURL,
			*settings.DepsgraphLastModified)
		req.Header.Set("X-Flamenco-If-Updated-Since", *settings.DepsgraphLastModified)
	} else {
		log.Infof("Getting tasks from upstream Flamenco %s", getURL)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Warningf("Unable to GET %s: %s", getURL, err)
		return
	}
	if resp.StatusCode == http.StatusNotModified {
		log.Debug("Server-side depsgraph was not modified, nothing to do.")
		return
	}
	if resp.StatusCode == http.StatusNoContent {
		log.Debug("No tasks for us; sleeping.")
		return
	}
	if resp.StatusCode >= 300 {
		body, ioErr := ioutil.ReadAll(resp.Body)
		if ioErr != nil {
			log.Errorf("Error %d GETing %s: %s", resp.StatusCode, getURL, ioErr)
			return
		}
		log.Errorf("Error %d GETing %s: %s", resp.StatusCode, getURL, body)
		return
	}

	// Parse the received tasks.
	var scheduledTasks ScheduledTasks
	decoder := json.NewDecoder(resp.Body)
	defer resp.Body.Close()

	if err = decoder.Decode(&scheduledTasks); err != nil {
		log.Warning("Unable to decode scheduled tasks JSON:", err)
		return
	}

	// Insert them into the MongoDB
	depsgraph := scheduledTasks.Depsgraph
	if len(depsgraph) > 0 {
		log.Infof("Received %d tasks from upstream Flamenco Server.", len(depsgraph))
	} else {
		// This shouldn't happen, as it should actually have been a 204 or 306.
		log.Debugf("Received %d tasks from upstream Flamenco Server.", len(depsgraph))
	}
	tasksColl := db.C("flamenco_tasks")
	utcNow := UtcNow()
	for _, task := range depsgraph {
		// Count this as an update. By storing the update as "now", we don't have
		// to parse the _updated field's date format from the Flamenco Server.
		task.LastUpdated = utcNow

		// For compatibility with older Flamen Servers, use an explicit string "unknown"
		// as task type if it's empty.
		if task.TaskType == "" {
			task.TaskType = defaultTaskType
			log.Warningf("Task %s has no task type, defaulting to task type \"%s\".",
				task.ID.Hex(), task.TaskType)
		}

		change, err := tasksColl.Upsert(bson.M{"_id": task.ID}, task)
		if err != nil {
			log.Errorf("unable to insert new task %s: %s", task.ID.Hex(), err)
			continue
		}

		if change.Updated > 0 {
			log.Debug("Upstream server re-queued existing task ", task.ID.Hex())
		} else if change.Matched > 0 {
			log.Debugf("Upstream server re-queued existing task %s, but nothing changed",
				task.ID.Hex())
		}
	}

	// Check if we had a Last-Modified header, since we need to remember that.
	lastModified := resp.Header.Get("X-Flamenco-Last-Updated")
	if lastModified != "" {
		log.Info("Last modified task was at ", lastModified)
		settings.DepsgraphLastModified = &lastModified
		SaveSettings(db, settings)
	}
}

// ResolveURL returns the given URL relative to the base URL of the upstream server, as absolute URL.
func (uc *UpstreamConnection) ResolveURL(relativeURL string, a ...interface{}) (*url.URL, error) {
	relURL, err := url.Parse(fmt.Sprintf(relativeURL, a...))
	if err != nil {
		return &url.URL{}, err
	}
	url := uc.config.Flamenco.ResolveReference(relURL)

	return url, nil
}

// SendJSON sends a JSON document to the given URL.
func (uc *UpstreamConnection) SendJSON(logprefix, method string, url *url.URL,
	payload interface{},
	responsehandler func(resp *http.Response, body []byte) error,
) error {
	authenticate := func(req *http.Request) {
		req.SetBasicAuth(uc.config.ManagerSecret, "")
	}

	return SendJSON(logprefix, method, url, payload, authenticate, responsehandler)
}

// SendTaskUpdates performs a POST to /api/flamenco/managers/{manager-id}/task-update-batch to
// send a batch of task updates to the Server.
func (uc *UpstreamConnection) SendTaskUpdates(updates *[]TaskUpdate) (*TaskUpdateResponse, error) {
	url, err := uc.ResolveURL("/api/flamenco/managers/%s/task-update-batch",
		uc.config.ManagerId)
	if err != nil {
		panic(fmt.Sprintf("SendTaskUpdates: unable to construct URL: %s\n", err))
	}

	response := TaskUpdateResponse{}
	parseResponse := func(resp *http.Response, body []byte) error {
		err = json.Unmarshal(body, &response)
		if err != nil {
			log.Warningf("SendTaskUpdates: error parsing server response: %s", err)
			return err
		}
		return nil
	}
	err = uc.SendJSON("SendTaskUpdates", "POST", url, updates, parseResponse)

	return &response, err
}

// RefetchTask re-fetches a task from the Server, but only if its etag changed.
// - If the etag changed, the differences between the old and new status are handled.
// - If the Server cannot be reached, this error is ignored and the task is untouched.
// - If the Server returns an error code other than 500 Internal Server Error,
//   (Forbidden, Not Found, etc.) the task is removed from the local task queue.
//
// If the task was untouched, this function returns false.
// If it was changed or removed, this function return true.
func (uc *UpstreamConnection) RefetchTask(task *Task) bool {
	getURL, err := uc.ResolveURL("/api/flamenco/tasks/%s", task.ID.Hex())
	if err != nil {
		log.Errorf("WARNING: Unable to resolve URL: %s", err)
		return false
	}
	log.Infof("Verifying task with Flamenco Server %s", getURL)

	req, err := http.NewRequest("GET", getURL.String(), nil)
	if err != nil {
		log.Errorf("WARNING: Unable to create GET request: %s", err)
		return false
	}
	req.SetBasicAuth(uc.config.ManagerSecret, "")
	req.Header["If-None-Match"] = []string{task.Etag}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Warningf("Unable to re-fetch task: %s", err)
		return false
	}

	if resp.StatusCode == http.StatusNotModified {
		// Nothing changed, we're good to go.
		log.Infof("Cached task %s is still the same on the Server", task.ID.Hex())
		return false
	}

	if resp.StatusCode >= 500 {
		// Internal errors, we'll ignore that.
		log.Warningf("Error %d trying to re-fetch task %s",
			resp.StatusCode, task.ID.Hex())
		return false
	}
	if 300 <= resp.StatusCode && resp.StatusCode < 400 {
		// Redirects, we'll ignore those too for now.
		log.Warningf("Redirect %d trying to re-fetch task %s, not following redirect.",
			resp.StatusCode, task.ID.Hex())
		return false
	}

	// Either the task is gone (or gone-ish, i.e. 4xx code) or it has changed.
	// If it is gone, we handle it as canceled.
	newTask := Task{}

	if resp.StatusCode >= 400 || resp.StatusCode == 204 {
		// Not found, access denied, that kind of stuff. Locally cancel the task.
		// TODO: probably better to go to "failed".
		log.Warningf("Code %d when re-fetching task %s; canceling local copy",
			resp.StatusCode, task.ID.Hex())

		newTask = *task
		newTask.Status = "canceled"
	} else {
		// Parse the new task we received.
		decoder := json.NewDecoder(resp.Body)
		defer resp.Body.Close()

		if err = decoder.Decode(&newTask); err != nil {
			// We can't decode what's being sent. Remove it from the queue, as we no longer know
			// whether this task is valid at all.
			log.Warningf("Unable to decode updated tasks JSON from %s: %s", getURL, err)

			newTask = *task
			newTask.Status = "canceled"
		}
	}

	// save the task to the queue.
	log.Infof("Cached task %s was changed on the Server, status=%s, priority=%d.",
		task.ID.Hex(), newTask.Status, newTask.Priority)
	tasksColl := uc.session.DB("").C("flamenco_tasks")
	tasksColl.UpdateId(task.ID,
		bson.M{"$set": newTask})

	return true
}
