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
