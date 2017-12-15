package flamenco

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// Reporter can show HTML and JSON reports.
type Reporter struct {
	session         *mgo.Session
	config          *Conf
	flamencoVersion string
	server          string
	root            string
}

// CreateReporter creates a new Reporter object.
func CreateReporter(config *Conf, session *mgo.Session, flamencoVersion string) *Reporter {
	return &Reporter{
		session,
		config,
		flamencoVersion,
		config.FlamencoStr,
		TemplatePathPrefix("templates/dashboard.html"),
	}
}

// AddRoutes adds routes to serve reporting status requests.
func (rep *Reporter) AddRoutes(router *mux.Router) {
	router.HandleFunc("/", rep.showStatusPage).Methods("GET")
	router.HandleFunc("/as-json", rep.sendStatusReport).Methods("GET")
	router.HandleFunc("/latest-image", rep.showLatestImagePage).Methods("GET")

	router.HandleFunc("/worker-action/{worker-id}", rep.workerAction).Methods("POST")

	static := noDirListing(http.StripPrefix("/static/", http.FileServer(http.Dir(rep.root+"static"))))
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

func (rep *Reporter) showTemplate(templfname string, w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles(rep.root + templfname)
	if err != nil {
		log.Error("Error parsing HTML template: ", err.Error())
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Version": rep.flamencoVersion,
		"Config":  rep.config,
	}

	tmpl.Execute(w, data)
}

func (rep *Reporter) showStatusPage(w http.ResponseWriter, r *http.Request) {
	rep.showTemplate("templates/dashboard.html", w, r)
}

func (rep *Reporter) showLatestImagePage(w http.ResponseWriter, r *http.Request) {
	rep.showTemplate("templates/latest_image.html", w, r)
}

// sendStatusReport reports the status of the manager in JSON.
func (rep *Reporter) sendStatusReport(w http.ResponseWriter, r *http.Request) {
	mongoSess := rep.session.Copy()
	defer mongoSess.Close()
	db := mongoSess.DB("")

	var taskCount, workerCount int
	var err error
	if taskCount, err = Count(db.C("flamenco_tasks")); err != nil {
		fmt.Printf("ERROR : %s\n", err.Error())
		return
	}
	if workerCount, err = Count(db.C("flamenco_workers")); err != nil {
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
		// 2: Unwind the 1-element task array.
		M{"$unwind": M{
			"path": "$_task",
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
			"last_activity":        1,
			"nickname":             1,
			"platform":             1,
			"software":             1,
			"status":               1,
			"status_requested":     1,
			"supported_task_types": 1,
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
		workerCount,
		taskCount,
		rep.flamencoVersion,
		workers,
		rep.server,
	}

	encoder := json.NewEncoder(w)
	if err := encoder.Encode(statusreport); err != nil {
		// This is probably fine; broken connections are bound to happen.
		log.Debugf("Unable to send dashboard data to %s: %s", r.RemoteAddr, err)
	}
}

func (rep *Reporter) workerAction(w http.ResponseWriter, r *http.Request) {
	workerIDstr := mux.Vars(r)["worker-id"]
	action := r.FormValue("action")

	logger := log.WithFields(log.Fields{
		"remote_addr": r.RemoteAddr,
		"worker_id":   workerIDstr,
		"action":      action,
	})

	if !bson.IsObjectIdHex(workerIDstr) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid worker ID")
		logger.Warning("workerAction: called with bad worker ID")
		return
	}

	session := rep.session.Clone()
	defer session.Close()

	db := session.DB("")
	worker, err := FindWorker(workerIDstr, M{}, db)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Worker not found")
		logger.WithError(err).Warning("workerAction: error finding worker")
		return
	}

	var actionResult string
	var actionErr error
	actionHandlers := map[string]func(){
		"set-status": func() {
			requestedStatus := r.FormValue("status")
			logger = logger.WithField("requested_status", requestedStatus)
			actionErr = worker.RequestStatusChange(requestedStatus, db)
		},
		"shutdown": func() {
			actionErr = worker.RequestStatusChange(workerStatusShutdown, db)
		},
		"ack-timeout": func() {
			actionErr = worker.AckTimeout(db)
		},
		"send-test-job": func() {
			actionResult, actionErr = SendTestJob(worker, rep.config, db)
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
