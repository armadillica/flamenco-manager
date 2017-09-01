package websetup

import (
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flamenco-manager/flamenco"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// End points at this Manager
const (
	indexURL           = "/setup"
	saveConfigURL      = "/setup/save-configuration"
	apiLinkRequiredURL = "/setup/api/link-required"
	apiLinkStartURL    = "/setup/api/link-start"
	linkReturnURL      = "/setup/link-return"
	linkDoneURL        = "/setup/link-done"
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
	tmpl, err := template.Must(template.New("").Funcs(template.FuncMap{
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
	}), nil).ParseFiles(
		"templates/websetup/layout.html",
		"templates/websetup/vartable.html",
		templfname)
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

	err = tmpl.ExecuteTemplate(w, "layout", usedData)
	if err != nil {
		log.Errorf("Error executing HTML template %s: %s", templfname, err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}

// addWebSetupRoutes registers HTTP endpoints for setup mode.
func (web *Routes) addWebSetupRoutes(router *mux.Router) {
	router.HandleFunc(indexURL, web.httpIndex)
	router.HandleFunc(saveConfigURL, web.httpSaveConfig).Methods("POST")
	router.HandleFunc(apiLinkRequiredURL, web.apiLinkRequired)
	router.HandleFunc(apiLinkStartURL, web.apiLinkStart)
	router.HandleFunc(linkReturnURL, web.httpReturn)
	router.HandleFunc(linkDoneURL, web.httpLinkDone)

	static := noDirListing(http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	router.PathPrefix("/static/").Handler(static).Methods("GET")
}

func (web *Routes) httpIndex(w http.ResponseWriter, r *http.Request) {
	urls := urlConfigOptions(web.config, r)

	// Set a default "own URL" when entering the setup.
	if web.config.OwnURL == "" {
		log.Infof("Own URL is not configured, choosing one based on the current request")
		for _, url := range urls {
			if url.IsUsedForSetup {
				web.config.OwnURL = url.URL.String()
				break
			}
		}
	}

	web.showTemplate("templates/websetup/index.html", w, r, TemplateData{
		"OwnURLs": urls,
	})
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
	serverURL := r.FormValue("server")
	if serverURL == "" {
		sendErrorMessage(w, r, http.StatusBadRequest, "No server URL given")
		return
	}

	ourURL, err := ourURL(web.config, r)
	if err != nil {
		log.Errorf("Unable to parse request host %q: %s", r.Host, err)
		sendErrorMessage(w, r, http.StatusInternalServerError, "I don't know what you're doing")
		return
	}

	linker, err := StartLinking(serverURL, ourURL)
	if err != nil {
		sendErrorMessage(w, r, http.StatusInternalServerError, "the linking process cannot start: %s", err)
		return
	}

	// Server URL has been parsed correctly, so we can save it to our configuration file.
	web.config.Flamenco = linker.upstream
	web.config.Overwrite()

	err = linker.ExchangeKey()
	if err != nil {
		sendErrorMessage(w, r, http.StatusInternalServerError, "unable to exchange secret key: %s", err)
		return
	}

	// Redirect the user to the Flamenco Server to log in and create/choose a Manager.
	log.Infof("%s: going to link to %s", r.URL, linker.upstream)
	redirectURL, err := linker.redirectURL()
	if err != nil {
		sendErrorMessage(w, r, http.StatusInternalServerError, "error constructing URL to redirect to: %s", err)
		return
	}
	log.Infof("%s: redirecting user to %s", r.URL, redirectURL)

	// Store the linker object in our memory. The server shouldn't be restarted while linking.
	web.linker = linker

	sendJSONnoCheck(w, r, linkStartResponse{redirectURL.String()})
}

func (web *Routes) httpReturn(w http.ResponseWriter, r *http.Request) {
	// Check the HMAC to see if we can trust this request.
	mac := r.FormValue("hmac")
	oid := r.FormValue("oid")
	if mac == "" || oid == "" {
		sendErrorMessage(w, r, http.StatusBadRequest, "no mac or oid received")
		return
	}

	if web.linker == nil {
		log.Warning("Flamenco Manager restarted mid link procedure, redirecting to setup again")
		http.Redirect(w, r, indexURL, http.StatusSeeOther)
		return
	}

	msg := []byte(web.linker.identifier + "-" + oid)
	hash, err := web.linker.hmacObject()
	if err != nil {
		sendErrorMessage(w, r, http.StatusInternalServerError, "error constructing HMAC: %s", err)
		return
	}

	if _, err = hash.Write(msg); err != nil {
		sendErrorMessage(w, r, http.StatusInternalServerError, "error computing HMAC: %s", err)
		return
	}
	receivedMac, err := hex.DecodeString(mac)
	if err != nil {
		log.Errorf("Unable to decode received mac: %s", err)
		sendErrorMessage(w, r, http.StatusBadRequest, "bad HMAC")
		return
	}
	computedMac := hash.Sum(nil)
	if !hmac.Equal(receivedMac, computedMac) {
		sendErrorMessage(w, r, http.StatusBadRequest, "bad HMAC")
		return
	}

	// Remember our Manager ID and request a reset of our auth token.
	log.Infof("Our Manager ID is %s", oid)
	web.config.ManagerID = oid
	web.linker.managerID = oid

	log.Infof("Requesting new authentication token from Flamenco Server")
	token, err := web.linker.resetAuthToken()
	if err != nil {
		sendErrorMessage(w, r, http.StatusInternalServerError,
			"Unable to request a new authentication token from Flamenco Server: %s", err)
		return
	}
	log.Infof("Received new authentication token")
	web.config.ManagerSecret = token

	// Save our configuration file.
	if err = web.config.Overwrite(); err != nil {
		sendErrorMessage(w, r, http.StatusInternalServerError, "error saving configuration: %s", err)
		return
	}

	// Redirect to the "done" page
	http.Redirect(w, r, linkDoneURL, http.StatusSeeOther)
}

func (web *Routes) httpLinkDone(w http.ResponseWriter, r *http.Request) {
	web.showTemplate("templates/websetup/link-done.html", w, r, nil)
}

func parseVariables(formValue string) (map[string]map[string]string, error) {
	var variables []map[string]string
	if err := json.Unmarshal([]byte(formValue), &variables); err != nil {
		return nil, fmt.Errorf("Unable to parse variables: %s", err)
	}

	variablesByVarName := make(map[string]map[string]string)
	for _, variable := range variables {
		name := variable["name"]
		delete(variable, "name")
		variablesByVarName[name] = variable
	}
	log.Debugf("Parsed variables: %v", variablesByVarName)

	return variablesByVarName, nil
}

func (web *Routes) httpSaveConfig(w http.ResponseWriter, r *http.Request) {
	log.Infof("Merging configuration with POST data")

	web.config.DatabaseURL = r.FormValue("database-url")
	web.config.DatabasePath = r.FormValue("database-path")
	web.config.Listen = r.FormValue("listen")
	web.config.OwnURL = r.FormValue("own-url")
	web.config.SSDPDiscovery = r.FormValue("ssdp-discovery") != ""

	// Parse the posted variables.
	if vars, err := parseVariables(r.FormValue("variables")); err != nil {
		log.Error(err)
	} else {
		web.config.VariablesByVarname = vars
	}
	if vars, err := parseVariables(r.FormValue("path-variables")); err != nil {
		log.Error(err)
	} else {
		web.config.PathReplacementByVarname = vars
	}
	web.config.Overwrite()

	http.Redirect(w, r, indexURL, http.StatusSeeOther)
}
