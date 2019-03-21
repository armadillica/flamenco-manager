package main

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
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/kardianos/osext"
	log "github.com/sirupsen/logrus"
)

func restart() {
	exename, err := osext.Executable()
	if err != nil {
		log.Fatal(err)
	}

	isWindows := runtime.GOOS == "windows"

	args := []string{}
	if cliArgs.debug {
		args = append(args, "-debug")
	} else if cliArgs.verbose {
		args = append(args, "-verbose")
	}
	if cliArgs.jsonLog {
		args = append(args, "-json")
	}
	if isWindows {
		args = append(args, "-kill-after-start")
		args = append(args, fmt.Sprintf("%d", syscall.Getpid()))
	}
	cmd := exec.Command(exename, args...)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logFields := log.Fields{
		"exename": exename,
		"args":    args,
	}
	if err = cmd.Start(); err != nil {
		log.WithFields(logFields).WithError(err).Fatal("Failed to launch new Manager")
	}
	log.WithFields(logFields).Info("Started another Flamenco Manager")

	// Give the other process time to start. This is required on Windows. Our child will kill us
	// when it has started succesfully.
	if isWindows {
		time.Sleep(15 * time.Second)
	}
}

func killParentProcess() {

	if cliArgs.killPID == 0 {
		return
	}

	logger := log.WithField("pid", cliArgs.killPID)

	proc, err := os.FindProcess(cliArgs.killPID)
	if err != nil {
		logger.Debug("Unable to find parent process, will not terminate it.")
		return
	}

	err = proc.Kill()
	if err != nil {
		logger.WithError(err).Warning("Unable to terminate parent process.")
	} else {
		logger.Debug("Parent process terminated.")
	}
}
