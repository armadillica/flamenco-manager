package flamenco

import (
	"testing"

	"github.com/stretchr/testify/assert"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func TestGetOrCreateMapCreate(t *testing.T) {
	doc := bson.M{"other-field": "some-value"}

	subdoc := GetOrCreateMap(doc, "test")
	subdoc["agent"] = 47

	assert.EqualValues(t, bson.M{
		"other-field": "some-value",
		"test":        bson.M{"agent": 47},
	}, doc)
}

func TestGetOrCreateMapGet(t *testing.T) {
	doc := bson.M{
		"other-field": "some-value",
		"test":        bson.M{"old-value": 327},
	}

	subdoc := GetOrCreateMap(doc, "test")
	subdoc["agent"] = 47

	assert.EqualValues(t, bson.M{
		"other-field": "some-value",
		"test": bson.M{
			"agent":     47,
			"old-value": 327,
		},
	}, doc)
}

func TestGetOrCreateMapExtend(t *testing.T) {
	doc := bson.M{
		"other-field": "some-value",
		"test": bson.M{
			"agents": bson.M{
				"Hendrick IJzerbroot": 327,
			},
		},
	}

	subdoc := GetOrCreateMap(doc, "test")
	agents := GetOrCreateMap(subdoc, "agents")
	agents["47"] = 47

	assert.EqualValues(t, bson.M{
		"other-field": "some-value",
		"test": bson.M{
			"agents": bson.M{
				"Hendrick IJzerbroot": 327,
				"47":                  47,
			},
		},
	}, doc)
}

func assertTaskStatus(t assert.TestingT, db *mgo.Database, taskID bson.ObjectId, expectedStatus string) {
	task := Task{}
	err := db.C("flamenco_tasks").FindId(taskID).Select(M{"status": true}).One(&task)
	assert.Nil(t, err)
	assert.Equal(t, expectedStatus, task.Status)
}

func assertTaskStatusesQueued(t assert.TestingT, db *mgo.Database, taskID bson.ObjectId, expectedStatus ...string) {
	queueColl := db.C(queueMgoCollection)
	query := queueColl.Find(M{"task_id": taskID}).Select(M{"task_status": true})
	iter := query.Iter()
	foundStatuses := []string{}
	taskUpdate := TaskUpdate{}
	for iter.Next(&taskUpdate) {
		foundStatuses = append(foundStatuses, taskUpdate.TaskStatus)
	}
	assert.Nil(t, iter.Err())

	if expectedStatus == nil {
		expectedStatus = []string{}
	}
	assert.EqualValues(t, expectedStatus, foundStatuses)
}
