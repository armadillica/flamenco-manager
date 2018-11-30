package flamenco

import (
	"net/http"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"gopkg.in/mgo.v2/bson"
)

// ServeTaskLog serves the latest task log file for the given job+task.
func ServeTaskLog(w http.ResponseWriter, r *http.Request,
	jobID, taskID bson.ObjectId, tuq *TaskUpdateQueue) {

	dirname, basename := tuq.taskLogPath(jobID, taskID)
	filename := filepath.Join(dirname, basename)

	log.WithFields(log.Fields{
		"remote_addr": r.RemoteAddr,
		"log_file":    filename,
	}).Info("serving task log file")

	http.ServeFile(w, r, filename)
}
