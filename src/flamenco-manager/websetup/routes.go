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

func (web *Routes) showTemplate(templfname string, w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles(templfname)
	if err != nil {
		log.Errorf("Error parsing HTML template %s: %s", templfname, err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Version": web.flamencoVersion,
		"Config":  web.config,
	}

	tmpl.Execute(w, data)
}

// addWebSetupRoutes registers HTTP endpoints for setup mode.
func (web *Routes) addWebSetupRoutes(router *mux.Router) {
	router.HandleFunc("/setup", web.httpIndex)
	router.HandleFunc("/setup/api/link-required", web.apiLinkRequired)

	static := noDirListing(http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	router.PathPrefix("/static/").Handler(static).Methods("GET")
}

func (web *Routes) httpIndex(w http.ResponseWriter, r *http.Request) {
	web.showTemplate("templates/websetup/index.html", w, r)
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
