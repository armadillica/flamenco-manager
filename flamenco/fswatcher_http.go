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
	"net/http"
	"path/filepath"

	"github.com/armadillica/flamenco-manager/flamenco/chantools"

	log "github.com/sirupsen/logrus"
)

// ImageWatcherHTTPPush starts a server-side events channel.
func ImageWatcherHTTPPush(w http.ResponseWriter, r *http.Request, broadcaster *chantools.OneToManyChan) {
	logger := log.WithFields(log.Fields{
		"remote_addr": r.RemoteAddr,
		"url":         r.URL.Path,
	})

	// Make sure that the writer supports flushing.
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		logger.Error("ImageWatcher: Streaming unsupported; writer does not implement http.Flusher interface")
		return
	}

	// Listen to the closing of the http connection via the CloseNotifier
	closeNotifier, ok := w.(http.CloseNotifier)
	if !ok {
		http.Error(w, "Cannot stream", http.StatusInternalServerError)
		logger.Error("ImageWatcher: Streaming unsupported; writer does not implement http.CloseNotifier interface")
		return
	}

	// Set the headers related to event streaming.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	f.Flush()

	// Firefox really want to have an immediate event on the channel,
	// or it'll think that the connection wasn't made properly.
	fmt.Fprintf(w, "event: notification\n")
	fmt.Fprintf(w, "data: hello there!\n\n")
	f.Flush()

	logger.Info("ImageWatcher: Channel started")
	defer logger.Debug("ImageWatcher: Closed HTTP stream")

	// Hook our channel up to the image broadcaster.
	pathChannel := make(chan string)
	broadcaster.AddOutputChan(pathChannel)
	defer broadcaster.RemoveOutputChan(pathChannel)

	for {
		select {
		case <-closeNotifier.CloseNotify():
			logger.Debug("ImageWatcher: Connection closed")
			return
		case path, ok := <-pathChannel:
			if !ok {
				// Shutting down.
				return
			}
			logger.Debug("ImageWatcher: Sending notification")
			fmt.Fprintf(w, "event: image\n")
			fmt.Fprintf(w, "data: %s\n\n", filepath.Base(path))
			f.Flush()
		}
	}
}
