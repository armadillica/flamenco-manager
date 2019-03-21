/* (c) 2019, Blender Foundation - Sybren A. Stüvel
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be
 * included in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

// Package flamenco periodically fetches new tasks from the Flamenco Server, and sends updates back.
package flamenco

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
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
	log.Debug("UpstreamConnection: shutting down, waiting for shutdown to complete.")
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
				log.Debug("KickDownloader: done.")
				return
			case <-uc.doneChan:
				log.Debug("KickDownloader: Aborting waiting for task downloader; shutting down.")
				return
			}
		}
	} else {
		log.Debug("KickDownloader: asynchronous kick, just kicking.")
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
				log.Debug("downloadTaskLoop: Going to fetch tasks due to periodic timeout.")
				downloadTasksFromUpstream(uc.config, mongoSess)
			case pingbackChan, ok := <-uc.downloadKick:
				if !ok {
					return
				}
				log.Debug("downloadTaskLoop: Going to fetch tasks due to kick.")
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

	strURL := fmt.Sprintf("/api/flamenco/managers/%s/depsgraph", config.ManagerID)
	logger := log.WithField("url", strURL)
	relURL, err := url.Parse(strURL)
	if err != nil {
		logger.Warning("Error parsing URL; unable to fetch tasks.")
		return
	}

	getURL := config.Flamenco.ResolveReference(relURL)
	req, err := http.NewRequest("GET", getURL.String(), nil)
	if err != nil {
		logger.WithError(err).Warning("Unable to create GET request")
		return
	}
	req.SetBasicAuth(config.ManagerSecret, "")

	// Set If-Modified-Since header on our request.
	settings := GetSettings(db)
	if settings.DepsgraphLastModified != nil {
		logger = logger.WithField("last_modified", *settings.DepsgraphLastModified)
		req.Header.Set("X-Flamenco-If-Updated-Since", *settings.DepsgraphLastModified)
	}
	logger.Debug("Getting tasks from upstream Flamenco")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.WithError(err).Warning("Unable to GET")
		return
	}
	if resp.StatusCode == http.StatusNotModified {
		logger.Debug("Server-side depsgraph was not modified, nothing to do.")
		return
	}
	if resp.StatusCode == http.StatusNoContent {
		logger.Debug("No tasks for us; sleeping.")
		return
	}
	if resp.StatusCode >= 300 {
		body, ioErr := ioutil.ReadAll(resp.Body)
		if ioErr != nil {
			logger = logger.WithError(ioErr)
		} else {
			logger = logger.WithField("body", string(body))
		}
		logger.WithField("status_code", resp.StatusCode).Error("Error GETing tasks")
		return
	}

	// Parse the received tasks.
	var scheduledTasks ScheduledTasks
	decoder := json.NewDecoder(resp.Body)
	defer resp.Body.Close()

	if err = decoder.Decode(&scheduledTasks); err != nil {
		logger.WithError(err).Warning("Unable to decode scheduled tasks JSON")
		return
	}

	// Insert them into the MongoDB
	depsgraph := scheduledTasks.Depsgraph
	if len(depsgraph) > 0 {
		logger.WithField("count", len(depsgraph)).Info("Received tasks from upstream Flamenco Server")
	} else {
		// This shouldn't happen, as it should actually have been a 204 or 306.
		logger.WithField("count", len(depsgraph)).Warning("Unexpectedly received no tasks from upstream Flamenco Server")
		return
	}

	// Erase the URL from the logger; it's not necessary to keep repeating it.
	logger = log.WithFields(log.Fields{})

	tasksColl := db.C("flamenco_tasks")
	utcNow := UtcNow()
	for _, task := range depsgraph {
		// Count this as an update. By storing the update as "now", we don't have
		// to parse the _updated field's date format from the Flamenco Server.
		task.LastUpdated = utcNow
		taskLogger := logger.WithField("task_id", task.ID.Hex())

		// For compatibility with older Flamenco Servers, use an explicit string "unknown"
		// as task type if it's empty.
		if task.TaskType == "" {
			task.TaskType = defaultTaskType
			taskLogger.WithField("default_task_type", defaultTaskType).
				Warning("Task has no task type, using default task type")
		}

		change, err := tasksColl.Upsert(bson.M{"_id": task.ID}, task)
		if err != nil {
			taskLogger.WithError(err).Error("unable to insert new task in MongoDB")
			continue
		}

		if change.Updated > 0 {
			taskLogger.Debug("Upstream server re-queued existing task")
		} else if change.Matched > 0 {
			taskLogger.Debug("Upstream server re-queued existing task, but nothing changed")
		}
	}

	// Check if we had a Last-Modified header, since we need to remember that.
	lastModified := resp.Header.Get("X-Flamenco-Last-Updated")
	if lastModified != "" {
		logger.WithField("last_modified", lastModified).Info("Storing 'last modified' timestamp")
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
func (uc *UpstreamConnection) SendTaskUpdates(updates []TaskUpdate) (*TaskUpdateResponse, error) {
	url, err := uc.ResolveURL("/api/flamenco/managers/%s/task-update-batch",
		uc.config.ManagerID)
	if err != nil {
		log.WithError(err).Error("SendTaskUpdates: unable to construct URL")
		return nil, err
	}

	response := TaskUpdateResponse{}
	parseResponse := func(resp *http.Response, body []byte) error {
		err = json.Unmarshal(body, &response)
		if err != nil {
			log.WithError(err).Warning("SendTaskUpdates: error parsing server response")
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
	logger := log.WithField("task_id", task.ID.Hex())
	getURL, err := uc.ResolveURL("/api/flamenco/tasks/%s", task.ID.Hex())
	if err != nil {
		logger.WithError(err).Error("Unable to resolve URL to re-fetch task")
		return false
	}
	urlLog := logger.WithField("url", getURL)
	urlLog.Info("Verifying task with Flamenco Server")

	req, err := http.NewRequest("GET", getURL.String(), nil)
	if err != nil {
		urlLog.WithError(err).Error("Unable to create GET request")
		return false
	}
	req.SetBasicAuth(uc.config.ManagerSecret, "")
	req.Header["If-None-Match"] = []string{task.Etag}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		urlLog.WithError(err).Warning("Unable to re-fetch task")
		return false
	}

	if resp.StatusCode == http.StatusNotModified {
		// Nothing changed, we're good to go.
		logger.Info("Cached task is still the same on the Server")
		return false
	}

	statusLog := urlLog.WithField("status_code", resp.StatusCode)
	if resp.StatusCode >= 500 {
		// Internal errors, we'll ignore that.
		statusLog.Warning("Error trying to re-fetch task")
		return false
	}
	if 300 <= resp.StatusCode && resp.StatusCode < 400 {
		// Redirects, we'll ignore those too for now.
		statusLog.Warning("Redirect trying to re-fetch task, not following")
		return false
	}

	// Either the task is gone (or gone-ish, i.e. 4xx code) or it has changed.
	// If it is gone, we handle it as canceled.
	newTask := Task{}

	if resp.StatusCode >= 400 || resp.StatusCode == 204 {
		// Not found, access denied, that kind of stuff. Locally cancel the task.
		// TODO: probably better to go to "failed".
		statusLog.Warning("Error code when re-fetching task; canceling local copy")

		newTask = *task
		newTask.Status = "canceled"
	} else {
		// Parse the new task we received.
		decoder := json.NewDecoder(resp.Body)
		defer resp.Body.Close()

		if err = decoder.Decode(&newTask); err != nil {
			// We can't decode what's being sent. Remove it from the queue, as we no longer know
			// whether this task is valid at all.
			urlLog.WithError(err).Warning("Unable to decode updated tasks JSON")

			newTask = *task
			newTask.Status = "canceled"
		}
	}

	// save the task to the queue.
	log.WithFields(log.Fields{
		"task_status":   newTask.Status,
		"task_priority": newTask.Priority,
	}).Info("Cached task was changed on the Server")
	tasksColl := uc.session.DB("").C("flamenco_tasks")
	tasksColl.UpdateId(task.ID,
		bson.M{"$set": newTask})

	return true
}
