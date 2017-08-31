package websetup

import (
	"encoding/json"
	"flamenco-manager/flamenco"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// Routes handles all HTTP routes and server-side context for the web setup wizard.
type Routes struct {
	config          *flamenco.Conf
	flamencoVersion string
	linker          *ServerLinker
}

// TemplateData is the mapping type we use to pass data to the template engine.
type TemplateData map[string]interface{}

// CreateWebSetup creates a new WebSetupRoutes object.
func CreateWebSetup(config *flamenco.Conf, flamencoVersion string) *Routes {
	return &Routes{
		config,
		flamencoVersion,
		nil,
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

// Sends JSON without letting the remote end know if encoding failed.
func sendJSONnoCheck(w http.ResponseWriter, r *http.Request, payload interface{}) error {
	w.Header().Set("Content-Type", "application/json")

	encoder := json.NewEncoder(w)
	if err := encoder.Encode(payload); err != nil {
		log.Warningf("%s: error encoding JSON response: %s", r.URL, err)
		return err
	}

	return nil
}

func sendErrorMessage(w http.ResponseWriter, r *http.Request, status int, msg string, args ...interface{}) error {
	urlPrefix := fmt.Sprintf("%s: ", r.URL)
	formattedMessage := fmt.Sprintf(msg, args...)
	log.Error(urlPrefix + formattedMessage)

	w.WriteHeader(status)
	_, err := fmt.Fprint(w, formattedMessage)
	return err
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
	router.HandleFunc("/setup/api/link-start", web.apiLinkStart)

	static := noDirListing(http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	router.PathPrefix("/static/").Handler(static).Methods("GET")
}

func (web *Routes) httpIndex(w http.ResponseWriter, r *http.Request) {
	web.showTemplate("templates/websetup/index.html", w, r, nil)
}

// Check connection to Flamenco Server. Response indicates whether (re)linking is necessary.
func (web *Routes) apiLinkRequired(w http.ResponseWriter, r *http.Request) {
	payload := linkRequiredResponse{
		Required: LinkRequired(web.config),
	}
	if web.config.Flamenco != nil {
		payload.ServerURL = web.config.Flamenco.String()
	}

	sendJSONnoCheck(w, r, payload)
}

// Starts the linking process, should result in a redirect to Server.
func (web *Routes) apiLinkStart(w http.ResponseWriter, r *http.Request) {
	// Refuse to run if there is already an active linker.
	if web.linker != nil {
		sendErrorMessage(w, r, http.StatusExpectationFailed,
			"the linking process is already running. If this is incorrect, restart Flamenco Manager.")
		return
	}

	serverURL := r.FormValue("server")
	if serverURL == "" {
		sendErrorMessage(w, r, http.StatusBadRequest, "No server URL given")
		return
	}

	linker, err := StartLinking(serverURL)
	if err != nil {
		sendErrorMessage(w, r, http.StatusInternalServerError, "the linking process cannot start: %s", err)
		return
	}

	err = linker.ExchangeKey()
	if err != nil {
		sendErrorMessage(w, r, http.StatusInternalServerError, "unable to exchange secret key: %s", err)
		return
	}

	// Redirect the user to the Flamenco Server to log in and create/choose a Manager.
	log.Infof("%s: going to link to %s", r.URL, linker.upstream)
	redirectURL, err := linker.redirectURL()
	if err != nil {
		log.Errorf("ERROR: %v", err)
		log.Errorf("r.URL: %v", r.URL)
		sendErrorMessage(w, r, http.StatusInternalServerError, "error constructing URL to redirect to: %s", err)
		return
	}
	log.Infof("%s: redirecting user to %s", r.URL, redirectURL)

	// Store the linker object in our memory. The server shouldn't be restarted while linking.
	web.linker = linker

	sendJSONnoCheck(w, r, linkStartResponse{redirectURL.String()})
}
