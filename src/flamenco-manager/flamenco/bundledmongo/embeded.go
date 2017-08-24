package bundledmongo

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"

	log "github.com/sirupsen/logrus"
)

// Runner provides an interface to start & stop the mongod executable.
type Runner struct {
	databasePath string
	localPort    int
	context      context.Context
	cancel       context.CancelFunc
	cmd          *exec.Cmd
	mutex        sync.Mutex
}

// CreateMongoRunner creates a new MongoRunner but doesn't start it yet.
func CreateMongoRunner(databasePath string, localPort int) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{
		databasePath: databasePath,
		localPort:    localPort,
		context:      ctx,
		cancel:       cancel,
	}
}

// Go starts mongodb and keeps it running in the background.
func (runner *Runner) Go() error {
	runner.mutex.Lock()
	defer runner.mutex.Unlock()
	var err error

	log.Infof("Starting MongoDB from path %s on port %d",
		runner.databasePath, runner.localPort)

	ensureDirExists(runner.databasePath, "database path")
	ensureDirExists("mongodb-logs", "MongoDB logs path")

	localPortStr := fmt.Sprintf("%d", runner.localPort)
	log.Debugf("Local port string is %v", localPortStr)

	runner.cmd = exec.CommandContext(
		runner.context,
		mongoDPath,
		"--port", localPortStr,
		"--bind_ip", "127.0.0.1",
		"--dbpath", runner.databasePath,
		"--quiet",
		"--logpath", "mongodb-logs/mongodb.log",
	)

	var stdout io.ReadCloser
	stdout, err = runner.cmd.StdoutPipe()
	if err != nil {
		log.Panicf("Unable to get pipe to MongoDB stdout: %s", err)
	}

	if err = runner.cmd.Start(); err != nil {
		log.Fatalf("Unable to start MongoDB: %s", err)
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		done := runner.context.Done()
		for {
			select {
			case <-done:
				log.Errorf("Runner context done, stopping reading!")
				return
			default:
			}

			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					log.Errorf("MongoDB: %s", err)
				}
				return
			}
			log.Warningf("MongoDB: %s", scanner.Text())
		}
	}()

	log.Infof("MongoDB is running at PID %d", runner.cmd.Process.Pid)
	return nil
}

// Close gracefully stops mongod.
func (runner *Runner) Close() {
	runner.mutex.Lock()
	defer runner.mutex.Unlock()

	if runner.cmd == nil {
		log.Info("Stopping MongoDB even before it started.")
		return
	}

	if err := runner.cmd.Process.Kill(); err != nil {
		log.Errorf("Error killing MongoDB process: %s", err)
		return
	}

	err := runner.cmd.Wait()
	if err != nil {
		log.Errorf("Error waiting for MongoDB: %s", err)
	}

	if runner.cmd.ProcessState.Success() {
		log.Info("Gracefully shut down MongoDB server")
	} else {
		log.Warning("MongoDB server did not shut down gracefully.")
	}
}
