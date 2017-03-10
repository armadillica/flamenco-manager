package flamenco

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	mgo "gopkg.in/mgo.v2"
)

// Reporter can show HTML and JSON reports.
type Reporter struct {
	session         *mgo.Session
	flamencoVersion string
	server          string
}

// CreateReporter creates a new Reporter object.
func CreateReporter(config *Conf, session *mgo.Session, flamencoVersion string) *Reporter {
	return &Reporter{
		session,
		flamencoVersion,
		config.FlamencoStr,
	}
}

// AddRoutes adds routes to serve reporting status requests.
func (rep *Reporter) AddRoutes(router *mux.Router) {
	router.HandleFunc("/", rep.showStatusPage).Methods("GET")
	router.HandleFunc("/as-json", rep.sendStatusReport).Methods("GET")

	static := noDirListing(http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
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

func (rep *Reporter) showStatusPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/dashboard.html")
	if err != nil {
		log.Error("Error parsing HTML template: ", err.Error())
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Version": rep.flamencoVersion,
	}

	tmpl.Execute(w, data)
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
	if err = db.C("flamenco_workers").Find(M{}).Sort("nickname", "status").All(&workers); err != nil {
		fmt.Printf("ERROR: %s\n", err.Error())
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
	encoder.Encode(statusreport)
}
