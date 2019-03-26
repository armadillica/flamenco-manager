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
	"time"

	"github.com/armadillica/flamenco-manager/jwtauth"
	shamanconfig "github.com/armadillica/flamenco-manager/shaman/config"
	log "github.com/sirupsen/logrus"
)

// ConfV1 is the first version of the configuration, and is implied
// when the YAML file doesn't have a version declared.
type ConfV1 struct {
	Base `yaml:",inline"`

	// VariablesByVarname: variable name -> platform -> value
	VariablesByVarname       map[string]map[string]string `yaml:"variables"`
	PathReplacementByVarname map[string]map[string]string `yaml:"path_replacement"`
}

// defaultConfigV1 returns a V1 configuration set to the V1 defaults.
func defaultConfigV1() ConfV1 {
	confV1 := ConfV1{
		Base: Base{
			Meta: ConfMeta{Version: 1},

			Mode:                        "production",
			ManagerName:                 "Flamenco Manager",
			Listen:                      ":8083",
			DatabasePath:                "./db",
			TaskLogsPath:                "./task-logs",
			DownloadTaskSleep:           10 * time.Minute,
			DownloadTaskRecheckThrottle: 10 * time.Second,
			TaskUpdatePushMaxInterval:   5 * time.Second,
			TaskUpdatePushMaxCount:      3000,
			CancelTaskFetchInterval:     10 * time.Second,
			ActiveTaskTimeoutInterval:   10 * time.Minute,
			ActiveWorkerTimeoutInterval: 1 * time.Minute,
			FlamencoStr:                 defaultServerURL,
			// Days are assumed to be 24 hours long. This is not exactly accurate, but should
			// be accurate enough for this type of cleanup.
			TaskCleanupMaxAge: 14 * 24 * time.Hour,
			SSDPDiscovery:     true,
			SSDPDeviceUUID:    "7401c189-ef69-434b-b4d8-56d00075faf5",

			BlacklistThreshold:         3,
			TaskFailAfterSoftFailCount: 3,

			WorkerCleanupStatus: []string{workerStatusOffline},

			TestTasks: TestTasks{
				BlenderRender: BlenderRenderConfig{
					JobStorage:   "{job_storage}/test-jobs",
					RenderOutput: "{render}/test-renders",
				},
			},

			Shaman: shamanconfig.Config{
				FileStorePath: "../shaman-file-store",
				CheckoutPath:  "../shaman-checkout",

				GarbageCollect: shamanconfig.GarbageCollect{
					Period:            0,
					MaxAge:            31 * 24 * time.Hour,
					ExtraCheckoutDirs: []string{},
				},
			},

			JWT: jwtauth.Config{
				DownloadKeysInterval: 1 * time.Hour,
			},
		},

		VariablesByVarname: map[string]map[string]string{
			"blender": map[string]string{
				"linux":   "/linux/path/to/blender",
				"windows": "C:/windows/path/to/blender.exe",
				"darwin":  "/Volume/Applications/Blender/blender",
			},
			"ffmpeg": map[string]string{
				"linux":   "/usr/bin/ffmpeg",
				"windows": "C:/windows/path/to/ffmpeg.exe",
				"darwin":  "/Volume/Applications/FFmpeg/ffmpeg",
			},
		},

		PathReplacementByVarname: map[string]map[string]string{
			"render": map[string]string{
				"linux":   "/render",
				"windows": "R:",
				"darwin":  "/Volume/render",
			},
			"job_storage": map[string]string{
				"linux":   "/shared/flamenco-jobs",
				"windows": "S:",
				"darwin":  "/Volume/shared/flamenco-jobs",
			},
		},
	}

	return confV1
}

// upgradeToV2 copies confV1's settings into confV2.
func (confV1 *ConfV1) upgradeToV2(confV2 *Conf) {
	if confV1.Meta.Version > 1 {
		log.WithField("version", confV1.Meta.Version).Panic("upgradeToV2() called on too new config")
	}
	confV2.Base = confV1.Base
	confV2.Meta.Version = 2

	confV2.Variables = map[string]ConfV2Variable{}
	for varName, perPlatform := range confV1.VariablesByVarname {
		confV2.Variables[varName] = convertVariablesV1toV2(varName, perPlatform, "oneway")
	}
	for varName, perPlatform := range confV1.PathReplacementByVarname {
		confV2.Variables[varName] = convertVariablesV1toV2(varName, perPlatform, "twoway")
	}

}

func convertVariablesV1toV2(varname string, v1var map[string]string, direction string) ConfV2Variable {
	logger := log.WithField("name", varname)

	v2var := ConfV2Variable{
		Direction: direction,
		Values:    ConfV2VariableValues{},
	}
	for platform, value := range v1var {
		if value == "" {
			logger.WithField("platform", platform).Debug("skipping empty value")
			continue
		}
		value := ConfV2VariableValue{
			Audience: "all",
			Value:    value,
		}
		value = value.addPlatform(platform)
		v2var.Values = append(v2var.Values, value)
	}
	return v2var
}

func (c ConfV2VariableValue) addPlatform(platform string) ConfV2VariableValue {
	if c.Platform == "" {
		if len(c.Platforms) == 0 {
			c.Platform = platform
		} else {
			c.Platforms = append(c.Platforms, platform)
		}
	} else {
		c.Platforms = []string{c.Platform, platform}
		c.Platform = ""
	}
	return c
}
