package flamenco

import (
	"time"

	log "github.com/sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
)

const (
	// Initial delay to allow workers to come back online after the Manager was down.
	workerRemoverInitialSleep  = 5 * time.Minute
	workerRemoverCheckInterval = 30 * time.Second
)

// WorkerRemover periodically removes offline workers.
type WorkerRemover struct {
	closable
	config  *Conf
	session *mgo.Session
}

// CreateWorkerRemover creates a WorkerRemover, or returns nil if the configuration disables automatic worker removal.
func CreateWorkerRemover(config *Conf, session *mgo.Session) *WorkerRemover {
	logger := log.WithField("worker_cleanup_max_age", config.WorkerCleanupMaxAge)
	if config.WorkerCleanupMaxAge == 0*time.Second {
		logger.Info("offline workers will not be removed")
		return nil
	}
	logger.Info("offline workers will be auto-removed")
	return &WorkerRemover{
		makeClosable(),
		config,
		session,
	}
}

// Close signals the WorkerRemover goroutine to stop and waits for it to close.
func (wr *WorkerRemover) Close() {
	log.Debug("WorkerRemover: Close() called.")
	wr.closableCloseAndWait()
	log.Debug("WorkerRemover: shutdown complete.")
}

// Go starts a goroutine that periodically checks workers.
func (wr *WorkerRemover) Go() {
	wr.closableAdd(1)
	go func() {
		session := wr.session.Copy()
		db := session.DB("")
		defer session.Close()
		defer wr.closableDone()
		defer log.Info("WorkerRemover: shutting down.")

		// Start with a delay, so that workers get a chance to push their updates
		// after the manager has started up.
		timer := Timer("WorkerRemover", workerRemoverCheckInterval, workerRemoverInitialSleep, &wr.closable)

		for range timer {
			wr.cleanupWorkers(db)
		}
	}()
}

func (wr *WorkerRemover) cleanupWorkers(db *mgo.Database) {
	log.Debug("WorkerRemover: cleaning up workers")

	// Any worker last seen before the threshold will be deleted.
	threshold := time.Now().UTC().Add(-wr.config.WorkerCleanupMaxAge)

	info, err := db.C("flamenco_workers").RemoveAll(M{
		"status":        workerStatusOffline,
		"last_activity": M{"$lt": threshold},
	})
	if err != nil {
		log.WithError(err).Warning("WorkerRemover: unable to remove workers")
	}
	if info.Removed > 0 {
		log.WithField("workers_removed", info.Removed).Info("WorkerRemover: removed offline workers")
	}
}
