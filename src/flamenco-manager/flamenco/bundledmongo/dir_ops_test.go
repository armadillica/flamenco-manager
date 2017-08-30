package bundledmongo

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/assert"
	check "gopkg.in/check.v1"
)

// BundledMongoDirOpsTestSuite tests dir_ops.go
type BundledMongoDirOpsTestSuite struct {
}

var _ = check.Suite(&BundledMongoDirOpsTestSuite{})

// TestRelativePath tests relativeToExecutable()
func (s *BundledMongoDirOpsTestSuite) TestRelativePath(t *check.C) {
	relpath, err := relativeToExecutable("mongodb/bin/filename.ext")
	assert.NoError(t, err)

	// Expect the returned path to be OS-specific, rather than the POSIX path we passed.
	expectedSuffix := filepath.Join("mongodb", "bin", "filename.ext")
	assert.True(t, strings.HasSuffix(relpath, expectedSuffix),
		fmt.Sprintf("Expected path %v to have suffix %v", relpath, expectedSuffix))
}
