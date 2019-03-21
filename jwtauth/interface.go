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

package jwtauth

import (
	"net/http"
	"os"
	"path"

	"github.com/sirupsen/logrus"
)

// StatusTokenExpired is the HTTP status code that's returned when an expired token is used.
const StatusTokenExpired = 498

// Authenticator is an interface for authenting HTTP wrappers.
type Authenticator interface {
	Wrap(handler http.Handler) http.Handler
	WrapFunc(handlerFunc func(w http.ResponseWriter, r *http.Request)) http.Handler
	GenerateToken() (string, error)
}

// Load JWT authentication keys from ./jwtkeys and create a new JWT authenticator.
func Load(conf Config) Authenticator {
	if conf.DisableSecurity {
		logrus.Warning("security is disabled, Flamenco Manager is open and will do anything requested by anyone")
		return AlwaysAllow{}
	}

	wd, err := os.Getwd()
	if err != nil {
		logrus.WithError(err).Fatal("unable to get current working directory")
	}
	LoadKeyStore(conf, path.Join(wd, "jwtkeys"))
	return NewJWT(true)
}
