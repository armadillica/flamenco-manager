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
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path"
	"sync"

	log "github.com/sirupsen/logrus"
	"gopkg.in/mgo.v2/bson"
)

// TaskLogUploader sends compressed task log files to Flamenco Server.
//
// The task IDs are queued first. If the tuple is already queued, queueing is a
// no-op, even when uploading is already in progress. This allows the Server to
// maintain the queue of to-be-uploaded task logs; we don't have to persist
// anything to disk.
type TaskLogUploader struct {
	closable
	sync.RWMutex
	config        *Conf
	queue         chan JobTask
	alreadyQueued map[JobTask]bool
	upstream      *UpstreamConnection
}

const taskLogUploadQueueSize = 1

// CreateTaskLogUploader creates a new TaskLogUploader.
func CreateTaskLogUploader(config *Conf, upstream *UpstreamConnection) *TaskLogUploader {
	tlu := TaskLogUploader{
		closable:      makeClosable(),
		config:        config,
		queue:         make(chan JobTask, taskLogUploadQueueSize),
		alreadyQueued: make(map[JobTask]bool),
		upstream:      upstream,
	}
	return &tlu
}

func (tlu *TaskLogUploader) isAlreadyQueued(jobTask JobTask) bool {
	tlu.RLock()
	defer tlu.RUnlock()

	return tlu.alreadyQueued[jobTask]
}

func (tlu *TaskLogUploader) markAlreadyQueued(jobTask JobTask) {
	tlu.Lock()
	defer tlu.Unlock()

	tlu.alreadyQueued[jobTask] = true
}

func (tlu *TaskLogUploader) unmarkAlreadyQueued(jobTask JobTask) {
	tlu.Lock()
	defer tlu.Unlock()

	delete(tlu.alreadyQueued, jobTask)
}

// QueueAll places all (Job ID, Task ID) tuples on the queue for uploading later.
// This function will keep getting called with tasks until those tasks have had
// their logfile uploaded to the Server.
func (tlu *TaskLogUploader) QueueAll(jobTasks []JobTask) {
	for _, jobTask := range jobTasks {
		if tlu.isAlreadyQueued(jobTask) {
			log.WithFields(log.Fields{
				"job_id":  jobTask.Job.Hex(),
				"task_id": jobTask.Task.Hex(),
			}).Debug("skipped already-queued task log upload request")
			continue
		}
		tlu.markAlreadyQueued(jobTask)

		select {
		case tlu.queue <- jobTask:
			log.WithFields(log.Fields{
				"job_id":  jobTask.Job.Hex(),
				"task_id": jobTask.Task.Hex(),
			}).Debug("queued request to upload task log")
		default:
			log.WithFields(log.Fields{
				"received": len(jobTasks),
				"queued":   taskLogUploadQueueSize,
			}).Debug("received too many task upload requests, only handling a subset now")
			return
		}
	}
}

// Close gracefully shuts down the task uploader goroutine.
func (tlu *TaskLogUploader) Close() {
	log.Debug("TaskLogUploader: Close() called.")
	close(tlu.queue)
	tlu.closableCloseAndWait()
	log.Debug("TaskLogUploader: shutdown complete.")
}

// Go starts a goroutine that monitors the queue and uploads task logs.
func (tlu *TaskLogUploader) Go() {
	tlu.closableAdd(1)
	go func() {
		defer tlu.closableDone()
		defer log.Info("TaskLogUploader: shutting down.")

		for jobTask := range tlu.queue {
			tlu.compressAndUpload(jobTask)
			tlu.unmarkAlreadyQueued(jobTask)
		}
	}()
}

func (tlu *TaskLogUploader) compressAndUpload(jobTask JobTask) {
	dirpath, filename := taskLogPath(jobTask.Job, jobTask.Task, tlu.config)
	filepath := path.Join(dirpath, filename)

	logger := log.WithFields(log.Fields{
		"job_id":   jobTask.Job.Hex(),
		"task_id":  jobTask.Task.Hex(),
		"filepath": filepath,
	})

	filepath, err := tlu.compressFile(filepath, logger)
	if err != nil {
		logger.WithError(err).Error("unable to compress log file for uploading to Server")
		return
	}

	url, err := tlu.upstream.ResolveURL("/api/flamenco/managers/%s/attach-task-log/%s", tlu.config.ManagerID, jobTask.Task.Hex())
	if err != nil {
		logger.WithError(err).Error("unable to resolve URL to attach-task-log Server endpoint")
		return
	}
	logger = logger.WithFields(log.Fields{
		"url":      url.String(),
		"filepath": filepath,
	})
	tlu.uploadFile(jobTask.Task, filepath, url.String(), logger)
}

// Compress the given file and return the compressed file's path.
// If the compressed file already exists on disk and is newer than the original,
// the original won't be recompressed.
func (tlu *TaskLogUploader) compressFile(filepath string, logger *log.Entry) (string, error) {
	gzPath := filepath + ".gz"
	origStat, origErr := os.Stat(filepath)
	gzStat, gzErr := os.Stat(gzPath)

	if gzErr == nil && os.IsNotExist(origErr) {
		// The original is gone, but the gzipped log file is still there.
		logger.Debug("plain log file does not exist, but gzipped does")
		return gzPath, nil
	}
	if origErr == nil && gzErr == nil && origStat.ModTime().Before(gzStat.ModTime()) {
		// Both exist, and gzipped file is newer.
		logger.Debug("plain and gzipped log file does not exist, gzipped is newer")
		return gzPath, nil
	}
	if os.IsNotExist(origErr) {
		// Original file does not exist, so just create a file that states this so that
		// we at least have something to send to the Server. Otherwise the server will
		// keep asking us for the log file indefinitely.
		logger.Debug("requested log file does not exist")
		err := ioutil.WriteFile(filepath, []byte("log file does not exist on Flamenco Manager"), 0777)
		if err != nil {
			logger.WithError(err).Error("unable to write 'the log file does not exist'")
			return "", err
		}
		origStat, origErr = os.Stat(filepath)
	}
	if origErr != nil {
		// Original cannot be accessed, and gzipped file is not there either.
		logger.WithError(origErr).Debug("log file cannot be accessed, but it does exist")
		return "", origErr
	}

	logger = logger.WithField("bytes_uncompressed", origStat.Size())
	logger.Info("compressing logfile")

	origFile, err := os.Open(filepath)
	if err != nil {
		logger.WithError(err).Debug("unable to open logfile for reading")
		return "", err
	}
	defer origFile.Close()

	gzFile, err := os.Create(gzPath)
	if err != nil {
		logger.WithError(err).Debug("error creating compressed logfile for writing")
		return "", err
	}
	defer gzFile.Close()

	gzWriter, err := gzip.NewWriterLevel(gzFile, 9)
	if err != nil {
		logger.WithError(err).Debug("unable to create GZip writer")
		return "", err
	}
	defer gzWriter.Close()

	_, err = io.Copy(gzWriter, origFile)
	if err != nil {
		logger.WithError(err).Debug("error copying bytes from plain to gzipped log")
		return "", err
	}
	return gzPath, nil
}

func (tlu *TaskLogUploader) uploadFile(taskID bson.ObjectId, filepath string, url string, logger *log.Entry) {
	fileReader, err := os.Open(filepath)
	if err != nil {
		logger.WithError(err).Error("unable to open compressed log file for uploading")
		return
	}

	// Read the compressed file into memory to construct the multipart/form body.
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="logfile"; filename="task-%s.txt.gz"`, taskID.Hex()))
	header.Set("Content-Type", "text/plain+gzip")
	fieldwriter, err := w.CreatePart(header)
	if err != nil {
		logger.WithError(err).Error("unable to create multipart/form field writer")
		return
	}
	if _, err := io.Copy(fieldwriter, fileReader); err != nil {
		logger.WithError(err).Error("unable to read compressed log file")
		return
	}
	w.Close()

	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		logger.WithError(err).Error("unable to create POST request")
		return
	}
	req.SetBasicAuth(tlu.config.ManagerSecret, "")
	req.Header.Set("Content-Type", w.FormDataContentType())

	logger.Info("uploading task log")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.WithError(err).Error("error uploading task log file")
		return
	}
	logger = logger.WithField("http_status", resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		logger.WithError(err).Warning("error reading response to uploaded task log file")
		return
	}

	if resp.StatusCode >= 300 {
		if resp.StatusCode != 404 {
			logger = logger.WithField("body", string(body))
		}
		logger.Warning("received error response from server after uploading task log file")
		return
	}

	logger.Info("task log file uploaded succesfully")
}
