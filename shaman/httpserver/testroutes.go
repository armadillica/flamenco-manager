package httpserver

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
	"fmt"
	"net/http"

	"github.com/armadillica/flamenco-manager/jwtauth"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

var userInfo = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	tokenSubject, ok := jwtauth.SubjectFromContext(r.Context())
	if !ok {
		fmt.Fprintf(w, "You are unknown to me")
		return
	}

	fmt.Fprintf(w, "You are subject %s", tokenSubject)
})

// RegisterTestRoutes registers some routes that should only be used for testing.
func RegisterTestRoutes(r *mux.Router, auther jwtauth.Authenticator) {
	// On the default page we will simply serve our static index page.
	r.Handle("/", http.FileServer(http.Dir("./views/")))

	// We will setup our server so we can serve static assest like images, css from the /static/{file} route
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	getTokenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString, err := auther.GenerateToken()
		if err != nil {
			logger := packageLogger.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"remoteAddr":    r.RemoteAddr,
				"requestURI":    r.RequestURI,
				"requestMethod": r.Method,
			})
			logger.Warning("unable to sign JWT")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("error signing token: %v", err)))
			return
		}

		w.Write([]byte(tokenString))
	})

	r.Handle("/get-token", getTokenHandler).Methods("GET")
	r.Handle("/my-info", auther.Wrap(userInfo)).Methods("GET")
}
