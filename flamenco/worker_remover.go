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
	config    *Conf
	session   *mgo.Session
	logFields log.Fields
}

// CreateWorkerRemover creates a WorkerRemover, or returns nil if the configuration disables automatic worker removal.
func CreateWorkerRemover(config *Conf, session *mgo.Session) *WorkerRemover {
	logFields := log.Fields{
		"worker_cleanup_max_age": config.WorkerCleanupMaxAge,
		"worker_cleanup_status":  config.WorkerCleanupStatus,
	}
	logger := log.WithFields(logFields)
	if config.WorkerCleanupMaxAge == 0*time.Second || len(config.WorkerCleanupStatus) == 0 {
		logger.Info("workers will not be auto-removed")
		return nil
	}
	logger.Info("workers will be auto-removed")
	return &WorkerRemover{
		makeClosable(),
		config,
		session,
		logFields,
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
	logger := log.WithFields(wr.logFields)
	logger.Debug("WorkerRemover: cleaning up workers")

	// Any worker last seen before the threshold will be deleted.
	threshold := time.Now().UTC().Add(-wr.config.WorkerCleanupMaxAge)

	info, err := db.C("flamenco_workers").RemoveAll(M{
		"status":        M{"$in": wr.config.WorkerCleanupStatus},
		"last_activity": M{"$lt": threshold},
	})
	if err != nil {
		logger.WithError(err).Warning("WorkerRemover: unable to remove workers")
	}
	if info.Removed > 0 {
		logger.WithField("workers_removed", info.Removed).Info("WorkerRemover: removed offline workers")
	}
}
