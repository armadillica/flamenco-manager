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

package flamenco

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/armadillica/flamenco-manager/jwtauth"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// Dashboard can show HTML and JSON reports.
type Dashboard struct {
	session         *mgo.Session
	config          *Conf
	sleeper         *SleepScheduler
	blacklist       *WorkerBlacklist
	flamencoVersion string
	serverName      string
	serverURL       string
	root            string
}

// CreateDashboard creates a new Dashboard object.
func CreateDashboard(config *Conf,
	session *mgo.Session,
	sleeper *SleepScheduler,
	blacklist *WorkerBlacklist,
	flamencoVersion string,
) *Dashboard {
	serverURL, err := url.Parse(config.FlamencoStr)
	if err != nil {
		log.WithError(err).Fatal("CreateReporter: unable to parse server URL")
	}
	serverURL.Path = "/flamenco/"

	return &Dashboard{
		session,
		config,
		sleeper,
		blacklist,
		flamencoVersion,
		serverURL.Host,
		serverURL.String(),
		TemplatePathPrefix("templates/dashboard.html"),
	}
}

// AddRoutes adds routes to serve reporting status requests.
func (dash *Dashboard) AddRoutes(router *mux.Router, auther jwtauth.Authenticator) {
	// JWT token protected:
	router.Handle("/as-json", auther.WrapFunc(dash.sendStatusReport)).Methods("GET")
	router.Handle("/set-sleep-schedule/{worker-id}", auther.WrapFunc(dash.setSleepSchedule)).Methods("POST")
	router.Handle("/static/latest-image.jpg", auther.WrapFunc(dash.serveLatestImage)).Methods("GET")
	router.Handle("/worker-action/{worker-id}", auther.WrapFunc(dash.workerAction)).Methods("POST")

	// Unprotected, treat as accessible to the world:
	router.HandleFunc("/", dash.showStatusPage).Methods("GET")
	router.HandleFunc("/latest-image", dash.showLatestImagePage).Methods("GET")

	static := noDirListing(http.StripPrefix("/static/", http.FileServer(http.Dir(dash.root+"static"))))
	router.PathPrefix("/static/").Handler(static).Methods("GET")

}

func noDirListing(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (dash *Dashboard) serveLatestImage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, path.Join(dash.root, "static", "latest-image.jpg"))
}

func (dash *Dashboard) showTemplate(templfname string, w http.ResponseWriter, r *http.Request) {
	tmpl := template.New("").Funcs(template.FuncMap{
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, errors.New("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, errors.New("dict keys must be strings")
				}
				dict[key] = values[i+1]
				log.Infof("dict[%q] = %q", key, values[i+1])
			}
			return dict, nil
		},
	})

	tmpl, err := tmpl.ParseFiles(
		dash.root+"templates/layout.html",
		dash.root+templfname)
	if err != nil {
		log.Errorf("Error parsing HTML template %s: %s", templfname, err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// TODO(Sybren): cache this in memory and check mtime.
	vueTemplates, err := ioutil.ReadFile(dash.root + "static/vue-components.html")
	if err != nil {
		log.WithError(err).Error("Error loading Vue.js templates")
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Version":      dash.flamencoVersion,
		"Config":       dash.config,
		"VueTemplates": template.HTML(vueTemplates),
	}

	log.WithField("fname", templfname).Debug("serving template")

	err = tmpl.ExecuteTemplate(w, "layout", data)
	if err != nil {
		log.Errorf("Error executing HTML template %s: %s", templfname, err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}

func (dash *Dashboard) showStatusPage(w http.ResponseWriter, r *http.Request) {
	dash.showTemplate("templates/dashboard.html", w, r)
}

func (dash *Dashboard) showLatestImagePage(w http.ResponseWriter, r *http.Request) {
	dash.showTemplate("templates/latest_image.html", w, r)
}

// sendStatusReport reports the status of the manager in JSON.
func (dash *Dashboard) sendStatusReport(w http.ResponseWriter, r *http.Request) {
	mongoSess := dash.session.Copy()
	defer mongoSess.Close()
	db := mongoSess.DB("")

	var taskCount, workerCount, upstreamQueueSize int
	var err error
	if taskCount, err = Count(db.C("flamenco_tasks")); err != nil {
		fmt.Printf("ERROR : %s\n", err.Error())
		return
	}
	if workerCount, err = Count(db.C("flamenco_workers")); err != nil {
		fmt.Printf("ERROR : %s\n", err.Error())
		return
	}
	if upstreamQueueSize, err = Count(db.C("task_update_queue")); err != nil {
		fmt.Printf("ERROR : %s\n", err.Error())
		return
	}

	var workers []Worker
	pipe := db.C("flamenco_workers").Pipe([]M{
		// 1: Look up the task for each worker.
		M{"$lookup": M{
			"from":         "flamenco_tasks",
			"localField":   "current_task",
			"foreignField": "_id",
			"as":           "_task",
		}},
		// Look up the blacklist for this worker.
		M{"$lookup": M{
			"from":         "worker_blacklist",
			"localField":   "_id",
			"foreignField": "worker_id",
			"as":           "blacklist",
		}},
		// 2: Unwind the 1-element task array.
		M{"$unwind": M{
			"path":                       "$_task",
			"preserveNullAndEmptyArrays": true,
		}},
		// 3: Project to just get what we need.
		M{"$project": M{
			// To get the info from the task itself, swap out these two lines with the two lines below.
			// "current_task_status":  "$_task.status",
			// "current_task_updated": "$_task.last_updated",
			"current_task_status":  1,
			"current_task_updated": 1,
			"address":              1,
			"current_task":         1,
			"current_job":          "$_task.job",
			"last_activity":        1,
			"nickname":             1,
			"platform":             1,
			"software":             1,
			"status":               1,
			"status_requested":     1,
			"lazy_status_request":  1,
			"supported_task_types": 1,
			"sleep_schedule":       1,
			"blacklist":            1,
		}},
		// 4: Sort.
		M{"$sort": bson.D{
			{Name: "nickname", Value: 1},
			{Name: "status", Value: 1},
		}},
	})

	if err := pipe.All(&workers); err != nil {
		log.Errorf("Unable to fetch dashboard data: %s", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "close")

	statusreport := StatusReport{
		NrOfWorkers:       workerCount,
		NrOfTasks:         taskCount,
		UpstreamQueueSize: upstreamQueueSize,
		Version:           dash.flamencoVersion,
		Workers:           workers,
		ManagerMode:       dash.config.Mode,
		ManagerName:       dash.config.ManagerName,
	}
	statusreport.Server.Name = dash.serverName
	statusreport.Server.URL = dash.serverURL

	encoder := json.NewEncoder(w)
	if err := encoder.Encode(statusreport); err != nil {
		// This is probably fine; broken connections are bound to happen.
		log.Debugf("Unable to send dashboard data to %s: %s", r.RemoteAddr, err)
	}
}

func (dash *Dashboard) parseRequest(w http.ResponseWriter, r *http.Request, session *mgo.Session) (*Worker, *log.Entry, bool) {
	workerIDstr := mux.Vars(r)["worker-id"]

	logger := log.WithFields(log.Fields{
		"remote_addr": r.RemoteAddr,
		"worker_id":   workerIDstr,
	})

	if !bson.IsObjectIdHex(workerIDstr) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid worker ID")
		logger.Warning("workerAction: called with bad worker ID")
		return nil, nil, false
	}

	db := session.DB("")
	worker, err := FindWorker(workerIDstr, M{}, db)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Worker not found")
		logger.WithError(err).Warning("workerAction: error finding worker")
		return nil, nil, false
	}

	return worker, logger, true
}

func (dash *Dashboard) workerAction(w http.ResponseWriter, r *http.Request) {
	session := dash.session.Clone()
	defer session.Close()

	worker, logger, ok := dash.parseRequest(w, r, session)
	if !ok {
		return
	}

	action := r.FormValue("action")
	logger = logger.WithField("action", action)

	db := session.DB("")
	var actionResult string
	var actionErr error
	actionHandlers := map[string]func(){
		"set-status": func() {
			requestedStatus := r.FormValue("status")
			lazy := Lazyness(r.FormValue("lazy") == "true")
			logger = logger.WithFields(log.Fields{
				"requested_status": requestedStatus,
				"lazy":             lazy,
			})
			actionErr = worker.RequestStatusChange(requestedStatus, lazy, db)
		},
		"shutdown": func() {
			lazy := Lazyness(r.FormValue("lazy") == "true")
			logger = logger.WithFields(log.Fields{
				"lazy": lazy,
			})
			actionErr = worker.RequestStatusChange(workerStatusShutdown, lazy, db)
		},
		"ack-timeout": func() {
			actionErr = worker.AckTimeout(db)
		},
		"send-test-job": func() {
			actionResult, actionErr = CreateTestTask(worker, dash.config, db)
		},
		"forget-worker": func() {
			actionErr = forgetWorker(worker, db)
		},
		"forget-blacklist-line": func() {
			jobIDstr := r.FormValue("job_id")
			taskType := r.FormValue("task_type")

			if !bson.IsObjectIdHex(jobIDstr) {
				actionErr = errors.New("Job ID is not a valid ObjectID")
				return
			}
			actionErr = dash.blacklist.RemoveLine(worker.ID, bson.ObjectIdHex(jobIDstr), taskType)
		},
	}

	handler, ok := actionHandlers[action]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid action")
		logger.Warning("workerAction: invalid action requested")
		return
	}

	handler()

	if actionErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, actionErr)
		logger.WithError(actionErr).Warning("workerAction: error occurred")
	} else {
		if actionResult == "" {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.Header().Add("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, actionResult)
		}
		logger.Info("workerAction: action OK")
	}
}

func (dash *Dashboard) setSleepSchedule(w http.ResponseWriter, r *http.Request) {
	session := dash.session.Clone()
	defer session.Close()

	worker, logger, ok := dash.parseRequest(w, r, session)
	if !ok {
		return
	}

	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "expected JSON", http.StatusNotAcceptable)
		return
	}

	var schedule ScheduleInfo
	if err := DecodeJSON(w, r.Body, &schedule, "setSleepSchedule"); err != nil {
		return
	}

	logger = logger.WithField("schedule", schedule)
	logger.Info("setting worker sleep schedule")
	if err := dash.sleeper.SetSleepSchedule(worker, schedule, session.DB("")); err != nil {
		logger.WithError(err).Error("unable to set worker schedule")
		http.Error(w, "Error setting sleep schedule: "+err.Error(), http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusNoContent)
}
