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
	"fmt"

	"github.com/armadillica/flamenco-manager/flamenco"
	"github.com/armadillica/flamenco-manager/jwtauth"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

const webroot = "/setup"

// RestartFunction is called when a restart is requested via the web interface.
type RestartFunction func()

// EnterSetupMode registers HTTP endpoints and logs which URLs are available to visit it.
func EnterSetupMode(config *flamenco.Conf, flamencoVersion string, router *mux.Router) (*Routes, error) {
	log.WithField("securityEnabled", !config.JWT.DisableSecurity).Info("Entering setup mode")

	// Disable security until we have a way to get our JWT keys from the server.
	var jwtAuther jwtauth.Authenticator
	linkRequired := LinkRequired(config)
	if linkRequired {
		log.Warning("This Manager is not yet linked to a Server, disabling security.")
		jwtAuther = jwtauth.AlwaysAllow{}
	} else {
		jwtAuther = jwtauth.Load(config.JWT)
		jwtRedirector := jwtauth.NewRedirector(config.ManagerID, config.ManagerSecret, config.Flamenco, "/setup")
		if !config.JWT.DisableSecurity {
			jwtRedirector.AddRoutes(router)
			jwtauth.GoDownloadLoop()
		}
	}

	urls, err := availableURLs(config, false)
	if err != nil {
		if err != ErrNoInterface {
			return nil, fmt.Errorf("Unable to find any network address: %s", err)
		}
		// Ignore ErrNoInterface errors, but if they occur, also don't show "Point your browser to..."
	} else {
		log.Info("Point your browser at any of these URLs:")
		for _, url := range urls {
			setupURL, err := url.Parse(webroot)
			if err != nil {
				log.WithFields(log.Fields{
					"webroot": webroot,
					"url":     setupURL.String(),
				}).WithError(err).Warning("Unable to append web root to URL", webroot, setupURL, err)
			}
			log.Infof("  - %s", setupURL)
		}
	}

	// We don't need to return a reference to this object, since it's still referred to by the
	// muxer.
	web := createWebSetup(config, flamencoVersion)
	web.addWebSetupRoutes(router, jwtAuther)

	return web, nil
}
