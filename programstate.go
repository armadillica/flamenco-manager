package main

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
	"context"
	"flag"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/armadillica/flamenco-manager/flamenco"
	"github.com/armadillica/flamenco-manager/flamenco/bundledmongo"
	"github.com/armadillica/flamenco-manager/jwtauth"
	"github.com/armadillica/flamenco-manager/shaman"
	log "github.com/sirupsen/logrus"
	"gitlab.com/blender-institute/gossdp"
	mgo "gopkg.in/mgo.v2"
)

var (
	blacklist         *flamenco.WorkerBlacklist
	config            flamenco.Conf
	httpServer        *http.Server
	latestImageSystem *flamenco.LatestImageSystem
	mongoRunner       *bundledmongo.Runner
	session           *mgo.Session
	sleeper           *flamenco.SleepScheduler
	ssdp              *gossdp.Ssdp
	taskCleaner       *flamenco.TaskCleaner
	taskLogUploader   *flamenco.TaskLogUploader
	taskScheduler     *flamenco.TaskScheduler
	taskUpdatePusher  *flamenco.TaskUpdatePusher
	taskUpdateQueue   *flamenco.TaskUpdateQueue
	timeoutChecker    *flamenco.TimeoutChecker
	upstream          *flamenco.UpstreamConnection
	upstreamNotifier  *flamenco.UpstreamNotifier
	workerRemover     *flamenco.WorkerRemover

	shamanServer *shaman.Server
)

var shutdownComplete chan struct{}
var httpShutdownComplete chan struct{}

func shutdown(signum os.Signal) {
	shutdownDone := make(chan bool)

	go func() {
		log.WithField("signal", signum).Info("Signal received, shutting down.")

		// ImageWatcher allows long-living HTTP connections, so it
		// should be shut down before the HTTP server.
		if latestImageSystem != nil {
			latestImageSystem.Close()
		}

		if ssdp != nil {
			log.Info("Shutting down UPnP/SSDP advertisement")
			ssdp.Stop()
		}

		if httpServer != nil {
			log.Info("Shutting down HTTP server")
			// the Shutdown() function seems to hang sometime, even though the
			// main goroutine continues execution after ListenAndServe().
			go httpServer.Shutdown(context.Background())
			<-httpShutdownComplete
		} else {
			log.Warning("HTTP server was not even started yet")
		}

		if shamanServer != nil {
			shamanServer.Close()
		}
		jwtauth.CloseKeyStore()

		if timeoutChecker != nil {
			timeoutChecker.Close()
		}
		if taskUpdatePusher != nil {
			taskUpdatePusher.Close()
		}
		if taskLogUploader != nil {
			taskLogUploader.Close()
		}
		if upstream != nil {
			upstream.Close()
		}
		if workerRemover != nil {
			workerRemover.Close()
		}
		if mongoRunner != nil {
			mongoRunner.Close(session)
		}
		if session != nil {
			session.Close()
		}

		shutdownDone <- true
	}()

	// Force shutdown after a bit longer than the HTTP server timeout.
	select {
	case <-shutdownDone:
		break
	case <-time.After(17 * time.Second):
		log.Error("Shutdown forced, stopping process.")
		os.Exit(-2)

	}

	log.Warning("Shutdown complete, stopping process.")
	close(shutdownComplete)
}

var cliArgs struct {
	verbose    bool
	quiet      bool
	debug      bool
	jsonLog    bool
	cleanSlate bool
	purgeQueue bool
	version    bool
	setup      bool
	killPID    int

	garbageCollect    bool
	iKnowWhatIAmDoing bool

	// Run mode, see validModes in flamenco/settings.go
	mode string
}

func parseCliArgs() {
	flag.BoolVar(&cliArgs.verbose, "verbose", false, "Ignored as this is now the default")
	flag.BoolVar(&cliArgs.quiet, "quiet", false, "Disable info-level logging")
	flag.BoolVar(&cliArgs.debug, "debug", false, "Enable debug-level logging")
	flag.BoolVar(&cliArgs.jsonLog, "json", false, "Log in JSON format")
	flag.BoolVar(&cliArgs.cleanSlate, "cleanslate", false, "Start with a clean slate; erases all tasks from the local MongoDB")
	flag.BoolVar(&cliArgs.purgeQueue, "purgequeue", false, "Purges all queued task updates from the local MongoDB")
	flag.BoolVar(&cliArgs.version, "version", false, "Show the version of Flamenco Manager")
	flag.BoolVar(&cliArgs.setup, "setup", false, "Enter setup mode, enabling the web-based configuration system")

	flag.BoolVar(&cliArgs.garbageCollect, "gc", false, "Runs the Shaman garbage collector in dry-run mode, then exits.")
	flag.BoolVar(&cliArgs.iKnowWhatIAmDoing, "i-know-what-i-am-doing", false,
		"Together with -gc runs the garbage collector for real (so DELETES FILES), then exits.")

	flag.StringVar(&cliArgs.mode, "mode", "", "Run mode, either 'develop' or 'production'. Overrides the 'mode' in the configuration file.")
	if runtime.GOOS == "windows" {
		flag.IntVar(&cliArgs.killPID, "kill-after-start", 0, "Used on Windows for restarting the daemon")
	}
	flag.Parse()
}

func configLogging() {
	if cliArgs.jsonLog {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
		})
	}

	// Only log the warning severity or above.
	level := log.InfoLevel
	if cliArgs.debug {
		level = log.DebugLevel
	} else if cliArgs.quiet {
		level = log.WarnLevel
	}
	log.SetLevel(level)
}
