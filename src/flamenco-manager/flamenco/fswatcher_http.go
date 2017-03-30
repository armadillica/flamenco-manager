package flamenco

import (
	"flamenco-manager/flamenco/chantools"
	"fmt"
	"net/http"

	log "github.com/Sirupsen/logrus"
)

// ImageWatcherHTTPPush starts a server-side events channel.
func ImageWatcherHTTPPush(w http.ResponseWriter, r *http.Request, broadcaster *chantools.OneToManyChan) {
	log.Println(r.RemoteAddr, "Channel started at", r.URL.Path)

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

	defer log.Infof(r.RemoteAddr, "Finished HTTP request at", r.URL.Path)

	// Hook our channel up to the image broadcaster.
	pathChannel := make(chan string)
	broadcaster.AddOutputChan(pathChannel)
	defer broadcaster.RemoveOutputChan(pathChannel)

	for {
		select {
		case <-closeNotifier.CloseNotify():
			log.Println(r.RemoteAddr, "Connection closed.")
			return
		case path, ok := <-pathChannel:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: image\n")
			fmt.Fprintf(w, "data: %s\n\n", path)
			f.Flush()
		}
	}
}
