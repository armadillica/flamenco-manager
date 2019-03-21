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

package filestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStoragePrefix(t *testing.T) {
	bin := storageBin{
		basePath: "/base",
		dirName:  "testunit",
	}
	assert.Equal(t, "/base/testunit", bin.storagePrefix(""))
	assert.Equal(t, "/base/testunit", bin.storagePrefix("/"))
	assert.Equal(t, "/base/testunit/xxx", bin.storagePrefix("xxx"))
	assert.Equal(t, "/base/testunit/xxx", bin.storagePrefix("/xxx"))
}

func TestContains(t *testing.T) {
	bin := storageBin{
		basePath: "/base",
		dirName:  "testunit",
	}
	assert.True(t, bin.contains("", "/base/testunit/jemoeder.txt"))
	assert.True(t, bin.contains("jemoeder", "/base/testunit/jemoeder.txt"))
	assert.False(t, bin.contains("jemoeder", "/base/testunit/opjehoofd/jemoeder.txt"))
	assert.False(t, bin.contains("", "/etc/passwd"))
	assert.False(t, bin.contains("/", "/etc/passwd"))
	assert.False(t, bin.contains("/etc", "/etc/passwd"))
}
