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
	"fmt"
	"hash"
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
	hmac      hash.Hash
	server    *url.URL
}

// NewRedirector creates a new Redirector instance.
func NewRedirector(managerID, managerSecret string, flamencoServer *url.URL) *Redirector {
	return &Redirector{
		managerID,
		hmac.New(sha256.New, []byte(managerSecret)),
		flamencoServer,
	}
}

// AddRoutes adds HTTP routes to the muxer.
func (red *Redirector) AddRoutes(router *mux.Router) {
	router.HandleFunc("/jwt/get-token", red.routeGetToken).Methods("GET")
}

// Redirect users to the URL they can get a JWT token.
func (red *Redirector) routeGetToken(w http.ResponseWriter, r *http.Request) {
	logger := packageLogger.WithFields(RequestLogFields(r))
	if red.server == nil {
		logger.Error("no Flamenco Server URL set, unable to redirect user to get JWT token")
		http.Error(w, "No Flamenco Server URL set", http.StatusServiceUnavailable)
		return
	}

	// Compute the deadline for the request, and HMAC it with our secret to make
	// the Server trust the redirect.
	expires := time.Now().In(time.UTC).Add(jwtRequestExpiry).Format(time.RFC3339)
	hmacPayload := expires + "-" + red.managerID
	red.hmac.Reset()
	red.hmac.Write([]byte(hmacPayload))
	hmac := red.hmac.Sum(nil)

	// Construct the URL to redirect to.
	urlQuery := url.Values{}
	urlQuery.Set("expires", expires)
	urlQuery.Set("hmac", hex.EncodeToString(hmac))
	url, err := red.server.Parse(fmt.Sprintf(jwtServerURL, red.managerID))
	if err != nil {
		logger.WithError(err).Error("unable to construct JWT URL to Flamenco Server")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	url.RawQuery = urlQuery.Encode()
	redirectTo := url.String()

	logger.WithField("redirectTo", redirectTo).Debug("redirecting user to obtain JWT token")
	http.Redirect(w, r, redirectTo, http.StatusTemporaryRedirect)
}
