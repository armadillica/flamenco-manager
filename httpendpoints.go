package main

/* ***** BEGIN MIT LICENSE BLOCK *****
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
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
 * ***** END MIT LICENCE BLOCK *****
 */

import (
	"fmt"
	"net/http"

	auth "github.com/abbot/go-http-auth"
	"github.com/armadillica/flamenco-manager/flamenco"
	"github.com/gorilla/mux"
)

// AddRoutes adds the main Flamenco Manager endpoints to the Router.
func AddRoutes(router *mux.Router, workerAuthenticator *auth.BasicAuth) {
	router.HandleFunc("/register-worker", httpRegisterWorker).Methods("POST")
	router.HandleFunc("/task", workerAuthenticator.Wrap(httpScheduleTask)).Methods("POST")
	router.HandleFunc("/tasks/{task-id}/update", workerAuthenticator.Wrap(httpTaskUpdate)).Methods("POST")
	router.HandleFunc("/tasks/{task-id}/return", workerAuthenticator.Wrap(httpTaskReturn)).Methods("POST")
	router.HandleFunc("/tasks/{task-id}/redir-to-server", httpTaskRedirToServer)
	router.HandleFunc("/may-i-run/{task-id}", workerAuthenticator.Wrap(httpWorkerMayRunTask)).Methods("GET")
	router.HandleFunc("/status-change", workerAuthenticator.Wrap(httpWorkerGetStatusChange)).Methods("GET")
	router.HandleFunc("/ack-status-change/{ack-status}", workerAuthenticator.Wrap(httpWorkerAckStatusChange)).Methods("POST")
	router.HandleFunc("/sign-on", workerAuthenticator.Wrap(httpWorkerSignOn)).Methods("POST")
	router.HandleFunc("/sign-off", workerAuthenticator.Wrap(httpWorkerSignOff)).Methods("POST")
	router.HandleFunc("/kick", httpKick)
	router.HandleFunc("/logfile/{job-id}/{task-id}", httpTaskLog)
}

func httpRegisterWorker(w http.ResponseWriter, r *http.Request) {
	mongoSess := session.Copy()
	defer mongoSess.Close()
	flamenco.RegisterWorker(w, r, mongoSess.DB(""))
}

func httpTaskRedirToServer(w http.ResponseWriter, r *http.Request) {
	taskID, err := flamenco.ObjectIDFromRequest(w, r, "task-id")
	if err != nil {
		return
	}

	serverURL, err := config.Flamenco.Parse("/flamenco/tasks/" + taskID.Hex())
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to construct URL: %s", err.Error()), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, serverURL.String(), http.StatusTemporaryRedirect)
}

func httpScheduleTask(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	taskScheduler.ScheduleTask(w, r)
}

func httpKick(w http.ResponseWriter, r *http.Request) {
	upstream.KickDownloader(false)
	fmt.Fprintln(w, "Kicked task downloader")
}

func httpTaskLog(w http.ResponseWriter, r *http.Request) {
	jobID, err := flamenco.ObjectIDFromRequest(w, r, "job-id")
	if err != nil {
		return
	}
	taskID, err := flamenco.ObjectIDFromRequest(w, r, "task-id")
	if err != nil {
		return
	}

	flamenco.ServeTaskLog(w, r, jobID, taskID, taskUpdateQueue)
}

func httpTaskUpdate(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	taskID, err := flamenco.ObjectIDFromRequest(w, &r.Request, "task-id")
	if err != nil {
		return
	}

	taskUpdateQueue.QueueTaskUpdateFromWorker(w, r, mongoSess.DB(""), taskID)
}

func httpTaskReturn(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	taskID, err := flamenco.ObjectIDFromRequest(w, &r.Request, "task-id")
	if err != nil {
		return
	}

	taskScheduler.ReturnTaskFromWorker(w, r, mongoSess.DB(""), taskID)
}

/**
 * Called by a worker, to check whether it is allowed to keep running this task.
 */
func httpWorkerMayRunTask(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	taskID, err := flamenco.ObjectIDFromRequest(w, &r.Request, "task-id")
	if err != nil {
		return
	}

	taskScheduler.WorkerMayRunTask(w, r, mongoSess.DB(""), taskID)
}

func httpWorkerAckStatusChange(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	vars := mux.Vars(&r.Request)
	ackStatus := vars["ack-status"]

	flamenco.WorkerAckStatusChange(w, r, mongoSess.DB(""), ackStatus)
}

func httpWorkerGetStatusChange(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	flamenco.WorkerGetStatusChange(w, r, mongoSess.DB(""))
}

func httpWorkerSignOn(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	flamenco.WorkerSignOn(w, r, mongoSess.DB(""), upstreamNotifier)
}

func httpWorkerSignOff(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	flamenco.WorkerSignOff(w, r, mongoSess.DB(""), taskScheduler)
}

func workerSecret(user, realm string) string {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	return flamenco.WorkerSecret(user, mongoSess.DB(""))
}
