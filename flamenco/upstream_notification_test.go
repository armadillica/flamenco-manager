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
	"net/http"
	"time"

	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"
	check "gopkg.in/check.v1"
	httpmock "gopkg.in/jarcoal/httpmock.v1"
	mgo "gopkg.in/mgo.v2"
)

type UpstreamNotificationTestSuite struct {
	workerLnx Worker
	workerWin Worker

	config   Conf
	session  *mgo.Session
	db       *mgo.Database
	upstream *UpstreamConnection
}

var _ = check.Suite(&UpstreamNotificationTestSuite{})

func (s *UpstreamNotificationTestSuite) SetUpSuite(c *check.C) {
	s.config = GetTestConfig()
	s.session = MongoSession(&s.config)
	s.db = s.session.DB("")
}

func (s *UpstreamNotificationTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()

	s.upstream = ConnectUpstream(&s.config, s.session)

	// Store workers in DB, on purpose in the opposite order as the tasks.
	s.workerLnx = Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"sleeping"},
		Nickname:           "workerLnx",
		Status:             workerStatusAwake,
	}
	if err := StoreNewWorker(&s.workerLnx, s.db); err != nil {
		c.Fatal("Unable to insert test workerLnx", err)
	}
	s.workerWin = Worker{
		Platform:           "windows",
		SupportedTaskTypes: []string{"testing"},
		Nickname:           "workerWin",
	}
	if err := StoreNewWorker(&s.workerWin, s.db); err != nil {
		c.Fatal("Unable to insert test workerWin", err)
	}
}

func (s *UpstreamNotificationTestSuite) TearDownTest(c *check.C) {
	log.Info("UpstreamNotificationTestSuite tearing down test, dropping database.")
	s.upstream.Close()
	s.db.DropDatabase()
	httpmock.DeactivateAndReset()
}

func (s *UpstreamNotificationTestSuite) TestStartupNotification(t *check.C) {
	callMade := make(chan bool, 1)
	httpmock.RegisterResponder(
		"POST",
		"http://localhost:51234/api/flamenco/managers/5852bc5198377351f95d103e/startup",
		func(req *http.Request) (*http.Response, error) {
			// TODO: test contents of request
			log.Info("HTTP POST to Flamenco was performed.")
			defer func() { callMade <- true }()
			return httpmock.NewStringResponse(204, ""), nil
		},
	)

	notifier := CreateUpstreamNotifier(&s.config, s.upstream, s.session)
	notifier.SendStartupNotification()
	defer notifier.Close()

	select {
	case <-callMade:
		break
	case <-time.After(startupNotificationInitialDelay + 250*time.Millisecond):
		assert.Fail(t, "Timeout waiting for startup notification")
	}

	assert.Equal(t, 1, httpmock.GetCallCountInfo()["POST http://localhost:51234/api/flamenco/managers/5852bc5198377351f95d103e/startup"],
		"Expected HTTP call to Flamenco Server not made")
}

func (s *UpstreamNotificationTestSuite) TestTaskTypesNotification(t *check.C) {
	callMade := make(chan bool, 1)
	httpmock.RegisterResponder(
		"POST",
		"http://localhost:51234/api/flamenco/managers/5852bc5198377351f95d103e/update",
		func(req *http.Request) (*http.Response, error) {
			log.Info("HTTP POST to Flamenco was performed.")
			defer func() { callMade <- true }()

			payload := UpstreamNotification{}
			assert.Nil(t, DecodeJSON(nil, req.Body, &payload, "unittest"))
			assert.Equal(t, 2, payload.NumberOfWorkers)

			log.WithField("task_types", payload.WorkerTaskTypes).Info("received task types")
			assert.ElementsMatch(t, []string{"testing", "sleeping"}, payload.WorkerTaskTypes)

			return httpmock.NewStringResponse(204, ""), nil
		},
	)

	notifier := CreateUpstreamNotifier(&s.config, s.upstream, s.session)
	notifier.SendTaskTypesNotification()
	defer notifier.Close()

	select {
	case <-callMade:
		break
	case <-time.After(250 * time.Millisecond):
		assert.Fail(t, "Timeout waiting for notification")
	}
}
