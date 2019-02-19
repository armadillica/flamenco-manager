package flamenco

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/mgo.v2/bson"
)

const (
	logBytesHead = 5 * 1024
	logBytesTail = 10 * 1024
)

// ServeTaskLog serves the latest task log file for the given job+task.
// Depending on the User-Agent header it servers head+tail or the entire file.
func ServeTaskLog(w http.ResponseWriter, r *http.Request,
	jobID, taskID bson.ObjectId, tuq *TaskUpdateQueue) {

	dirname, basename := tuq.taskLogPath(jobID, taskID)
	filename := filepath.Join(dirname, basename)

	userAgent := r.Header.Get("User-Agent")
	showEntireFile := strings.HasPrefix(userAgent, "Wget/") || strings.HasPrefix(userAgent, "curl/")

	logger := log.WithFields(log.Fields{
		"remote_addr": r.RemoteAddr,
		"log_file":    filename,
		"entire_file": showEntireFile,
	})

	stat, err := os.Stat(filename)
	if err != nil {
		if os.IsNotExist(err) {
			// Attempt to access the gzipped file.
			filename += ".gz"
			basename = path.Base(filename)
			stat, err = os.Stat(filename)
			logger = logger.WithField("log_file", filename)
		}
		if os.IsNotExist(err) {
			logger.Info("unable to find task log file")
			http.Error(w, "unable to stat task log file", http.StatusNotFound)
			return
		}
		if err != nil {
			logger.WithError(err).Error("unable to stat task log file")
			http.Error(w, "unable to access task log file", http.StatusInternalServerError)
		}

		// If we're here, we could succesfully stat the gzipped file.
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+basename+"\"")
		showEntireFile = true
		logger = logger.WithField("entire_file", showEntireFile)
	} else {
		// Check the file size -- if it's smaller than head + tail combined, we just send the entire thing.
		// This only makes sense when we send files uncompressed, though.
		showEntireFile = showEntireFile || stat.Size() < int64(logBytesHead+logBytesTail)
	}

	logger.Info("serving task log file")

	w.Header().Set("Content-Type", "text/plain")
	if showEntireFile {
		http.ServeFile(w, r, filename)
		return
	}

	logfile, err := os.Open(filename)
	if err != nil {
		logger.WithError(err).Error("unable to open task log file")
		http.Error(w, "unable to open task log file", http.StatusInternalServerError)
		return
	}
	defer logfile.Close()

	w.WriteHeader(http.StatusOK)
	reader := bufio.NewReader(logfile)
	if _, err := io.CopyN(w, reader, logBytesHead); err != nil {
		logger.WithError(err).Info("unable to copy log file head to HTTP client")
		return
	}

	offset, err := logfile.Seek(-logBytesTail, 2) // 2 = 'from the end'
	if err != nil {
		logger.WithError(err).Info("unable to seek in log file")
		return
	}

	msg := "...\n\n... Skipped %d bytes, use WGet or Curl to download the entire log ... \n\n"
	if _, err := fmt.Fprintf(w, msg, offset-logBytesHead); err != nil {
		logger.WithError(err).Info("unable to copy log file 'skipped' bit to HTTP client")
		return
	}

	reader.Reset(logfile)
	reader.ReadLine() // just skip until the end of the current line, so we only present entire lines.
	if _, err := io.Copy(w, reader); err != nil {
		logger.WithError(err).Info("unable to copy log file tail to HTTP client")
		return
	}
}
