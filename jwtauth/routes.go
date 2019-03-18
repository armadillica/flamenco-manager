package jwtauth

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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/mux"
)

const (
	jwtRequestExpiry = 30 * time.Second

	// %s is replaced by our manager ID.
	jwtServerURL = "api/flamenco/jwt/generate-token/%s"
)

// Redirector redirects a HTTP client to the URL on Flamenco Server to get a JWT token.
type Redirector struct {
	managerID string
	hmacKey   []byte
	server    *url.URL
}

// RedirectorResponse is sent to the browser when it asks for a token.
type RedirectorResponse struct {
	// Where to get the token.
	TokenURL string `json:"tokenURL"`
	// Where to send the browser if the Token URL sends a 403 Forbidden.
	LoginURL string `json:"loginURL"`
}

// NewRedirector creates a new Redirector instance.
func NewRedirector(managerID, managerSecret string, flamencoServer *url.URL) *Redirector {
	return &Redirector{
		managerID,
		[]byte(managerSecret),
		flamencoServer,
	}
}

// AddRoutes adds HTTP routes to the muxer.
func (red *Redirector) AddRoutes(router *mux.Router) {
	router.HandleFunc("/jwt/token-urls", red.routeGetTokenURLs).Methods("GET")
}

// Construct the URL back to the dashboard.
func (red *Redirector) dashboardURL(r *http.Request) string {
	dashURL := url.URL{
		Host: r.Host,
		Path: "/",
	}

	if r.TLS == nil {
		dashURL.Scheme = "http"
	} else {
		dashURL.Scheme = "https"
	}

	return dashURL.String()
}

// Return the URLs users can use to get a JWT token and to log in at the server.
// The 'get token' URL is only valid for a short time.
func (red *Redirector) routeGetTokenURLs(w http.ResponseWriter, r *http.Request) {
	logger := packageLogger.WithFields(RequestLogFields(r))
	if red.server == nil {
		logger.Error("no Flamenco Server URL set, unable to redirect user to get JWT token")
		http.Error(w, "No Flamenco Server URL set", http.StatusServiceUnavailable)
		return
	}

	// Compute the deadline for the request, and HMAC it with our secret to make
	// the Server trust the redirect.
	expires := time.Now().In(time.UTC).Add(jwtRequestExpiry).Format(time.RFC3339)
	hasher := hmac.New(sha256.New, red.hmacKey)
	hasher.Write([]byte(expires + "-" + red.managerID))
	computedHMAC := hasher.Sum(nil)

	// Construct the URL to redirect to.
	urlQuery := url.Values{}
	urlQuery.Set("expires", expires)
	urlQuery.Set("hmac", hex.EncodeToString(computedHMAC))
	tokenURL, err := red.server.Parse(fmt.Sprintf(jwtServerURL, red.managerID))
	if err != nil {
		logger.WithError(err).Error("unable to construct JWT URL to Flamenco Server")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	tokenURL.RawQuery = urlQuery.Encode()
	redirectTo := tokenURL.String()

	loginURL, err := red.server.Parse("login")
	if err != nil {
		logger.WithError(err).Error("unable to construct URL to Flamenco Server login endpoint")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	urlQuery = url.Values{}
	urlQuery.Set("next", red.dashboardURL(r))
	loginURL.RawQuery = urlQuery.Encode()

	logger.Debug("redirecting user to obtain JWT token")

	response := RedirectorResponse{
		TokenURL: redirectTo,
		LoginURL: loginURL.String(),
	}
	payload, err := json.Marshal(response)
	if err != nil {
		logger.WithError(err).Error("unable to construct JSON response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}
