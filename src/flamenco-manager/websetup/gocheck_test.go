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
