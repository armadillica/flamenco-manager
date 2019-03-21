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
	"github.com/stretchr/testify/assert"

	check "gopkg.in/check.v1"
)

type SettingsTestSuite struct{}

var _ = check.Suite(&SettingsTestSuite{})

func (s *SettingsTestSuite) TestDefaultSettings(t *check.C) {
	config, err := LoadConf("nonexistant.yaml")
	assert.NotNil(t, err) // should indicate an error to open the file.

	assert.Equal(t, "./task-logs", config.TaskLogsPath)

	// The settings should contain the defaults, though.
	assert.Equal(t, "7401c189-ef69-434b-b4d8-56d00075faf5", config.SSDPDeviceUUID)
	assert.Contains(t, config.PathReplacementByVarname, "job_storage")
	assert.Contains(t, config.PathReplacementByVarname, "render")

	linuxPVars, ok := config.PathReplacementByPlatform["linux"]
	assert.True(t, ok)
	assert.Equal(t, "/shared/flamenco-jobs", linuxPVars["job_storage"])

	winPVars, ok := config.PathReplacementByPlatform["windows"]
	assert.True(t, ok)
	assert.Equal(t, "S:", winPVars["job_storage"])
}

func (s *SettingsTestSuite) TestDuplicateVars(t *check.C) {
	config, err := LoadConf("settings_test_duplicate_vars.yaml")
	assert.Equal(t, ErrDuplicateVariables, err)

	// The settings should contain the defaults.
	assert.Equal(t, "7401c189-ef69-434b-b4d8-56d00075faf5", config.SSDPDeviceUUID)
}
