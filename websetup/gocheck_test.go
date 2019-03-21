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

/**
 * Common test functionality, and integration with GoCheck.
 */
package websetup

import (
	"encoding/json"
	"net/http"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	check "gopkg.in/check.v1"
	httpmock "gopkg.in/jarcoal/httpmock.v1"
)

// Hook up gocheck into the "go test" runner.
// You only need one of these per package, or tests will run multiple times.
func TestWithGocheck(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	check.TestingT(t)
}

func parseRequestJSON(c *check.C, req *http.Request, parsed interface{}) {
	assert.Equal(c, "application/json", req.Header.Get("Content-Type"))

	if req == nil {
		panic("req is nil")
	}
	if parsed == nil {
		panic("parsed is nil")
	}
	if &parsed == nil {
		panic("&parsed is nil")
	}

	decoder := json.NewDecoder(req.Body)
	if decoder == nil {
		panic("decoder is nil")
	}
	if err := decoder.Decode(parsed); err != nil {
		c.Fatalf("Unable to decode JSON: %s", err)
	}
}

// NewJSONResponder results in a JSON response and sends false to the timeout channel.
func NewJSONResponder(status int, body interface{}, timeout chan bool) httpmock.Responder {
	responder := func(req *http.Request) (*http.Response, error) {
		defer func() { timeout <- false }()
		log.Infof("%s from manager received on server, sending back response.", req.Method)
		return httpmock.NewJsonResponse(status, body)
	}
	return responder
}
