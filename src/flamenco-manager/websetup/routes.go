package websetup

import (
	"encoding/json"
	"flamenco-manager/flamenco"
	"html/template"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// Routes handles all HTTP routes for the web setup wizard.
type Routes struct {
	config          *flamenco.Conf
	flamencoVersion string
}

// TemplateData is the mapping type we use to pass data to the template engine.
type TemplateData map[string]interface{}

// CreateWebSetup creates a new WebSetupRoutes object.
func CreateWebSetup(config *flamenco.Conf, flamencoVersion string) *Routes {
	return &Routes{
		config,
		flamencoVersion,
	}
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

// Merges 'two' into 'one'
func merge(one map[string]interface{}, two map[string]interface{}) {
	for key := range two {
		one[key] = two[key]
	}
}

func (web *Routes) showTemplate(templfname string, w http.ResponseWriter, r *http.Request, templateData TemplateData) {
	tmpl, err := template.ParseFiles(templfname)
	if err != nil {
		log.Errorf("Error parsing HTML template %s: %s", templfname, err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	usedData := TemplateData{
		"Version": web.flamencoVersion,
		"Config":  web.config,
	}
	merge(usedData, templateData)

	tmpl.Execute(w, usedData)
}

// addWebSetupRoutes registers HTTP endpoints for setup mode.
func (web *Routes) addWebSetupRoutes(router *mux.Router) {
	router.HandleFunc("/setup", web.httpIndex)
	router.HandleFunc("/setup/api/link-required", web.apiLinkRequired)

	static := noDirListing(http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	router.PathPrefix("/static/").Handler(static).Methods("GET")
}

func (web *Routes) httpIndex(w http.ResponseWriter, r *http.Request) {
	web.showTemplate("templates/websetup/index.html", w, r, nil)
}

// Check connection to Flamenco Server. Response indicates whether (re)linking is necessary.
func (web *Routes) apiLinkRequired(w http.ResponseWriter, r *http.Request) {
	resp := linkRequiredResponse{
		Required: LinkRequired(web.config),
	}
	if web.config.Flamenco != nil {
		resp.ServerURL = web.config.Flamenco.String()
	}

	w.Header().Set("Content-Type", "application/json")

	encoder := json.NewEncoder(w)
	if err := encoder.Encode(resp); err != nil {
		log.Warningf("%s: error encoding JSON response: %s", r.URL, err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
