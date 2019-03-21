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

// WorkersLeft returns the IDs of workers NOT blacklisted for this task type on this job.
func (wbl *WorkerBlacklist) WorkersLeft(jobID bson.ObjectId, taskType string) map[bson.ObjectId]bool {
	logger := log.WithFields(log.Fields{
		"job_id":    jobID.Hex(),
		"task_type": taskType,
	})
	coll := wbl.collection()

	// Construct list of blacklisted worker IDs.
	query := coll.Find(M{"job_id": jobID, "task_type": taskType}).Select(M{"worker_id": true})
	blacklisted := []bson.ObjectId{}
	found := WorkerBlacklistEntry{}
	iter := query.Iter()
	for iter.Next(&found) {
		blacklisted = append(blacklisted, found.WorkerID)
	}
	if err := iter.Close(); err != nil {
		logger.WithError(err).Error("WorkersLeft: unable to query for blacklisted workers")
	}

	// Count how many workers were not blacklisted
	workersColl := wbl.session.DB("").C("flamenco_workers")
	iter = workersColl.Find(M{
		"_id":                  M{"$nin": blacklisted},
		"supported_task_types": taskType,
	}).Select(M{"_id": true}).Iter()
	workerIDs := map[bson.ObjectId]bool{}
	worker := Worker{}
	for iter.Next(&worker) {
		workerIDs[worker.ID] = true
	}
	if err := iter.Close(); err != nil {
		logger.WithError(err).Error("WorkersLeft: unable to fetch non-blacklisted workers")
	}
	logger.WithFields(log.Fields{
		"workers_left":        len(workerIDs),
		"workers_blacklisted": len(blacklisted),
	}).Debug("WorkersLeft: counted non-blacklisted workers")

	return workerIDs
}

// RemoveLine removes a single blacklist entry.
// This is a no-op if the entry doesn't exist.
func (wbl *WorkerBlacklist) RemoveLine(workerID bson.ObjectId, jobID bson.ObjectId, taskType string) error {
	logger := log.WithFields(log.Fields{
		"worker_id": workerID.Hex(),
		"job_id":    jobID.Hex(),
		"task_type": taskType,
	})
	logger.Info("un-blacklisting worker")

	err := wbl.collection().Remove(M{
		"worker_id": workerID,
		"job_id":    jobID,
		"task_type": taskType,
	})
	if err != nil {
		logger.WithError(err).Warning("unable to un-blacklist worker")
		return err
	}

	return nil
}
