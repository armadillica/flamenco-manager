package flamenco

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
)

var stringType = reflect.TypeOf("somestring")

// ReplaceVariables performs variable and path replacement for tasks.
func ReplaceVariables(config *Conf, task *Task, worker *Worker) {
	varmap := config.VariablesByPlatform[worker.Platform]
	pathmap := config.PathReplacementByPlatform[worker.Platform]

	for _, cmd := range task.Commands {
		for key, value := range cmd.Settings {
			// Only do replacement on string types
			if reflect.TypeOf(value) != stringType {
				continue
			}

			strvalue := reflect.ValueOf(value).String()
			// Variable replacement
			for varname, varvalue := range varmap {
				placeholder := fmt.Sprintf("{%s}", varname)
				strvalue = strings.Replace(strvalue, placeholder, varvalue, -1)
			}
			// Path replacement
			for varname, varvalue := range pathmap {
				placeholder := fmt.Sprintf("{%s}", varname)
				strvalue = strings.Replace(strvalue, placeholder, varvalue, -1)
			}

			cmd.Settings[key] = strvalue
		}
	}
}

// ReplaceLocal performs variable and path replacement for strings based on the local platform.
func ReplaceLocal(strvalue string, config *Conf) string {
	varmap := config.VariablesByPlatform[runtime.GOOS]
	pathmap := config.PathReplacementByPlatform[runtime.GOOS]

	// Variable replacement
	for varname, varvalue := range varmap {
		placeholder := fmt.Sprintf("{%s}", varname)
		strvalue = strings.Replace(strvalue, placeholder, varvalue, -1)
	}
	// Path replacement
	for varname, varvalue := range pathmap {
		placeholder := fmt.Sprintf("{%s}", varname)
		strvalue = strings.Replace(strvalue, placeholder, varvalue, -1)
	}

	return strvalue
}
