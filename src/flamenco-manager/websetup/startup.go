package websetup

import (
	"fmt"

	"flamenco-manager/flamenco"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

const webroot = "/setup"

// EnterSetupMode registers HTTP endpoints and logs which URLs are available to visit it.
func EnterSetupMode(config *flamenco.Conf, flamencoVersion string, router *mux.Router) error {
	log.Info("Entering setup mode")

	urls, err := availableURLs(config)
	if err != nil {
		return fmt.Errorf("Unable to find any network address: %s", err)
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
	web := CreateWebSetup(config, flamencoVersion)
	web.addWebSetupRoutes(router)

	return nil
}