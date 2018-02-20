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
