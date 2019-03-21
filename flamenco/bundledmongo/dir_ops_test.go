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
