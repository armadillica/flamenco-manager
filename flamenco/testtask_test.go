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
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	check "gopkg.in/check.v1"
	mgo "gopkg.in/mgo.v2"
)

type TestTaskTestSuite struct {
	worker Worker

	config  Conf
	session *mgo.Session
	db      *mgo.Database
}

var _ = check.Suite(&TestTaskTestSuite{})

func (s *TestTaskTestSuite) SetUpSuite(c *check.C) {
	s.config = GetTestConfig()
	s.session = MongoSession(&s.config)
	s.db = s.session.DB("")
}

func (s *TestTaskTestSuite) SetUpTest(c *check.C) {
	s.worker = Worker{
		Platform:           "linux",
		SupportedTaskTypes: []string{"sleeping", "test-blender-render"},
		Nickname:           "worker",
		Status:             workerStatusAwake,
	}
	if err := StoreNewWorker(&s.worker, s.db); err != nil {
		c.Fatal("Unable to insert test workerLnx", err)
	}
}

func (s *TestTaskTestSuite) TearDownTest(c *check.C) {
	log.Info("TestTaskTestSuite tearing down test, dropping database.")
	s.db.DropDatabase()
}

func (s *TestTaskTestSuite) TestBadWorkerStatus(t *check.C) {
	msg, err := CreateTestTask(&s.worker, &s.config, s.db)
	assert.Equal(t, "", msg)
	assert.Contains(t, err.Error(), "test jobs only work in status 'testing'")
}

func (s *TestTaskTestSuite) TestHappy(t *check.C) {
	s.worker.Status = workerStatusTesting
	msg, err := CreateTestTask(&s.worker, &s.config, s.db)
	assert.Nil(t, err)
	assert.Contains(t, msg, "test-blender-render")

	// Just test that the config values are still what we expect.
	assert.Equal(t, "{job_storage}/test-jobs", s.config.TestTasks.BlenderRender.JobStorage)
	assert.Equal(t, "{render}/test-renders", s.config.TestTasks.BlenderRender.RenderOutput)

	replacedJobStorage := ReplaceLocal(s.config.TestTasks.BlenderRender.JobStorage, &s.config)
	assert.DirExists(t, replacedJobStorage)
	_, err = os.Stat(s.config.TestTasks.BlenderRender.JobStorage)
	assert.True(t, os.IsNotExist(err), "%s should not exist", s.config.TestTasks.BlenderRender.JobStorage)

	replacedRenderOutput := ReplaceLocal(s.config.TestTasks.BlenderRender.RenderOutput, &s.config)
	assert.DirExists(t, replacedRenderOutput)
	_, err = os.Stat(s.config.TestTasks.BlenderRender.RenderOutput)
	assert.True(t, os.IsNotExist(err), "%s should not exist", s.config.TestTasks.BlenderRender.RenderOutput)

}
