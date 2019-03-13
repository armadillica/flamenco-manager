package flamenco

/* ***** BEGIN MIT LICENSE BLOCK *****
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
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
 * ***** END MIT LICENCE BLOCK *****
 */

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"

	"gopkg.in/mgo.v2/bson"

	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"
	check "gopkg.in/check.v1"
	httpmock "gopkg.in/jarcoal/httpmock.v1"
	mgo "gopkg.in/mgo.v2"
)

type TaskLogUploaderTestSuite struct {
	config          Conf
	session         *mgo.Session
	upstream        *UpstreamConnection
	taskLogUploader *TaskLogUploader
}

var _ = check.Suite(&TaskLogUploaderTestSuite{})

func (s *TaskLogUploaderTestSuite) SetUpSuite(c *check.C) {
	s.config = GetTestConfig()
	s.session = MongoSession(&s.config)
}

func (s *TaskLogUploaderTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()

	taskLogsPath, err := ioutil.TempDir("", "testlogs")
	assert.Nil(c, err)
	s.config.TaskLogsPath = taskLogsPath

	s.upstream = ConnectUpstream(&s.config, s.session)
	s.taskLogUploader = CreateTaskLogUploader(&s.config, s.upstream)
}

func (s *TaskLogUploaderTestSuite) TearDownTest(c *check.C) {
	log.Info("TaskLogUploaderTestSuite tearing down test, dropping database.")
	os.RemoveAll(s.config.TaskLogsPath)

	s.upstream.Close()
	s.session.DB("").DropDatabase()
	httpmock.DeactivateAndReset()
}

func (s *TaskLogUploaderTestSuite) TestCompressFile(t *check.C) {
	payload := []byte("this is some log file\n")
	fname := path.Join(s.config.TaskLogsPath, "testfile.txt")
	err := ioutil.WriteFile(fname, payload, 0666)
	assert.Nil(t, err)

	gzPath, err := s.taskLogUploader.compressFile(fname, log.WithField("filepath", fname))
	assert.Nil(t, err)
	assert.Equal(t, fname+".gz", gzPath)

	gzFile, err := os.Open(gzPath)
	assert.Nil(t, err)

	gzReader, err := gzip.NewReader(gzFile)
	assert.Nil(t, err)

	decomp, err := ioutil.ReadAll(gzReader)
	assert.Nil(t, err)

	assert.Equal(t, payload, decomp)
}

func (s *TaskLogUploaderTestSuite) TestNoRecompressFile(t *check.C) {
	payload := []byte("this is some log file\n")
	fname := path.Join(s.config.TaskLogsPath, "testfile.txt")
	err := ioutil.WriteFile(fname, payload, 0666)
	assert.Nil(t, err)

	// Turn back the date on the file we just created, to ensure we are within filesystem timestamp precision.
	mtime := time.Now().Add(-1 * time.Hour)
	os.Chtimes(fname, mtime, mtime)

	// Just write some nonsense. It should be kept (and the original file not recompressed).
	gzPayload := []byte("I can't believe this is not GZip\n")
	err = ioutil.WriteFile(fname+".gz", gzPayload, 0666)
	assert.Nil(t, err)

	gzPath, err := s.taskLogUploader.compressFile(fname, log.WithField("filepath", fname))
	assert.Nil(t, err)
	assert.Equal(t, fname+".gz", gzPath)

	decomp, err := ioutil.ReadFile(gzPath)
	assert.Nil(t, err)

	assert.Equal(t, gzPayload, decomp)
}

func (s *TaskLogUploaderTestSuite) TestCompressNoOriginalFile(t *check.C) {
	fname := path.Join(s.config.TaskLogsPath, "testfile.txt")

	// Just write some nonsense. It should be kept as the original file never existed.
	gzPayload := []byte("I can't believe this is not GZip\n")
	err := ioutil.WriteFile(fname+".gz", gzPayload, 0666)
	assert.Nil(t, err)

	gzPath, err := s.taskLogUploader.compressFile(fname, log.WithField("filepath", fname))
	assert.Nil(t, err)
	assert.Equal(t, fname+".gz", gzPath)

	decomp, err := ioutil.ReadFile(gzPath)
	assert.Nil(t, err)

	assert.Equal(t, gzPayload, decomp)
}

func (s *TaskLogUploaderTestSuite) TestCompressNoFile(t *check.C) {
	fname := path.Join(s.config.TaskLogsPath, "testfile.txt")

	gzPath, err := s.taskLogUploader.compressFile(fname, log.WithField("filepath", fname))
	assert.Nil(t, err)
	assert.Equal(t, fname+".gz", gzPath)

	gzBuffer := bytes.Buffer{}
	gzWriter, err := gzip.NewWriterLevel(&gzBuffer, 9)
	assert.Nil(t, err)

	_, err = io.Copy(gzWriter, bytes.NewBuffer([]byte("log file does not exist on Flamenco Manager")))
	assert.Nil(t, err)
	gzWriter.Close()

	gzFileContents, err := ioutil.ReadFile(gzPath)
	assert.Nil(t, err)

	assert.Equal(t, gzBuffer.Bytes(), gzFileContents)
}

func (s *TaskLogUploaderTestSuite) TestUploadFile(t *check.C) {
	payload := []byte("this is some log file\n")
	fname := path.Join(s.config.TaskLogsPath, "testfile.txt")
	err := ioutil.WriteFile(fname, payload, 0666)
	assert.Nil(t, err)

	requestMade := false
	httpmock.RegisterResponder(
		"POST", "http://localhost:51234/the-url",
		func(req *http.Request) (*http.Response, error) {
			req.ParseMultipartForm(1024 * 1024)
			filepart := req.MultipartForm.File["logfile"][0]
			assert.Equal(t, "text/plain+gzip", filepart.Header.Get("Content-Type"))

			file, err := filepart.Open()
			assert.Nil(t, err)
			defer file.Close()

			body, err := ioutil.ReadAll(file)
			assert.Nil(t, err)

			// The contents won't be compressed, because uploadFile() expects
			// to get an already-compressed file.
			assert.Equal(t, payload, body)

			requestMade = true
			return httpmock.NewBytesResponse(204, nil), nil
		},
	)

	s.taskLogUploader.uploadFile(bson.NewObjectId(), fname, "http://localhost:51234/the-url", log.WithField("unit", "test"))
	assert.True(t, requestMade)
}
