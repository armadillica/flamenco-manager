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

package checkout

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefinitionReader(t *testing.T) {
	file, err := os.Open("definition_test_example.txt")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	ctx, cancelFunc := context.WithCancel(context.Background())
	reader := NewDefinitionReader(ctx, file)
	readChan := reader.Read()

	line := <-readChan
	assert.Equal(t, "35b0491c27b0333d1fb45fc0789a12ca06b1d640d2569780b807de504d7029e0", line.Checksum)
	assert.Equal(t, int64(1424), line.FileSize)
	assert.Equal(t, "definition.go", line.FilePath)

	line = <-readChan
	assert.Equal(t, "63b72c63b9424fd13b9370fb60069080c3a15717cf3ad442635b187c6a895079", line.Checksum)
	assert.Equal(t, int64(127), line.FileSize)
	assert.Equal(t, "logging.go", line.FilePath)
	assert.Nil(t, reader.Err)

	// Cancelling is only found out after the next read.
	cancelFunc()
	line = <-readChan
	assert.Nil(t, line)
	assert.Equal(t, context.Canceled, reader.Err)
	assert.Equal(t, 2, reader.LineNumber)
}

func TestDefinitionReaderBadRequests(t *testing.T) {
	ctx := context.Background()

	testRejects := func(checksum, path string) {
		buffer := bytes.NewReader([]byte(checksum + " 30 " + path))
		reader := NewDefinitionReader(ctx, buffer)
		readChan := reader.Read()

		var line *DefinitionLine
		line = <-readChan
		assert.Nil(t, line)
		assert.NotNil(t, reader.Err)
		assert.Equal(t, 1, reader.LineNumber)
	}

	testRejects("35b0491c27b0333d1fb45fc0789a12c", "/etc/passwd")                  // absolute
	testRejects("35b0491c27b0333d1fb45fc0789a12c", "../../../../../../etc/passwd") // ../ in there that path.Clean() will keep
	testRejects("35b0491c27b0333d1fb45fc0789a12c", "some/path/../etc/passwd")      // ../ in there that path.Clean() will remove

	testRejects("35b", "some/path")                             // checksum way too short
	testRejects("35b0491c.7b0333d1fb45fc0789a12c", "some/path") // checksum invalid
	testRejects("35b0491c/7b0333d1fb45fc0789a12c", "some/path") // checksum invalid
}
