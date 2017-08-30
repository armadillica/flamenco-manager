package websetup

import (
	"flamenco-manager/flamenco"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	check "gopkg.in/check.v1"
	httpmock "gopkg.in/jarcoal/httpmock.v1"
)

// ServerLinkerTestSuite tests link.go
type ServerLinkerTestSuite struct {
}

var _ = check.Suite(&ServerLinkerTestSuite{})

func (s *ServerLinkerTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()
}

func (s *ServerLinkerTestSuite) TearDownTest(c *check.C) {
	httpmock.DeactivateAndReset()
}

func (s *ServerLinkerTestSuite) TestStartLinking(t *check.C) {
	linker, err := StartLinking("http://cloud.localhost:5000/")

	assert.Nil(t, err)
	assert.False(t, linker.HasIdentifier())
	assert.False(t, linker.HasManagerID())
	assert.Equal(t, 32, len(linker.key))
}

func (s *ServerLinkerTestSuite) TestExchangeKeyHappy(t *check.C) {
	linker, err := StartLinking("http://cloud.localhost:5000/")
	assert.Nil(t, err)

	timeout := flamenco.TimeoutAfter(2 * time.Second)
	defer close(timeout)

	// Mock that the server receives the request and sends an identifier back.
	var receivedKey string
	httpmock.RegisterResponder(
		"POST",
		"http://cloud.localhost:5000/api/flamenco/managers/link/exchange",
		func(req *http.Request) (*http.Response, error) {
			defer func() { timeout <- false }()
			log.Info("POST from manager received on server, sending back response.")

			// Check the key
			jsonRequest := keyExchangeRequest{}
			parseRequestJSON(t, req, &jsonRequest)
			receivedKey = jsonRequest.KeyHex

			resp := keyExchangeResponse{"123magic"}
			return httpmock.NewJsonResponse(200, &resp)
		},
	)

	err = linker.ExchangeKey()

	timedout := <-timeout
	assert.False(t, timedout, "HTTP request to Flamenco Server not performed")

	assert.Nil(t, err)

	assert.Equal(t, receivedKey, linker.keyHex())
	assert.Equal(t, "123magic", linker.identifier)
	assert.True(t, linker.HasIdentifier())
	assert.False(t, linker.HasManagerID())
}

func (s *ServerLinkerTestSuite) TestExchangeKeyNon200Response(t *check.C) {
	linker, err := StartLinking("http://cloud.localhost:5000/")
	assert.Nil(t, err)

	timeout := flamenco.TimeoutAfter(2 * time.Second)
	defer close(timeout)

	// Mock that the server receives the request and sends an identifier back.
	var receivedKey string
	httpmock.RegisterResponder(
		"POST",
		"http://cloud.localhost:5000/api/flamenco/managers/link/exchange",
		func(req *http.Request) (*http.Response, error) {
			defer func() { timeout <- false }()
			log.Info("POST from manager received on server, sending back response.")

			// Check the key
			jsonRequest := keyExchangeRequest{}
			parseRequestJSON(t, req, &jsonRequest)
			receivedKey = jsonRequest.KeyHex

			resp := keyExchangeResponse{"123magic"}
			return httpmock.NewJsonResponse(404, &resp)
		},
	)

	err = linker.ExchangeKey()

	timedout := <-timeout
	assert.False(t, timedout, "HTTP request to Flamenco Server not performed")

	assert.NotNil(t, err)

	assert.Equal(t, receivedKey, linker.keyHex())
	assert.Equal(t, "", linker.identifier)
	assert.False(t, linker.HasIdentifier())
	assert.False(t, linker.HasManagerID())
}
