package checkout

/* ***** BEGIN GPL LICENSE BLOCK *****
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License
 * as published by the Free Software Foundation; either version 2
 * of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software Foundation,
 * Inc., 59 Temple Place - Suite 330, Boston, MA  02111-1307, USA.
 *
 * ***** END GPL LICENCE BLOCK *****
 *
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
 */

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
