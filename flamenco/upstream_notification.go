package flamenco

/* Notifcations sent to Flamenco Server.
 *
 * There are two notifications sent, with pretty much the same content.
 * The URL the notification is sent to determines the semantics. One is
 * the startup notification, and the other is to tell the Server that the
 * supported task types changed.
 */

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// Gives the system some time to start up (and open listening HTTP port)
const startupNotificationInitialDelay = 500 * time.Millisecond

// Duration between consecutive retries of sending the startup notification.
const startupNotificationRetry = 30 * time.Second

// Duration between consecutive retries of sending the task types notification.
const taskTypesNotificationRetry = 10 * time.Second

// UpstreamNotifier sends a signal to Flamenco Server that we've started or changed configuration.
type UpstreamNotifier struct {
	closable
	startupClosable closable

	taskTypesMutex    *sync.Mutex
	taskTypesClosable *closable

	config   *Conf
	upstream *UpstreamConnection
	session  *mgo.Session
}

// CreateUpstreamNotifier creates a new notifier.
func CreateUpstreamNotifier(config *Conf, upstream *UpstreamConnection, session *mgo.Session) *UpstreamNotifier {
	notifier := UpstreamNotifier{
		makeClosable(),
		makeClosable(),

		new(sync.Mutex),
		nil,
		config,
		upstream,
		session,
	}

	return &notifier
}

// Close performs a clean shutdown.
func (un *UpstreamNotifier) Close() {
	log.Debugf("UpstreamNotifier: shutting down, waiting for shutdown to complete.")
	un.startupClosable.closableCloseAndWait()

	un.taskTypesMutex.Lock()
	if un.taskTypesClosable != nil {
		un.taskTypesClosable.closableCloseAndWait()
	}
	un.taskTypesMutex.Unlock()

	un.closableCloseAndWait()
	log.Info("UpstreamNotifier: shutdown complete.")
}

func (un *UpstreamNotifier) constructNotification(db *mgo.Database) UpstreamNotification {

	notification := UpstreamNotification{
		ManagerURL:               un.config.OwnURL,
		VariablesByVarname:       un.config.VariablesByVarname,
		PathReplacementByVarname: un.config.PathReplacementByVarname,
		NumberOfWorkers:          WorkerCount(db),
		WorkerTaskTypes:          []string{},
	}

	logger := log.WithField("number_of_workers", notification.NumberOfWorkers)

	coll := db.C("flamenco_workers")
	err := coll.Find(bson.M{}).Distinct("supported_task_types", &notification.WorkerTaskTypes)
	if err != nil {
		logger.WithError(err).Error("UpstreamNotifier: unable to find supported task types for workers")
	} else {
		logger.WithField("task_types", notification.WorkerTaskTypes).Info("constructed upstream notification")
	}

	return notification
}

func (un *UpstreamNotifier) sendNotification(url *url.URL,
	timer <-chan struct{}, timerClosable *closable) {

	logger := log.WithField("url", url.String())

	go func() {
		// Register as a loop that responds to 'done' being closed.
		un.closableAdd(1)
		defer un.closableDone()

		mongoSession := un.session.Copy()
		defer mongoSession.Close()
		db := mongoSession.DB("")

		for range timer {
			logger.Info("trying to send notification")

			// Send the notification. The notification object is constructed inside the timer loop,
			// so that it's as up-to-date as possible, even when we have been retrying.
			notification := un.constructNotification(db)
			err := un.upstream.SendJSON("UpstreamNotifier", "POST", url, &notification, nil)
			if err != nil {
				logger.WithError(err).Warning("unable to send, will retry later")
				continue
			}
			// Success! The timer will close the loop.
			timerClosable.closableCloseNotWait()
		}
		logger.Info("done sending notification to upstream Flamenco")
	}()
}

// SendStartupNotification sends a StartupNotification document to upstream Flamenco Server.
// Keeps trying in a goroutine until the notification was succesful.
func (un *UpstreamNotifier) SendStartupNotification() {
	timer := Timer("StartupNotifier", startupNotificationRetry,
		startupNotificationInitialDelay, &un.startupClosable)

	url, err := un.upstream.ResolveURL("/api/flamenco/managers/%s/startup", un.config.ManagerID)
	if err != nil {
		panic(fmt.Sprintf("SendStartupNotification: unable to construct URL: %s\n", err))
	}

	un.sendNotification(url, timer, &un.startupClosable)
}

// SendTaskTypesNotification sends a StartupNotification document to upstream Flamenco Server.
// Keeps trying in a goroutine until the notification was succesful.
func (un *UpstreamNotifier) SendTaskTypesNotification() {
	un.taskTypesMutex.Lock()
	defer un.taskTypesMutex.Unlock()

	// Check whether we're already trying to push a notification.
	if un.taskTypesClosable != nil {
		ttc := un.taskTypesClosable
		ttc.closableClosingLock()
		defer ttc.closableClosingUnlock()

		if !ttc.isClosed {
			log.Warning("SendTaskTypesNotification: a notification is already trying to be sent.")
			return
		}
	}

	url, err := un.upstream.ResolveURL("/api/flamenco/managers/%s/update", un.config.ManagerID)
	if err != nil {
		log.WithError(err).Error("SendTaskTypesNotification: unable to construct update URL")
		return
	}

	newClosable := makeClosable()
	un.taskTypesClosable = &newClosable

	timer := Timer("TaskTypesNotifier", taskTypesNotificationRetry, 0, un.taskTypesClosable)
	un.sendNotification(url, timer, un.taskTypesClosable)
}
