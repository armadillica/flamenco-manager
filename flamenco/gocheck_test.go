/**
 * Common test functionality, and integration with GoCheck.
 */
package flamenco

/* ***** BEGIN MIT LICENSE BLOCK *****
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
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
 * ***** END MIT LICENCE BLOCK *****
 */

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"

	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

// Hook up gocheck into the "go test" runner.
// You only need one of these per package, or tests will run multiple times.
func TestWithGocheck(t *testing.T) {
	log.SetLevel(log.WarnLevel)
	check.TestingT(t)
}

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
		Status:   statusQueued,
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

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func fileTouch(filename string) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_RDONLY, 0666)
	if err != nil {
		panic(err.Error())
	}
	file.Close()
}

func testRequestWithBody(body io.Reader, method, url string, vargs ...interface{}) (*httptest.ResponseRecorder, *http.Request) {
	respRec := httptest.NewRecorder()
	if respRec == nil {
		panic("testRequestWithBody: respRec is nil")
	}

	request, err := http.NewRequest(method, fmt.Sprintf(url, vargs...), body)
	if err != nil {
		panic(err)
	}
	if request == nil {
		panic("testRequestWithBody: request is nil")
	}
	request.RemoteAddr = "[::1]:47327"
	return respRec, request
}

func testJSONRequest(payload interface{}, method, url string, vargs ...interface{}) (*httptest.ResponseRecorder, *http.Request) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	respRec, request := testRequestWithBody(bytes.NewBuffer(payloadBytes), method, url, vargs...)
	request.Header.Set("Content-Type", "application/json")

	return respRec, request
}
