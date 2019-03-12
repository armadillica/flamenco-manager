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
