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
	"crypto/hmac"
	"encoding/hex"
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"
)

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

func sendErrorMessage(w http.ResponseWriter, r *http.Request, status int, msg string, args ...interface{}) {
	formattedMessage := fmt.Sprintf(msg, args...)
	log.Error(r.RequestURI + ": " + formattedMessage)
	http.Error(w, formattedMessage, status)
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

func (web *Routes) httpLinkReturn(w http.ResponseWriter, r *http.Request) {
	// Check the HMAC to see if we can trust this request.
	mac := r.FormValue("hmac")
	oid := r.FormValue("oid")
	if mac == "" || oid == "" {
		sendErrorMessage(w, r, http.StatusBadRequest, "no mac or oid received")
		return
	}

	if web.linker == nil {
		log.Warning("Flamenco Manager restarted mid link procedure, redirecting to setup again")
		http.Redirect(w, r, setupURL, http.StatusSeeOther)
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
