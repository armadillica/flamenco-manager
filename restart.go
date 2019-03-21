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

package main

import (
	"runtime"
	"strconv"
	"syscall"

	"github.com/kardianos/osext"
	log "github.com/sirupsen/logrus"
)

// Try to restart in an environment-dependent way.
// - Windows: the child kills the parent (the child is killed if the parent stops too early).
// - POSIX: we use execve() to replace the current process with a new one.
func restart() {
	exename, err := osext.Executable()
	if err != nil {
		log.WithError(err).Fatal("unable to determine the path of the currently running executable")
	}

	args := reconstructCliForRestart()
	platformSpecificRestart(exename, args)
}

func reconstructCliForRestart() []string {
	args := []string{
		"-mode", cliArgs.mode,
	}

	if cliArgs.debug {
		args = append(args, "-debug")
	} else if cliArgs.quiet {
		args = append(args, "-quiet")
	}
	if cliArgs.jsonLog {
		args = append(args, "-json")
	}
	if cliArgs.setup {
		args = append(args, "-setup")
	}

	if runtime.GOOS == "windows" {
		args = append(args, "-kill-after-start")
		args = append(args, strconv.Itoa(syscall.Getpid()))
	}

	return args
}
