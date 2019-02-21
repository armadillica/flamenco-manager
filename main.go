package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	mgo "gopkg.in/mgo.v2"

	auth "github.com/abbot/go-http-auth"
	"github.com/kardianos/osext"
	log "github.com/sirupsen/logrus"

	"github.com/armadillica/flamenco-manager/flamenco"
	"github.com/armadillica/flamenco-manager/flamenco/bundledmongo"
	"github.com/armadillica/flamenco-manager/websetup"
	"github.com/gorilla/mux"
	"gitlab.com/blender-institute/gossdp"
)

const flamencoVersion = "2.4-dev3"
const ssdpServiceType = "urn:flamenco:manager:0"

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
)

var shutdownComplete chan struct{}
var httpShutdownComplete chan struct{}

func httpRegisterWorker(w http.ResponseWriter, r *http.Request) {
	mongoSess := session.Copy()
	defer mongoSess.Close()
	flamenco.RegisterWorker(w, r, mongoSess.DB(""))
}

func httpTaskRedirToServer(w http.ResponseWriter, r *http.Request) {
	taskID, err := flamenco.ObjectIDFromRequest(w, r, "task-id")
	if err != nil {
		return
	}

	serverURL, err := config.Flamenco.Parse("/flamenco/tasks/" + taskID.Hex())
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to construct URL: %s", err.Error()), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, serverURL.String(), http.StatusTemporaryRedirect)
}

func httpScheduleTask(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	taskScheduler.ScheduleTask(w, r)
}

func httpKick(w http.ResponseWriter, r *http.Request) {
	upstream.KickDownloader(false)
	fmt.Fprintln(w, "Kicked task downloader")
}

func httpTaskLog(w http.ResponseWriter, r *http.Request) {
	jobID, err := flamenco.ObjectIDFromRequest(w, r, "job-id")
	if err != nil {
		return
	}
	taskID, err := flamenco.ObjectIDFromRequest(w, r, "task-id")
	if err != nil {
		return
	}

	flamenco.ServeTaskLog(w, r, jobID, taskID, taskUpdateQueue)
}

func httpTaskUpdate(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	taskID, err := flamenco.ObjectIDFromRequest(w, &r.Request, "task-id")
	if err != nil {
		return
	}

	taskUpdateQueue.QueueTaskUpdateFromWorker(w, r, mongoSess.DB(""), taskID)
}

func httpTaskReturn(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	taskID, err := flamenco.ObjectIDFromRequest(w, &r.Request, "task-id")
	if err != nil {
		return
	}

	taskScheduler.ReturnTaskFromWorker(w, r, mongoSess.DB(""), taskID)
}

/**
 * Called by a worker, to check whether it is allowed to keep running this task.
 */
func httpWorkerMayRunTask(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	taskID, err := flamenco.ObjectIDFromRequest(w, &r.Request, "task-id")
	if err != nil {
		return
	}

	taskScheduler.WorkerMayRunTask(w, r, mongoSess.DB(""), taskID)
}

func httpWorkerAckStatusChange(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	vars := mux.Vars(&r.Request)
	ackStatus := vars["ack-status"]

	flamenco.WorkerAckStatusChange(w, r, mongoSess.DB(""), ackStatus)
}

func httpWorkerGetStatusChange(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	flamenco.WorkerGetStatusChange(w, r, mongoSess.DB(""))
}

func httpWorkerSignOn(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	flamenco.WorkerSignOn(w, r, mongoSess.DB(""), upstreamNotifier)
}

func httpWorkerSignOff(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	flamenco.WorkerSignOff(w, r, mongoSess.DB(""), taskScheduler)
}

func workerSecret(user, realm string) string {
	mongoSess := session.Copy()
	defer mongoSess.Close()

	return flamenco.WorkerSecret(user, mongoSess.DB(""))
}

func startSSDPServer() *gossdp.Ssdp {
	ssdpServer, err := gossdp.NewSsdpWithLogger(nil, log.StandardLogger())
	if err != nil {
		log.WithError(err).Fatal("Error creating UPnP/SSDP server to allow autodetection")
		return nil
	}

	log.Info("Starting UPnP/SSDP advertisement")

	// This will block until stop is called. so open it in a goroutine here
	go func() {
		ssdpServer.Start()
		log.Info("Shut down UPnP/SSDP advertisement")
	}()

	// Define the service we want to advertise
	serverDef := gossdp.AdvertisableServer{
		ServiceType: "urn:flamenco:manager:0", // define the service type
		DeviceUuid:  config.SSDPDeviceUUID,    // make this unique!
		Location:    config.OwnURL,            // this is the location of the service we are advertising
		MaxAge:      3600,                     // Max age this advertisment is valid for
	}
	ssdpServer.AdvertiseServer(serverDef)

	return ssdpServer
}

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
	debug      bool
	jsonLog    bool
	cleanSlate bool
	purgeQueue bool
	version    bool
	setup      bool
	killPID    int

	// Run mode, see validModes in flamenco/settings.go
	mode string
}

func parseCliArgs() {
	flag.BoolVar(&cliArgs.verbose, "verbose", false, "Enable info-level logging")
	flag.BoolVar(&cliArgs.debug, "debug", false, "Enable debug-level logging")
	flag.BoolVar(&cliArgs.jsonLog, "json", false, "Log in JSON format")
	flag.BoolVar(&cliArgs.cleanSlate, "cleanslate", false, "Start with a clean slate; erases all tasks from the local MongoDB")
	flag.BoolVar(&cliArgs.purgeQueue, "purgequeue", false, "Purges all queued task updates from the local MongoDB")
	flag.BoolVar(&cliArgs.version, "version", false, "Show the version of Flamenco Manager")
	flag.BoolVar(&cliArgs.setup, "setup", false, "Enter setup mode, enabling the web-based configuration system")
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
	level := log.WarnLevel
	if cliArgs.debug {
		level = log.DebugLevel
	} else if cliArgs.verbose {
		level = log.InfoLevel
	}
	log.SetLevel(level)
}

func normalMode() (*mux.Router, error) {
	if strings.TrimSpace(config.DatabaseURL) == "" {
		// TODO: see if we can find an available port rather than hoping for the best.
		localMongoPort := 27019
		config.DatabaseURL = fmt.Sprintf("mongodb://localhost:%d/flamanager", localMongoPort)

		mongoRunner = bundledmongo.CreateMongoRunner(config.DatabasePath, localMongoPort)
		if err := mongoRunner.Go(); err != nil {
			return nil, fmt.Errorf("Error starting MongoDB: %s", err)
		}
	}

	log.WithField("database_url", config.DatabaseURL).Info("MongoDB database server")
	log.WithField("flamenco", config.Flamenco).Info("Upstream Flamenco server")

	session = flamenco.MongoSession(&config)

	if cliArgs.cleanSlate {
		flamenco.CleanSlate(session.DB(""))
		log.Warning("Shutting down after performing clean slate")
		os.Exit(0)
		return nil, nil
	}

	if cliArgs.purgeQueue {
		flamenco.PurgeOutgoingQueue(session.DB(""))
		log.Warning("Shutting down after performing queue purge")
		os.Exit(0)
		return nil, nil
	}

	if config.HasTLS() {
		config.OwnURL = strings.Replace(config.OwnURL, "http://", "https://", 1)
	} else {
		config.OwnURL = strings.Replace(config.OwnURL, "https://", "http://", 1)
	}
	log.WithFields(log.Fields{
		"own_url": config.OwnURL,
		"listen":  config.Listen,
	}).Info("Starting up subsystems.")

	upstream = flamenco.ConnectUpstream(&config, session)
	upstreamNotifier = flamenco.CreateUpstreamNotifier(&config, upstream, session)
	blacklist = flamenco.CreateWorkerBlackList(&config, session)
	taskUpdateQueue = flamenco.CreateTaskUpdateQueue(&config, blacklist)
	sleeper = flamenco.CreateSleepScheduler(session)
	taskLogUploader = flamenco.CreateTaskLogUploader(&config, upstream)
	taskScheduler = flamenco.CreateTaskScheduler(&config, upstream, session, taskUpdateQueue, blacklist)
	taskUpdatePusher = flamenco.CreateTaskUpdatePusher(&config, upstream, session, taskUpdateQueue, taskLogUploader)
	timeoutChecker = flamenco.CreateTimeoutChecker(&config, session, taskUpdateQueue, taskScheduler)
	taskCleaner = flamenco.CreateTaskCleaner(&config, session)
	dashboard := flamenco.CreateDashboard(&config, session, sleeper, flamencoVersion)
	latestImageSystem = flamenco.CreateLatestImageSystem(config.WatchForLatestImage)
	workerRemover = flamenco.CreateWorkerRemover(&config, session)

	// Set up our own HTTP server
	workerAuthenticator := auth.NewBasicAuthenticator("Flamenco Manager", workerSecret)
	router := mux.NewRouter().StrictSlash(true)
	dashboard.AddRoutes(router)
	latestImageSystem.AddRoutes(router, workerAuthenticator)
	router.HandleFunc("/register-worker", httpRegisterWorker).Methods("POST")
	router.HandleFunc("/task", workerAuthenticator.Wrap(httpScheduleTask)).Methods("POST")
	router.HandleFunc("/tasks/{task-id}/update", workerAuthenticator.Wrap(httpTaskUpdate)).Methods("POST")
	router.HandleFunc("/tasks/{task-id}/return", workerAuthenticator.Wrap(httpTaskReturn)).Methods("POST")
	router.HandleFunc("/tasks/{task-id}/redir-to-server", httpTaskRedirToServer)
	router.HandleFunc("/may-i-run/{task-id}", workerAuthenticator.Wrap(httpWorkerMayRunTask)).Methods("GET")
	router.HandleFunc("/status-change", workerAuthenticator.Wrap(httpWorkerGetStatusChange)).Methods("GET")
	router.HandleFunc("/ack-status-change/{ack-status}", workerAuthenticator.Wrap(httpWorkerAckStatusChange)).Methods("POST")
	router.HandleFunc("/sign-on", workerAuthenticator.Wrap(httpWorkerSignOn)).Methods("POST")
	router.HandleFunc("/sign-off", workerAuthenticator.Wrap(httpWorkerSignOff)).Methods("POST")
	router.HandleFunc("/kick", httpKick)
	router.HandleFunc("/logfile/{job-id}/{task-id}", httpTaskLog)

	upstreamNotifier.SendStartupNotification()
	blacklist.EnsureDBIndices()

	sleeper.Go()
	taskUpdatePusher.Go()
	timeoutChecker.Go()
	taskCleaner.Go()
	latestImageSystem.Go()
	taskLogUploader.Go()
	if workerRemover != nil {
		workerRemover.Go()
	}

	// Make ourselves discoverable through SSDP.
	if config.SSDPDiscovery {
		ssdp = startSSDPServer()
	} else {
		log.Info("UPnP/SSDP auto-discovery was disabled in the configuration file.")
	}

	return router, nil
}

func setupMode() (*websetup.Routes, *mux.Router, error) {
	// Always do verbose logging while running setup mode. It wouldn't make sense to log normal
	// informative things (like the URLs available to access the server) at warning level just to
	// ensure visibility.
	cliArgs.verbose = true
	configLogging()

	router := mux.NewRouter().StrictSlash(true)
	web, err := websetup.EnterSetupMode(&config, flamencoVersion, router)

	return web, router, err
}

func showStartup() {
	// This *always* has to be logged.
	oldLevel := log.GetLevel()
	defer log.SetLevel(oldLevel)
	log.SetLevel(log.InfoLevel)
	log.WithField("version", flamencoVersion).Info("Starting Flamenco Manager")
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
		fmt.Println(flamencoVersion)
		return
	}

	configLogging()
	showStartup()
	killParentProcess()

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

func restart() {
	exename, err := osext.Executable()
	if err != nil {
		log.Fatal(err)
	}

	isWindows := runtime.GOOS == "windows"

	args := []string{}
	if cliArgs.debug {
		args = append(args, "-debug")
	} else if cliArgs.verbose {
		args = append(args, "-verbose")
	}
	if cliArgs.jsonLog {
		args = append(args, "-json")
	}
	if isWindows {
		args = append(args, "-kill-after-start")
		args = append(args, fmt.Sprintf("%d", syscall.Getpid()))
	}
	cmd := exec.Command(exename, args...)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logFields := log.Fields{
		"exename": exename,
		"args":    args,
	}
	if err = cmd.Start(); err != nil {
		log.WithFields(logFields).WithError(err).Fatal("Failed to launch new Manager")
	}
	log.WithFields(logFields).Info("Started another Flamenco Manager")

	// Give the other process time to start. This is required on Windows. Our child will kill us
	// when it has started succesfully.
	if isWindows {
		time.Sleep(15 * time.Second)
	}
}

func killParentProcess() {

	if cliArgs.killPID == 0 {
		return
	}

	logger := log.WithField("pid", cliArgs.killPID)

	proc, err := os.FindProcess(cliArgs.killPID)
	if err != nil {
		logger.Debug("Unable to find parent process, will not terminate it.")
		return
	}

	err = proc.Kill()
	if err != nil {
		logger.WithError(err).Warning("Unable to terminate parent process.")
	} else {
		logger.Debug("Parent process terminated.")
	}
}
