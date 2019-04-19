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
	"os"
	"strings"

	auth "github.com/abbot/go-http-auth"
	"github.com/armadillica/flamenco-manager/flamenco"
	"github.com/armadillica/flamenco-manager/flamenco/bundledmongo"
	"github.com/armadillica/flamenco-manager/jwtauth"
	"github.com/armadillica/flamenco-manager/shaman"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"gitlab.com/blender-institute/gossdp"
)

const ssdpServiceType = "urn:flamenco:manager:0"

func normalMode() (*mux.Router, error) {
	if strings.TrimSpace(config.DatabaseURL) == "" {
		// TODO: see if we can find an available port rather than hoping for the best.
		localMongoPort := 27019
		config.DatabaseURL = fmt.Sprintf("mongodb://127.0.0.1:%d/flamanager", localMongoPort)

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

	if config.HasCustomTLS() {
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
	taskUpdatePusher = flamenco.CreateTaskUpdatePusher(&config, upstream, session, taskUpdateQueue, taskLogUploader)
	taskScheduler = flamenco.CreateTaskScheduler(&config, upstream, session, taskUpdateQueue, blacklist, taskUpdatePusher)
	timeoutChecker = flamenco.CreateTimeoutChecker(&config, session, taskUpdateQueue, taskScheduler)
	taskCleaner = flamenco.CreateTaskCleaner(&config, session)
	dashboard = flamenco.CreateDashboard(&config, session, sleeper, blacklist, applicationVersion)
	latestImageSystem = flamenco.CreateLatestImageSystem(config.WatchForLatestImage)
	workerRemover = flamenco.CreateWorkerRemover(&config, session, taskScheduler)
	jwtRedirector := jwtauth.NewRedirector(config.ManagerID, config.ManagerSecret, config.Flamenco, "/")
	jwtAuther := jwtauth.Load(config.JWT)
	shamanServer = shaman.NewServer(config.Shaman, jwtAuther)

	// Set up our own HTTP server
	workerAuthenticator := auth.NewBasicAuthenticator("Flamenco Manager", workerSecret)
	registrationAuthenticator := flamenco.NewWorkerRegistrationAuthoriser(&config)
	router := mux.NewRouter().StrictSlash(true)
	dashboard.AddRoutes(router, shamanServer.Auther())
	latestImageSystem.AddRoutes(router, workerAuthenticator, jwtAuther)
	shamanServer.AddRoutes(router)
	if !config.JWT.DisableSecurity {
		jwtRedirector.AddRoutes(router)
	}
	AddRoutes(router, workerAuthenticator, jwtAuther, registrationAuthenticator)

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
	shamanServer.Go()

	if !config.JWT.DisableSecurity {
		jwtauth.GoDownloadLoop()
	}

	// Make ourselves discoverable through SSDP.
	if config.SSDPDiscovery {
		ssdp = startSSDPServer()
	} else {
		log.Info("UPnP/SSDP auto-discovery was disabled in the configuration file.")
	}

	return router, nil
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
