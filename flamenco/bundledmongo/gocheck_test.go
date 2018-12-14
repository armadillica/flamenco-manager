/**
 * Common test functionality, and integration with GoCheck.
 */
package bundledmongo

import (
	"testing"

	log "github.com/sirupsen/logrus"

	check "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
// You only need one of these per package, or tests will run multiple times.
func TestWithGocheck(t *testing.T) {
	log.SetLevel(log.WarnLevel)
	check.TestingT(t)
}
