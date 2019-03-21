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

package flamenco

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/stretchr/testify/assert"

	check "gopkg.in/check.v1"
)

type LogRotationTestSuite struct {
	temppath string
}

var _ = check.Suite(&LogRotationTestSuite{})

func (s *LogRotationTestSuite) SetUpTest(c *check.C) {
	temppath, err := ioutil.TempDir("", "testlogs")
	assert.Nil(c, err)
	s.temppath = temppath
}

func (s *LogRotationTestSuite) TearDownTest(c *check.C) {
	os.RemoveAll(s.temppath)
}

func (s *LogRotationTestSuite) TestCreateNumberedPath(c *check.C) {
	numtest := func(path string, number int, basepath string) {
		result := createNumberedPath(path)
		assert.Equal(c, numberedPath{path, number, basepath}, result)
	}

	numtest("", -1, "")
	numtest(" ", -1, " ")
	numtest("jemoeder.1", 1, "jemoeder")
	numtest("jemoeder.", -1, "jemoeder.")
	numtest("jemoeder", -1, "jemoeder")
	numtest("jemoeder.abc", -1, "jemoeder.abc")
	numtest("jemoeder.-4", -4, "jemoeder")
	numtest("jemoeder.1.2.3", 3, "jemoeder.1.2")
	numtest("jemoeder.001", 1, "jemoeder")
	numtest("jemoeder.01", 1, "jemoeder")
	numtest("jemoeder.010", 10, "jemoeder")
	numtest("jemoeder 47 42.327", 327, "jemoeder 47 42")
	numtest("/path/üničøde.327/.47", 47, "/path/üničøde.327/")
	numtest("üničøde.327.what?", -1, "üničøde.327.what?")
}

func (s *LogRotationTestSuite) TestNoFiles(c *check.C) {
	filepath := filepath.Join(s.temppath, "nonexisting.txt")
	err := rotateLogFile(filepath)
	assert.Nil(c, err)
	assert.False(c, fileExists(filepath))
}

func (s *LogRotationTestSuite) TestOneFile(c *check.C) {
	filepath := filepath.Join(s.temppath, "existing.txt")
	fileTouch(filepath)

	err := rotateLogFile(filepath)
	assert.Nil(c, err)
	assert.False(c, fileExists(filepath))
	assert.True(c, fileExists(filepath+".1"))
}

func (s *LogRotationTestSuite) TestMultipleFilesWithHoles(c *check.C) {
	filepath := filepath.Join(s.temppath, "existing.txt")
	assert.Nil(c, ioutil.WriteFile(filepath, []byte("thefile"), 0666))
	assert.Nil(c, ioutil.WriteFile(filepath+".1", []byte("file .1"), 0666))
	assert.Nil(c, ioutil.WriteFile(filepath+".2", []byte("file .2"), 0666))
	assert.Nil(c, ioutil.WriteFile(filepath+".3", []byte("file .3"), 0666))
	assert.Nil(c, ioutil.WriteFile(filepath+".5", []byte("file .5"), 0666))
	assert.Nil(c, ioutil.WriteFile(filepath+".7", []byte("file .7"), 0666))

	err := rotateLogFile(filepath)

	assert.Nil(c, err)
	assert.False(c, fileExists(filepath))
	assert.True(c, fileExists(filepath+".1"))
	assert.True(c, fileExists(filepath+".2"))
	assert.True(c, fileExists(filepath+".3"))
	assert.True(c, fileExists(filepath+".4"))
	assert.False(c, fileExists(filepath+".5"))
	assert.True(c, fileExists(filepath+".6"))
	assert.False(c, fileExists(filepath+".7"))
	assert.True(c, fileExists(filepath+".8"))
	assert.False(c, fileExists(filepath+".9"))

	read := func(filename string) string {
		content, err := ioutil.ReadFile(filename)
		assert.Nil(c, err)
		return string(content)
	}

	assert.Equal(c, "thefile", read(filepath+".1"))
	assert.Equal(c, "file .1", read(filepath+".2"))
	assert.Equal(c, "file .2", read(filepath+".3"))
	assert.Equal(c, "file .3", read(filepath+".4"))
	assert.Equal(c, "file .5", read(filepath+".6"))
	assert.Equal(c, "file .7", read(filepath+".8"))
}
