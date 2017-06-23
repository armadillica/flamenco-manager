package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	auth "github.com/abbot/go-http-auth"
	log "github.com/sirupsen/logrus"

	"flamenco-manager/flamenco"

	"github.com/gorilla/mux"
)

const flamencoVersion = "2.0.11"

// MongoDB session
var session *mgo.Session
var config flamenco.Conf
var upstream *flamenco.UpstreamConnection
var taskScheduler *flamenco.TaskScheduler
var taskUpdatePusher *flamenco.TaskUpdatePusher
var timeoutChecker *flamenco.TimeoutChecker
var taskCleaner *flamenco.TaskCleaner
var startupNotifier *flamenco.StartupNotifier
var httpServer *http.Server
var latestImageSystem *flamenco.LatestImageSystem
var shutdownComplete chan struct{}
var httpShutdownComplete chan struct{}

func http_register_worker(w http.ResponseWriter, r *http.Request) {
	mongoSess := session.Copy()
	defer mongoSess.Close()
	flamenco.RegisterWorker(w, r, mongoSess.DB(""))
}

func http_schedule_task(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	taskScheduler.ScheduleTask(w, r)
}

func http_kick(w http.ResponseWriter, r *http.Request) {
	upstream.KickDownloader(false)
	fmt.Fprintln(w, "Kicked task downloader")
}

func http_task_update(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	vars := mux.Vars(&r.Request)
	taskID := vars["task-id"]

	if !bson.IsObjectIdHex(taskID) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Invalid ObjectID passed as task ID: %s\n", taskID)
		return
	}

	flamenco.QueueTaskUpdateFromWorker(w, r, mongoSess.DB(""), bson.ObjectIdHex(taskID))
}

/**
 * Called by a worker, to check whether it is allowed to keep running this task.
 */
func http_worker_may_run_task(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	vars := mux.Vars(&r.Request)
	taskID := vars["task-id"]

	if !bson.IsObjectIdHex(taskID) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Invalid ObjectID passed as task ID: %s\n", taskID)
		return
	}

	flamenco.WorkerMayRunTask(w, r, mongoSess.DB(""), bson.ObjectIdHex(taskID))
}

func http_worker_sign_on(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	flamenco.WorkerSignOn(w, r, mongoSess.DB(""))
}

func http_worker_sign_off(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	flamenco.WorkerSignOff(w, r, mongoSess.DB(""))
}

func worker_secret(user, realm string) string {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	return flamenco.WorkerSecret(user, mongoSess.DB(""))
}

func shutdown(signum os.Signal) {
	// Force shutdown after a bit longer than the HTTP server timeout.
	timeout := flamenco.TimeoutAfter(17 * time.Second)

	go func() {
		log.Infof("Signal '%s' received, shutting down.", signum)

		// ImageWatcher allows long-living HTTP connections, so it
		// should be shut down before the HTTP server.
		latestImageSystem.Close()

		if httpServer != nil {
			log.Info("Shutting down HTTP server")
			// the Shutdown() function seems to hang sometime, even though the
			// main goroutine continues execution after ListenAndServe().
			go httpServer.Shutdown(context.Background())
			<-httpShutdownComplete
		} else {
			log.Warning("HTTP server was not even started yet")
		}

		timeoutChecker.Close()
		taskUpdatePusher.Close()
		upstream.Close()
		session.Close()
		timeout <- false
	}()

	if <-timeout {
		log.Error("Shutdown forced, stopping process.")
		os.Exit(-2)
	}

	log.Warning("Shutdown complete, stopping process.")
	close(shutdownComplete)
}

var cliArgs struct {
	verbose    bool
	debug      bool
	jsonLog    bool
	cleanSlate bool
	version    bool
}

func parseCliArgs() {
	flag.BoolVar(&cliArgs.verbose, "verbose", false, "Enable info-level logging")
	flag.BoolVar(&cliArgs.debug, "debug", false, "Enable debug-level logging")
	flag.BoolVar(&cliArgs.jsonLog, "json", false, "Log in JSON format")
	flag.BoolVar(&cliArgs.cleanSlate, "cleanslate", false, "Start with a clean slate; erases all tasks from the local MongoDB")
	flag.BoolVar(&cliArgs.version, "version", false, "Show the version of Flamenco Manager")
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
	level := log.WarnLevel
	if cliArgs.debug {
		level = log.DebugLevel
	} else if cliArgs.verbose {
		level = log.InfoLevel
	}
	log.SetLevel(level)
}

func main() {
	parseCliArgs()
	if cliArgs.version {
		fmt.Println(flamencoVersion)
		return
	}

	configLogging()
	log.Infof("Starting Flamenco Manager version %s", flamencoVersion)

	defer func() {
		// If there was a panic, make sure we log it before quitting.
		if r := recover(); r != nil {
			log.Panic(r)
		}
	}()

	config = flamenco.GetConf()
	hasTLS := config.TLSCert != "" && config.TLSKey != ""
	if hasTLS {
		config.OwnURL = strings.Replace(config.OwnURL, "http://", "https://", 1)
	} else {
		config.OwnURL = strings.Replace(config.OwnURL, "https://", "http://", 1)
		log.Warning("WARNING: TLS not enabled!")
	}

	log.Info("MongoDB database server :", config.DatabaseURL)
	log.Info("Upstream Flamenco server:", config.Flamenco)
	log.Info("My URL is               :", config.OwnURL)
	log.Info("Listening at            :", config.Listen)

	session = flamenco.MongoSession(&config)

	if cliArgs.cleanSlate {
		flamenco.CleanSlate(session.DB(""))
		log.Warning("Shutting down after performing clean slate")
		return
	}

	upstream = flamenco.ConnectUpstream(&config, session)
	startupNotifier = flamenco.CreateStartupNotifier(&config, upstream, session)
	taskScheduler = flamenco.CreateTaskScheduler(&config, upstream, session)
	taskUpdatePusher = flamenco.CreateTaskUpdatePusher(&config, upstream, session)
	timeoutChecker = flamenco.CreateTimeoutChecker(&config, session)
	taskCleaner = flamenco.CreateTaskCleaner(&config, session)
	reporter := flamenco.CreateReporter(&config, session, flamencoVersion)
	latestImageSystem = flamenco.CreateLatestImageSystem(config.WatchForLatestImage)

	// Set up our own HTTP server
	workerAuthenticator := auth.NewBasicAuthenticator("Flamenco Manager", worker_secret)
	router := mux.NewRouter().StrictSlash(true)
	reporter.AddRoutes(router)
	latestImageSystem.AddRoutes(router, workerAuthenticator)
	router.HandleFunc("/register-worker", http_register_worker).Methods("POST")
	router.HandleFunc("/task", workerAuthenticator.Wrap(http_schedule_task)).Methods("POST")
	router.HandleFunc("/tasks/{task-id}/update", workerAuthenticator.Wrap(http_task_update)).Methods("POST")
	router.HandleFunc("/may-i-run/{task-id}", workerAuthenticator.Wrap(http_worker_may_run_task)).Methods("GET")
	router.HandleFunc("/sign-on", workerAuthenticator.Wrap(http_worker_sign_on)).Methods("POST")
	router.HandleFunc("/sign-off", workerAuthenticator.Wrap(http_worker_sign_off)).Methods("POST")
	router.HandleFunc("/kick", http_kick)

	startupNotifier.Go()
	taskUpdatePusher.Go()
	timeoutChecker.Go()
	taskCleaner.Go()
	latestImageSystem.Go()

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

	// Fall back to insecure server if TLS certificate/key is not defined.
	if !hasTLS {
		log.Warning(httpServer.ListenAndServe())
	} else {
		log.Warning(httpServer.ListenAndServeTLS(config.TLSCert, config.TLSKey))
	}
	close(httpShutdownComplete)

	log.Info("Waiting for shutdown to complete.")

	<-shutdownComplete
}
