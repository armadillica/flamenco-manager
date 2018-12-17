package flamenco

import (
	"time"

	log "github.com/sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// WorkerBlacklist stores (worker ID, job ID, task type) tuples for failed tasks.
type WorkerBlacklist struct {
	config  *Conf
	session *mgo.Session
}

// CreateWorkerBlackList creates a new WorkerBlackList instance.
func CreateWorkerBlackList(config *Conf, session *mgo.Session) *WorkerBlacklist {
	return &WorkerBlacklist{
		config,
		session,
	}
}

func (wbl *WorkerBlacklist) collection() *mgo.Collection {
	return wbl.session.DB("").C("worker_blacklist")
}

// EnsureDBIndices ensures the MongoDB indices are there.
func (wbl *WorkerBlacklist) EnsureDBIndices() {
	coll := wbl.collection()

	coll.EnsureIndex(mgo.Index{
		Name: "worker-id",
		Key:  []string{"worker_id"},
	})
	coll.EnsureIndex(mgo.Index{
		Name:        "cleanup",
		Key:         []string{"_created"},
		ExpireAfter: 7 * 24 * time.Hour,
	})
	coll.EnsureIndex(mgo.Index{
		Name:     "unique",
		Key:      []string{"worker_id", "job_id", "task_type"},
		Unique:   true,
		DropDups: true,
	})
}

// Add makes it impossible for the worker to run tasks of the same type on the same job.
func (wbl *WorkerBlacklist) Add(workerID bson.ObjectId, task *Task) error {
	coll := wbl.collection()
	entry := WorkerBlacklistEntry{
		WorkerID: workerID,
		JobID:    task.Job,
		TaskType: task.TaskType,
		Created:  time.Now(),
	}

	logger := log.WithFields(log.Fields{
		"worker":    entry.WorkerID.Hex(),
		"job":       entry.JobID.Hex(),
		"task_type": entry.TaskType,
	})
	if err := coll.Insert(&entry); err != nil {
		if mgo.IsDup(err) {
			logger.Warning("ignoring duplicate blacklist request")
			return nil
		}
		logger.WithError(err).Error("unable to save black list entry")
		return err
	}
	logger.Info("saved worker blacklist entry")
	return nil
}

// BlacklistForWorker returns a partial MongoDB query that can be used to filter out blacklisted tasks.
func (wbl *WorkerBlacklist) BlacklistForWorker(workerID bson.ObjectId) M {
	coll := wbl.collection()
	pipe := coll.Pipe([]M{
		M{"$match": M{
			"worker_id": workerID,
		}},
		M{"$group": M{
			"_id":        "$job_id",
			"task_types": M{"$addToSet": "$task_type"},
		}},
	})
	iter := pipe.Iter()

	aggrResult := struct {
		JobID     bson.ObjectId `bson:"_id"`
		TaskTypes []string      `bson:"task_types"`
	}{}

	blacklist := make([]M, 0)
	for iter.Next(&aggrResult) {
		blacklist = append(blacklist, M{
			"job":       aggrResult.JobID,
			"task_type": M{"$in": aggrResult.TaskTypes},
		})
	}
	if err := iter.Close(); err != nil {
		log.WithError(err).Error("BlacklistForWorker: error querying MongoDB, blacklist could be partial")
	}
	if len(blacklist) == 0 {
		return M{}
	}
	// The result will be something like this:
	// M{
	// 	"$nor": []M{
	// 		M{
	// 			"job":       bson.ObjectId("5c0165dc494ba92dc0552a00"),
	// 			"task_type": M{"$in": []string{"blender-render"}},
	// 		},
	// 		M{
	// 			"job":       bson.ObjectId("5c0a4651494ba97321ba006b"),
	// 			"task_type": M{"$in": []string{"blender-render", "file-management"}},
	// 		},
	// 	},
	// }
	return M{"$nor": blacklist}
}
