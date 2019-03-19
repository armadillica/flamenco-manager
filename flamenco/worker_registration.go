package flamenco

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/armadillica/flamenco-manager/jwtauth"
	jwt "github.com/dgrijalva/jwt-go"
	log "github.com/sirupsen/logrus"
)

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

// RegistrationAuth is the interface for all worker registration authorisers.
type RegistrationAuth interface {
	Wrap(handler http.Handler) http.Handler
	WrapFunc(handlerFunc func(w http.ResponseWriter, r *http.Request)) http.Handler
}

// PSKRegAuth authorises based on JWT tokens signed with HMAC + a pre-shared key.
type PSKRegAuth struct {
	preSharedSecret []byte
}

// OpenRegAuth allows anybody to register as a Worker.
type OpenRegAuth struct{}

// NewWorkerRegistrationAuthoriser creates a new RegistrationAuth.
// If a pre-shared secret key is configured, creates a PSKRegAuth,
// otherwise creates an OpenRegAuth.
func NewWorkerRegistrationAuthoriser(config *Conf) RegistrationAuth {
	secret := strings.TrimSpace(config.WorkerRegistrationSecret)
	if secret == "" {
		log.Warn("worker_registration_secret setting is not set, worker registration is open")
		return OpenRegAuth{}
	}
	log.Info("Worker registration is secured by pre-shared key")
	return &PSKRegAuth{[]byte(secret)}
}

// WrapFunc does not do anything.
func (ora OpenRegAuth) WrapFunc(handlerFunc func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(handlerFunc)
}

// Wrap does not do anything.
func (ora OpenRegAuth) Wrap(handler http.Handler) http.Handler {
	return handler
}

// WrapFunc requires that the request is authenticated with the proper JWT bearer token.
func (pra *PSKRegAuth) WrapFunc(handlerFunc func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return pra.Wrap(http.HandlerFunc(handlerFunc))
}

// Wrap requires that the request is authenticated with the proper JWT bearer token.
func (pra *PSKRegAuth) Wrap(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := log.WithField("remote_addr", r.RemoteAddr)
		tokenString := jwtauth.GetBearerToken(r, logger)

		if tokenString == "" {
			logger.Warn("missing Bearer token, rejecting worker registration")
			http.Error(w, "Missing Bearer token for registration", http.StatusForbidden)
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}
			return pra.preSharedSecret, nil
		})

		if err != nil || !token.Valid {
			logger.WithError(err).Warn("invalid bearer token, rejecting worker registration")
			http.Error(w, "Invalid Bearer token for registration", http.StatusForbidden)
			return
		}

		handler.ServeHTTP(w, r)
	})
}
