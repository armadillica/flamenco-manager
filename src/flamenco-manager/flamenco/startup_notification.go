package flamenco

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
)

// Gives the system some time to start up (and open listening HTTP port)
const STARTUP_NOTIFICATION_INITIAL_DELAY = 500 * time.Millisecond

// Duration between consecutive retries of sending the startup notification.
const STARTUP_NOTIFICATION_RETRY = 30 * time.Second

// StartupNotifier sends a signal to Flamenco Server that we've started.
type StartupNotifier struct {
	closable
	config   *Conf
	upstream *UpstreamConnection
	session  *mgo.Session
}

// CreateStartupNotifier creates a new startup notifier.
func CreateStartupNotifier(config *Conf, upstream *UpstreamConnection, session *mgo.Session) *StartupNotifier {
	notifier := StartupNotifier{
		makeClosable(),
		config,
		upstream,
		session,
	}

	return &notifier
}

// Close performs a clean shutdown.
func (sn *StartupNotifier) Close() {
	log.Debugf("StartupNotifier: shutting down, waiting for shutdown to complete.")
	sn.closableCloseAndWait()
	log.Info("StartupNotifier: shutdown complete.")
}

// Go sends a StartupNotification document to upstream Flamenco Server.
// Keeps trying in a goroutine until the notification was succesful.
func (sn *StartupNotifier) Go() {
	notification := StartupNotification{
		ManagerURL:         sn.config.OwnURL,
		VariablesByVarname: sn.config.VariablesByVarname,
		NumberOfWorkers:    0,
	}

	url, err := sn.upstream.ResolveURL("/api/flamenco/managers/%s/startup", sn.config.ManagerID)
	if err != nil {
		panic(fmt.Sprintf("SendStartupNotification: unable to construct URL: %s\n", err))
	}

	go func() {
		// Register as a loop that responds to 'done' being closed.
		sn.closableAdd(1)
		defer sn.closableDone()

		mongoSession := sn.session.Copy()
		defer mongoSession.Close()

		db := mongoSession.DB("")

		timer := Timer("StartupNotifier", STARTUP_NOTIFICATION_RETRY,
			STARTUP_NOTIFICATION_INITIAL_DELAY, &sn.closable)

		for _ = range timer {
			log.Info("SendStartupNotification: trying to send notification.")

			// Send the notification
			notification.NumberOfWorkers = WorkerCount(db)
			err := sn.upstream.SendJSON("SendStartupNotification", "POST", url, &notification, nil)
			if err == nil {
				// Success!
				sn.closableCloseNotWait() // the timer will close the loop.
				continue
			}

			log.Warningf("SendStartupNotification: Unable to send, will retry later: %s", err)
		}

		log.Infof("SendStartupNotification: Done sending notification to upstream Flamenco")
	}()
}
