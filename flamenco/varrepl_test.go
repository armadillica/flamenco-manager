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
	"runtime"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	check "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
)

type VarReplTestSuite struct {
	config Conf
	task   Task
}

var _ = check.Suite(&VarReplTestSuite{})

func (s *VarReplTestSuite) SetUpTest(c *check.C) {
	log.SetLevel(log.InfoLevel)

	s.config = GetTestConfig()
	s.task = Task{
		Commands: []Command{
			Command{"echo", bson.M{"message": "Running Blender from {blender} {blender}"}},
			Command{"sleep", bson.M{"{blender}": 3}},
			Command{
				"blender_render",
				bson.M{
					"filepath":      "{job_storage}/_flamenco/storage/sybren/2017-06-08-181223.625800-sybren-flamenco-test.flamenco/flamenco-test.flamenco.blend",
					"otherpath":     "{hey}/haha",
					"format":        "EXR",
					"frames":        "47",
					"cycles_chunk":  1.0,
					"blender_cmd":   "{blender}",
					"render_output": "{render_long}/_flamenco/output/sybren/blender-cloud-addon/flamenco-test__intermediate/render-smpl-0001-0084-frm-######",
				},
			},
		},
	}
}

func (s *VarReplTestSuite) TestReplaceVariables(t *check.C) {
	worker := Worker{Platform: "linux"}
	ReplaceVariables(&s.config, &s.task, &worker)

	assert.Equal(t,
		"/opt/myblenderbuild/blender",
		s.task.Commands[2].Settings["blender_cmd"],
	)

	// Substitution should happen as often as needed.
	assert.Equal(t,
		"Running Blender from /opt/myblenderbuild/blender /opt/myblenderbuild/blender",
		s.task.Commands[0].Settings["message"],
	)

	// No substitution should happen on keys, just on values.
	assert.Equal(t, 3, s.task.Commands[1].Settings["{blender}"])
}

func (s *VarReplTestSuite) TestReplacePathsLinux(t *check.C) {
	worker := Worker{Platform: "linux"}
	ReplaceVariables(&s.config, &s.task, &worker)

	assert.Equal(t,
		"/shared/_flamenco/storage/sybren/2017-06-08-181223.625800-sybren-flamenco-test.flamenco/flamenco-test.flamenco.blend",
		s.task.Commands[2].Settings["filepath"],
	)
	assert.Equal(t,
		"/render/long/_flamenco/output/sybren/blender-cloud-addon/flamenco-test__intermediate/render-smpl-0001-0084-frm-######",
		s.task.Commands[2].Settings["render_output"],
	)
	assert.Equal(t, "{hey}/haha", s.task.Commands[2].Settings["otherpath"])
}

func (s *VarReplTestSuite) TestReplacePathsWindows(t *check.C) {
	worker := Worker{Platform: "windows"}
	ReplaceVariables(&s.config, &s.task, &worker)

	assert.Equal(t,
		"s:/_flamenco/storage/sybren/2017-06-08-181223.625800-sybren-flamenco-test.flamenco/flamenco-test.flamenco.blend",
		s.task.Commands[2].Settings["filepath"],
	)
	assert.Equal(t,
		"r:/long/_flamenco/output/sybren/blender-cloud-addon/flamenco-test__intermediate/render-smpl-0001-0084-frm-######",
		s.task.Commands[2].Settings["render_output"],
	)
	assert.Equal(t, "{hey}/haha", s.task.Commands[2].Settings["otherpath"])
}

func (s *VarReplTestSuite) TestReplacePathsMacOS(t *check.C) {
	worker := Worker{Platform: "darwin"}
	ReplaceVariables(&s.config, &s.task, &worker)

	assert.Equal(t,
		"/Volume/shared/_flamenco/storage/sybren/2017-06-08-181223.625800-sybren-flamenco-test.flamenco/flamenco-test.flamenco.blend",
		s.task.Commands[2].Settings["filepath"],
	)
	assert.Equal(t,
		"/Volume/render/long/_flamenco/output/sybren/blender-cloud-addon/flamenco-test__intermediate/render-smpl-0001-0084-frm-######",
		s.task.Commands[2].Settings["render_output"],
	)
	assert.Equal(t, "{hey}/haha", s.task.Commands[2].Settings["otherpath"])
}

func (s *VarReplTestSuite) TestReplacePathsUnknownOS(t *check.C) {
	worker := Worker{Platform: "autumn"}
	ReplaceVariables(&s.config, &s.task, &worker)

	assert.Equal(t,
		"hey/_flamenco/storage/sybren/2017-06-08-181223.625800-sybren-flamenco-test.flamenco/flamenco-test.flamenco.blend",
		s.task.Commands[2].Settings["filepath"],
	)
	assert.Equal(t,
		"{render_long}/_flamenco/output/sybren/blender-cloud-addon/flamenco-test__intermediate/render-smpl-0001-0084-frm-######",
		s.task.Commands[2].Settings["render_output"],
	)
	assert.Equal(t, "{hey}/haha", s.task.Commands[2].Settings["otherpath"])
}

func (s *VarReplTestSuite) TestReplaceLocal(t *check.C) {
	assert.Equal(t, "", ReplaceLocal("", &s.config))
	assert.Equal(t, "hheyyyy", ReplaceLocal("hheyyyy", &s.config))
	assert.Equal(t, "{unknown}", ReplaceLocal("{unknown}", &s.config))

	expected, ok := map[string]string{
		"windows": "r:/here",
		"linux":   "/render/here",
		"darwin":  "/Volume/render/here",
	}[runtime.GOOS]
	if !ok {
		panic("unknown runtime OS '" + runtime.GOOS + "'")
	}
	assert.Equal(t, expected, ReplaceLocal("{render}/here", &s.config))
}
