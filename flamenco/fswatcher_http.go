package flamenco

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/armadillica/flamenco-manager/flamenco/chantools"

	log "github.com/sirupsen/logrus"
)

// ImageWatcherHTTPPush starts a server-side events channel.
func ImageWatcherHTTPPush(w http.ResponseWriter, r *http.Request, broadcaster *chantools.OneToManyChan) {
	log.Infof("ImageWatcherHTTPPush: Channel started at %s for %s", r.URL.Path, r.RemoteAddr)

	// Make sure that the writer supports flushing.
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Listen to the closing of the http connection via the CloseNotifier
	closeNotifier, ok := w.(http.CloseNotifier)
	if !ok {
		http.Error(w, "Cannot stream", http.StatusInternalServerError)
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

	defer log.Debugf("Finished HTTP request at %s from %s", r.URL.Path, r.RemoteAddr)

	// Hook our channel up to the image broadcaster.
	pathChannel := make(chan string)
	broadcaster.AddOutputChan(pathChannel)
	defer broadcaster.RemoveOutputChan(pathChannel)

	for {
		select {
		case <-closeNotifier.CloseNotify():
			log.Debugf("ImageWatcher: Connection from %s closed", r.RemoteAddr)
			return
		case path, ok := <-pathChannel:
			if !ok {
				// Shutting down.
				return
			}
			log.Debugf("ImageWatcher: Sending notification to %s", r.RemoteAddr)
			fmt.Fprintf(w, "event: image\n")
			fmt.Fprintf(w, "data: %s\n\n", filepath.Base(path))
			f.Flush()
		}
	}
}
