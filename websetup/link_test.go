package websetup

import (
	"crypto/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/armadillica/flamenco-manager/flamenco"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	check "gopkg.in/check.v1"
	httpmock "gopkg.in/jarcoal/httpmock.v1"
	"gopkg.in/mgo.v2/bson"
)

// ServerLinkerTestSuite tests link.go
type ServerLinkerTestSuite struct {
	config   *flamenco.Conf
	localURL *url.URL
}

var _ = check.Suite(&ServerLinkerTestSuite{})

func (s *ServerLinkerTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()

	serverURL, err := url.Parse("http://cloud.localhost:5000/")
	assert.Nil(c, err)

	s.localURL, err = url.Parse("http://flamanager:8083/")
	assert.Nil(c, err)

	s.config = &flamenco.Conf{
		Flamenco:      serverURL,
		ManagerID:     "123",
		ManagerSecret: "jemoeder",
	}
}

func (s *ServerLinkerTestSuite) TearDownTest(c *check.C) {
	httpmock.DeactivateAndReset()
}

func (s *ServerLinkerTestSuite) TestStartLinking(t *check.C) {
	linker, err := StartLinking("http://cloud.localhost:5000/", nil)

	assert.Nil(t, err)
	assert.False(t, linker.HasIdentifier())
	assert.False(t, linker.HasManagerID())
	assert.Equal(t, 32, len(linker.key))
}

func (s *ServerLinkerTestSuite) TestExchangeKeyHappy(t *check.C) {
	linker, err := StartLinking("http://cloud.localhost:5000/", nil)
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
	linker, err := StartLinking("http://cloud.localhost:5000/", nil)
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

func (s *ServerLinkerTestSuite) TestLinkRequiredNon200Response(t *check.C) {
	serverURL, err := url.Parse("http://cloud.localhost:5000/")
	assert.Nil(t, err)

	config := flamenco.Conf{
		Flamenco:      serverURL,
		ManagerID:     "123",
		ManagerSecret: "jemoeder",
	}

	timeout := flamenco.TimeoutAfter(1 * time.Second)
	defer close(timeout)

	// Mock that the server receives the request and sends an identifier back.
	httpmock.RegisterResponder(
		"GET", "http://cloud.localhost:5000/api/flamenco/managers/123",
		NewJSONResponder(403, bson.M{"_error": "access denied"}, timeout),
	)

	required := LinkRequired(&config)

	timedout := <-timeout
	assert.False(t, timedout, "HTTP request to Flamenco Server not performed")

	assert.True(t, required)
}

func (s *ServerLinkerTestSuite) TestLinkRequired200Response(t *check.C) {
	timeout := flamenco.TimeoutAfter(1 * time.Second)
	defer close(timeout)

	// Mock that the server receives the request and sends an identifier back.
	httpmock.RegisterResponder(
		"GET", "http://cloud.localhost:5000/api/flamenco/managers/123",
		NewJSONResponder(200, bson.M{"yes": "this is you"}, timeout),
	)

	required := LinkRequired(s.config)

	timedout := <-timeout
	assert.False(t, timedout, "HTTP request to Flamenco Server not performed")

	assert.False(t, required)
}

func (s *ServerLinkerTestSuite) TestLinkRequiredMissingData(t *check.C) {
	serverURL, err := url.Parse("http://cloud.localhost:5000/")
	assert.Nil(t, err)

	// Mock that the server receives the request and sends an identifier back.
	// This should *not* be called, but if there is a mistake then it may be,
	// and in that case I want LinkRequired() to return false so we detect this.
	resp, err := httpmock.NewJsonResponder(200, bson.M{"yes": "this is you"})
	assert.Nil(t, err)
	httpmock.RegisterResponder(
		"GET", "http://cloud.localhost:5000/api/flamenco/managers/123",
		resp,
	)

	// No server URL
	config := flamenco.Conf{}
	assert.True(t, LinkRequired(&config))

	// No Manager ID
	config.Flamenco = serverURL
	assert.True(t, LinkRequired(&config))

	// No auth token
	config.ManagerID = "123"
	assert.True(t, LinkRequired(&config))
}

func (s *ServerLinkerTestSuite) TestRedirectURL(t *check.C) {
	linker := ServerLinker{
		upstream:   s.config.Flamenco,
		identifier: "hahaha ident",
		key:        make([]byte, 32),
		localURL:   s.localURL,
	}
	_, err := rand.Read(linker.key)
	assert.Nil(t, err, "Unable to generate secret key")

	url, err := linker.redirectURL()
	assert.Nil(t, err)

	q := url.Query()
	assert.Equal(t, "hahaha ident", q.Get("identifier"))
	assert.Equal(t, "http://flamanager:8083/setup/link-return", q.Get("return"))
	assert.True(t, len(q.Get("hmac")) > 10)
}

func (s *ServerLinkerTestSuite) TestResetToken(t *check.C) {
	linker := ServerLinker{
		upstream:   s.config.Flamenco,
		identifier: "hahaha ident",
		managerID:  "aabbccddeeff",
		key:        make([]byte, 32),
		localURL:   s.localURL,
	}
	_, err := rand.Read(linker.key)
	assert.Nil(t, err, "Unable to generate secret key")

	timeout := flamenco.TimeoutAfter(1 * time.Second)
	defer close(timeout)

	var receivedIdentifier, receivedManagerID string

	httpmock.RegisterResponder(
		"POST", "http://cloud.localhost:5000/api/flamenco/managers/link/reset-token",
		func(req *http.Request) (*http.Response, error) {
			defer func() { timeout <- false }()
			log.Info("POST from manager received on server, sending back response.")

			// Check the payload
			jsonRequest := authTokenResetRequest{}
			parseRequestJSON(t, req, &jsonRequest)
			receivedIdentifier = jsonRequest.Identifier
			receivedManagerID = jsonRequest.ManagerID

			resp := authTokenResetResponse{"new-token", "unparsed datetime"}
			return httpmock.NewJsonResponse(200, &resp)
		},
	)

	token, err := linker.resetAuthToken()
	timedout := <-timeout
	assert.False(t, timedout, "HTTP request to Flamenco Server not performed")

	assert.Nil(t, err)
	assert.Equal(t, "new-token", token)
	assert.Equal(t, "hahaha ident", receivedIdentifier)
	assert.Equal(t, "aabbccddeeff", receivedManagerID)
}

func (s *ServerLinkerTestSuite) TestResetTokenUnhappy(t *check.C) {
	linker := ServerLinker{
		upstream:   s.config.Flamenco,
		identifier: "hahaha ident",
		managerID:  "aabbccddeeff",
		key:        make([]byte, 32),
		localURL:   s.localURL,
	}
	_, err := rand.Read(linker.key)
	assert.Nil(t, err, "Unable to generate secret key")

	timeout := flamenco.TimeoutAfter(1 * time.Second)
	defer close(timeout)

	httpmock.RegisterResponder(
		"POST", "http://cloud.localhost:5000/api/flamenco/managers/link/reset-token",
		NewJSONResponder(400, bson.M{"_error": "invalid MAC"}, timeout),
	)

	token, err := linker.resetAuthToken()
	timedout := <-timeout
	assert.False(t, timedout, "HTTP request to Flamenco Server not performed")

	assert.NotNil(t, err)
	assert.Equal(t, "", token)
}
