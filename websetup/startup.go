package websetup

import (
	"fmt"

	"github.com/armadillica/flamenco-manager/flamenco"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

const webroot = "/setup"

// RestartFunction is called when a restart is requested via the web interface.
type RestartFunction func()

// EnterSetupMode registers HTTP endpoints and logs which URLs are available to visit it.
func EnterSetupMode(config *flamenco.Conf, flamencoVersion string, router *mux.Router) (*Routes, error) {
	log.Info("Entering setup mode")

	urls, err := availableURLs(config, false)
	if err != nil {
		return nil, fmt.Errorf("Unable to find any network address: %s", err)
	}

	log.Info("Point your browser at any of these URLs:")
	for _, url := range urls {
		setupURL, err := url.Parse(webroot)
		if err != nil {
			log.Warning("Unable to append web root %s to URL %s: %s", webroot, setupURL, err)
		}
		log.Infof("  - %s", setupURL)
	}

	// We don't need to return a reference to this object, since it's still referred to by the
	// muxer.
	web := createWebSetup(config, flamencoVersion)
	web.addWebSetupRoutes(router)

	return web, nil
}
