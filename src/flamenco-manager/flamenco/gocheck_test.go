/**
 * Common test functionality, and integration with GoCheck.
 */
package flamenco

import (
	"testing"

	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

// Hook up gocheck into the "go test" runner.
// You only need one of these per package, or tests will run multiple times.
func TestWithGocheck(t *testing.T) { check.TestingT(t) }

func ConstructTestTask(taskID, taskType string) Task {
	return ConstructTestTaskWithPrio(taskID, taskType, 50)
}

func ConstructTestTaskWithPrio(taskID, taskType string, priority int) Task {
	return Task{
		ID:       bson.ObjectIdHex(taskID),
		Etag:     "1234567",
		Job:      bson.ObjectIdHex("bbbbbbbbbbbbbbbbbbbbbbbb"),
		Manager:  bson.ObjectIdHex("cccccccccccccccccccccccc"),
		Project:  bson.ObjectIdHex("dddddddddddddddddddddddd"),
		User:     bson.ObjectIdHex("eeeeeeeeeeeeeeeeeeeeeeee"),
		Name:     "Test task",
		Status:   "queued",
		Priority: priority,
		JobType:  "unittest",
		TaskType: taskType,
		Commands: []Command{
			Command{"echo", bson.M{"message": "Running Blender from {blender}"}},
			Command{"sleep", bson.M{"time_in_seconds": 3}},
		},
		Parents: []bson.ObjectId{
			bson.ObjectIdHex("ffffffffffffffffffffffff"),
		},
		Worker: "worker1",
	}
}
