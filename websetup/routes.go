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

package websetup

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/armadillica/flamenco-manager/jwtauth"

	"github.com/armadillica/flamenco-manager/flamenco"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

// End points at this Manager
const (
	setupURL           = "/setup"
	setupDataURL       = "/setup/data"
	saveConfigURL      = "/setup/save-configuration"
	apiLinkRequiredURL = "/setup/api/link-required"
	apiLinkStartURL    = "/setup/api/link-start"
	linkReturnURL      = "/setup/link-return"
	linkDoneURL        = "/setup/link-done"
	restartURL         = "/setup/restart"
	restartToSetupURL  = "/setup/restart-to-setup"
)

// Routes handles all HTTP routes and server-side context for the web setup wizard.
type Routes struct {
	config          *flamenco.Conf
	flamencoVersion string
	linker          *ServerLinker
	RestartFunction func(enterSetup bool)
	root            string
}

// TemplateData is the mapping type we use to pass data to the template engine.
type TemplateData map[string]interface{}

// createWebSetup creates a new WebSetupRoutes object.
func createWebSetup(config *flamenco.Conf, flamencoVersion string) *Routes {
	return &Routes{
		config,
		flamencoVersion,
		nil,
		nil,
		flamenco.TemplatePathPrefix("templates/layout.html"),
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

func (web *Routes) showTemplate(templfname string, w http.ResponseWriter, r *http.Request, templateData TemplateData) {
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
		web.root+"templates/layout.html",
		web.root+templfname)
	if err != nil {
		log.WithFields(log.Fields{
			"template":   templfname,
			log.ErrorKey: err,
		}).Error("Error parsing HTML template")
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// TODO(Sybren): cache this in memory and check mtime.
	vueTemplates, err := ioutil.ReadFile(web.root + "static/websetup/vue-components.html")
	if err != nil {
		log.WithError(err).Error("Error loading Vue.js templates")
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	usedData := TemplateData{
		"Version":      web.flamencoVersion,
		"Config":       web.config,
		"VueTemplates": template.HTML(vueTemplates),
	}
	merge(usedData, templateData)

	err = tmpl.ExecuteTemplate(w, "layout", usedData)
	if err != nil {
		log.WithFields(log.Fields{
			"template":   templfname,
			log.ErrorKey: err,
		}).Error("Error executing HTML template")
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}
}

// addWebSetupRoutes registers HTTP endpoints for setup mode.
func (web *Routes) addWebSetupRoutes(router *mux.Router, userauth jwtauth.Authenticator) {
	router.HandleFunc("/", web.httpIndex)
	router.HandleFunc(setupURL, web.httpSetupIndex).Methods("GET")

	router.Handle(setupDataURL, userauth.WrapFunc(web.httpSetupData)).Methods("GET")
	router.Handle(setupDataURL, userauth.WrapFunc(web.httpSaveYAML)).Methods("POST")

	router.Handle(apiLinkRequiredURL, userauth.WrapFunc(web.apiLinkRequired))
	router.Handle(apiLinkStartURL, userauth.WrapFunc(web.apiLinkStart))
	router.Handle(linkReturnURL, userauth.WrapFunc(web.httpLinkReturn))
	router.Handle(linkDoneURL, userauth.WrapFunc(web.httpLinkDone))

	router.Handle(restartURL, userauth.WrapFunc(web.httpRestart)).Methods("GET", "POST")
	router.Handle(restartToSetupURL, userauth.WrapFunc(web.httpRestartToSetup)).Methods("POST")

	// The last-rendered image is private, and not used in web setup, so mask it out.
	router.HandleFunc("/static/latest-image.jpg", http.NotFound).Methods("GET")
	static := noDirListing(http.StripPrefix("/static/", http.FileServer(http.Dir(web.root+"static"))))
	router.PathPrefix("/static/").Handler(static).Methods("GET")
}

func (web *Routes) httpIndex(w http.ResponseWriter, r *http.Request) {
	web.showTemplate("templates/websetup/setup-mode-enabled.html", w, r, nil)
}

func (web *Routes) httpSetupIndex(w http.ResponseWriter, r *http.Request) {
	urls := urlConfigOptions(web.config, r)

	// Set a default "own URL" when entering the setup.
	if web.config.OwnURL == "" {
		log.Infof("Own URL is not configured, choosing one based on the current request")
		for _, url := range urls {
			if url.IsUsedForSetup {
				web.config.OwnURL = url.URL
				break
			}
		}
	}

	// Avoid having a nil pointer in the template.
	websetupConfig := web.config.Websetup
	if websetupConfig == nil {
		websetupConfig = &flamenco.WebsetupConf{}
	}

	web.showTemplate("templates/websetup/index.html", w, r, TemplateData{
		"OwnURLs":        urls,
		"WebsetupConfig": websetupConfig,
	})
}

func (web *Routes) httpSetupData(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("remote_addr", r.RemoteAddr)
	urls := urlConfigOptions(web.config, r)

	// Set a default "own URL" when entering the setup.
	if web.config.OwnURL == "" {
		logger.Info("Own URL is not configured, choosing one based on the current request")
		for _, url := range urls {
			if url.IsUsedForSetup {
				web.config.OwnURL = url.URL
				break
			}
		}
	}

	payload := setupData{
		OwnURLs: urls,
		Config:  *web.config,
		// TODO: Include mtime of config file so that saving can require If-Unmodified-Since header.
	}
	payload.Config.ManagerSecret = ""

	asBytes, err := yaml.Marshal(payload)
	if err != nil {
		logger.WithError(err).Error("unable to create YAML response")
		http.Error(w, "Error creating YAML response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.Write(asBytes)
}

func (web *Routes) httpSaveYAML(w http.ResponseWriter, r *http.Request) {
	logger := log.WithFields(jwtauth.RequestLogFields(r))

	ct := r.Header.Get("Content-Type")
	if ct != "application/x-yaml" {
		http.Error(w, "Expecting application/x-yaml content type", http.StatusNotAcceptable)
		logger.WithField("contentTYpe", ct).Warning("invalid content type received")
		return
	}

	logger.Info("receiving new YAML configuration file")

	config := *web.config
	dec := yaml.NewDecoder(r.Body)

	// Send YAML decoding errors as-is back to the HTTP client.
	// They need to know what they did wrong.
	if err := dec.Decode(&config); err != nil {
		logger.WithError(err).Warning("unable to decode user-supplied YAML")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: Require If-Unmodified-Since header + config file mtime check.

	// Errors saving the config file shouldn't be sent back, because
	// those contain potentially security-sensitive information.
	if err := config.Overwrite(); err != nil {
		logger.WithError(err).Error("unable to save new configuration file from user-supplied YAML")
		http.Error(w, "Unable to save configuration", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

func (web *Routes) httpRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		web.showTemplate("templates/websetup/restart.html", w, r, nil)
		return
	}

	web.showTemplate("templates/websetup/restarting.html", w, r, nil)
	logger := log.WithField("remote_addr", r.RemoteAddr)
	logger.Warning("Restarting Flamenco Manager by request of the web setup.")

	go func() {
		// Give the browser some time to load static files for the template, before shutting down.
		time.Sleep(1 * time.Second)

		if web.RestartFunction == nil {
			logger.Error("Unable to restart Flamenco Manager, no restart function was registered.")
			return
		}

		web.RestartFunction(false)
	}()
}

func (web *Routes) httpRestartToSetup(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("remote_addr", r.RemoteAddr)
	if web.RestartFunction == nil {
		logger.Error("web setup has no restart function configured")
		http.Error(w, "Unable to restart Flamenco Manager, no restart function was registered.", http.StatusInternalServerError)
		return
	}
	logger.Warning("Restarting Flamenco Manager into setup mode by request of the web setup.")
	w.WriteHeader(http.StatusNoContent)
	web.RestartFunction(true)
}
