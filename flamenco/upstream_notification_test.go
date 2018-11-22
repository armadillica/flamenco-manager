package flamenco

import (
	"net/http"
	"time"

	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"
	check "gopkg.in/check.v1"
	httpmock "gopkg.in/jarcoal/httpmock.v1"
)

type UpstreamNotificationTestSuite struct{}

var _ = check.Suite(&UpstreamNotificationTestSuite{})

func (s *UpstreamNotificationTestSuite) TestStartupNotification(t *check.C) {
	config := GetTestConfig()
	session := MongoSession(&config)

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	timeout := make(chan bool, 1)
	go func() {
		time.Sleep(startupNotificationInitialDelay + 250*time.Millisecond)
		timeout <- true
	}()

	httpmock.RegisterResponder(
		"POST",
		"http://localhost:51234/api/flamenco/managers/5852bc5198377351f95d103e/startup",
		func(req *http.Request) (*http.Response, error) {
			// TODO: test contents of request
			log.Info("HTTP POST to Flamenco was performed.")
			defer func() { timeout <- false }()
			return httpmock.NewStringResponse(204, ""), nil
		},
	)

	upstream := ConnectUpstream(&config, session)
	defer upstream.Close()

	notifier := CreateUpstreamNotifier(&config, upstream, session)
	notifier.SendStartupNotification()
	defer notifier.Close()

	timedout := <-timeout
	assert.False(t, timedout, "HTTP POST to Flamenco not performed")
}
