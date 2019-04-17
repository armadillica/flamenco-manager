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

package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/armadillica/flamenco-manager/flamenco"
	"github.com/armadillica/flamenco-manager/jwtauth"
	"github.com/armadillica/flamenco-manager/shaman"
	"github.com/armadillica/flamenco-manager/websetup"
	"github.com/gorilla/mux"
)

var applicationVersion = "set-during-build"

func garbageCollectMode() {
	config.Shaman.GarbageCollect.SilentlyDisable = true
	shamanServer = shaman.NewServer(config.Shaman, jwtauth.AlwaysDeny{})
	stats := shamanServer.GCStorage(!cliArgs.iKnowWhatIAmDoing)
	log.Debugf("ran GC: %#v", stats)
}

func showStartup() {
	// This *always* has to be logged.
	oldLevel := log.GetLevel()
	defer log.SetLevel(oldLevel)
	log.SetLevel(log.InfoLevel)
	log.WithField("version", applicationVersion).Info("Starting Flamenco Manager")

	if cliArgs.verbose {
		log.Warning("The -verbose CLI argument is deprecated. INFO-level logging is " +
			"enabled by default; use -quiet to only see warnings and errors.")
	}
}

func showFlamencoServerURL() {
	if config.Flamenco == nil {
		log.Warning("no Flamenco Server URL configured")
		return
	}

	// This *always* has to be logged.
	oldLevel := log.GetLevel()
	defer log.SetLevel(oldLevel)
	log.SetLevel(log.InfoLevel)
	log.WithField("url", config.Flamenco.String()).Info("Flamenco Server URL")
}

func main() {
	parseCliArgs()
	if cliArgs.version {
		fmt.Println(applicationVersion)
		return
	}

	configLogging()
	showStartup()
	platformSpecificPostStartup()

	defer func() {
		// If there was a panic, make sure we log it before quitting.
		if r := recover(); r != nil {
			log.Panic(r)
		}
	}()

	var err error
	config, err = flamenco.GetConf()
	if err != nil {
		if os.IsNotExist(err) {
			log.Warning("Flamenco Manager configuration file not found, entering setup mode.")
			cliArgs.setup = true
		} else {
			log.WithError(err).Fatal("Unable to load configuration")
		}
	}
	if config.ManagerID == "" {
		log.Warning("Flamenco Manager not yet linked, entering setup mode.")
		cliArgs.setup = true
	}

	if strings.TrimSpace(cliArgs.mode) != "" {
		config.OverrideMode(cliArgs.mode)
	} else {
		log.WithField("mode", config.Mode).Info("Run mode")
	}
	showFlamencoServerURL()

	var router *mux.Router
	var setup *websetup.Routes
	if cliArgs.setup {
		setup, router, err = setupMode()
	} else if cliArgs.garbageCollect {
		garbageCollectMode()
		return
	} else {
		router, err = normalMode()
	}
	if err != nil {
		log.WithError(err).Fatal("There was an error setting up Flamenco Manager for operation")
	}

	// Create the HTTP server before allowing the shutdown signal Handler
	// to exist. This prevents a race condition when Ctrl+C is pressed after
	// the http.Server is created, but before it is assigned to httpServer.
	httpServer = &http.Server{
		Addr:        config.Listen,
		Handler:     router,
		ReadTimeout: 15 * time.Second,
	}

	shutdownComplete = make(chan struct{})
	httpShutdownComplete = make(chan struct{})

	// Handle Ctrl+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		for signum := range c {
			// Run the shutdown sequence in a goroutine, so that multiple Ctrl+C presses can be handled in parallel.
			go shutdown(signum)
		}
	}()

	// Register the restart function when we're in setup mode
	doRestartAfterShutdown := false
	if setup != nil {
		setup.RestartFunction = func() {
			doRestartAfterShutdown = true
			cliArgs.setup = false // The Web Setup restarts to the Dashboard.
			shutdown(os.Interrupt)
		}
	}
	if dashboard != nil {
		dashboard.RestartFunction = func() {
			doRestartAfterShutdown = true
			cliArgs.setup = true // The Dashboard restarts to the Web Setup.
			shutdown(os.Interrupt)
		}
	}

	// Fall back to insecure server if TLS certificate/key is not defined.
	var httpError error
	if config.HasTLS() {
		httpError = httpServer.ListenAndServeTLS(config.TLSCert, config.TLSKey)
	} else {
		httpError = httpServer.ListenAndServe()
	}
	log.WithError(httpError).Warning("HTTP server stopped")
	close(httpShutdownComplete)

	log.Info("Waiting for shutdown to complete.")

	<-shutdownComplete

	if doRestartAfterShutdown {
		log.Warning("Restarting Flamenco Server")
		restart()
	}
}
