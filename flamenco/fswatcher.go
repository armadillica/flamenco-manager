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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/armadillica/flamenco-manager/flamenco/chantools"

	auth "github.com/abbot/go-http-auth"
	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

var imageExtensions = map[string]bool{
	".exr": true,
	".jpg": true,
	".png": true,
}

// If a file hasn't been written to in this amount of time,
// it's considered "old enough" to be considered "written".
const fileAgeThreshold = 2 * time.Second

// Maximum number of images to queue up for conversion. The output channel returned
// by ConvertAndForward() will block after this many.
const imageQueueSize = 3

// LatestImageSystem ties an ImageWatcher to a the fswatcher_middle and fswatcher_http stuff,
// allowing the results to be pushed via HTTP to browsers.
type LatestImageSystem struct {
	imageWatcher *ImageWatcher
	broadcaster  *chantools.OneToManyChan

	// Paths to images can be sent to this channel, and will be
	// run through the middleware and sent to the broadcaster.
	imageCreated chan string
}

// Struct to keep track of image files in a heap.
type imageFile struct {
	path      string
	lastWrite time.Time
}

// Mapping from path name to imageFile pointer that's stored in the heap.
type imageMap map[string]*imageFile

// ImageWatcher watches a filesystem directory.
type ImageWatcher struct {
	closable
	pathToWatch string
	watcher     *fsnotify.Watcher

	// Our internal channel, where we can send stuff into.
	imageCreated chan string

	// The public channel, from which can only be read.
	ImageCreated <-chan string

	// Image information so that we can wait for writes to images
	// to be long enough ago to consider them "fully written".
	imageMapLock *sync.Mutex
	imageMap     imageMap
}

// CreateLatestImageSystem sets up a LatestImageSystem
func CreateLatestImageSystem(watchPath string) *LatestImageSystem {
	lis := &LatestImageSystem{}

	if watchPath != "" {
		lis.imageWatcher = CreateImageWatcher(watchPath, imageQueueSize)
		lis.imageCreated = lis.imageWatcher.imageCreated
	} else {
		lis.imageCreated = make(chan string, imageQueueSize)
	}

	middleware := ConvertAndForward(lis.imageCreated, "static")
	lis.broadcaster = chantools.NewOneToManyChan(middleware)

	return lis
}

// AddRoutes adds the HTTP Server-Side Events endpoint to the router.
func (lis *LatestImageSystem) AddRoutes(router *mux.Router, workerAuth *auth.BasicAuth) {
	router.HandleFunc("/imagewatch", lis.serverSideEvents).Methods("GET")
	router.HandleFunc("/output-produced", workerAuth.Wrap(lis.outputProduced)).Methods("POST")

	// Just for logging stuff, nothing special.
	go func() {
		pathChannel := make(chan string)
		lis.broadcaster.AddOutputChan(pathChannel)
		defer lis.broadcaster.RemoveOutputChan(pathChannel)

		for path := range pathChannel {
			log.Infof("New image rendered: %s", path)
		}
	}()
}

func (lis *LatestImageSystem) serverSideEvents(w http.ResponseWriter, r *http.Request) {
	ImageWatcherHTTPPush(w, r, lis.broadcaster)
}

func (lis *LatestImageSystem) outputProduced(w http.ResponseWriter, r *auth.AuthenticatedRequest) {
	if r.Body == nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "No HTTP body received")
		return
	}
	payload := FileProduced{}
	defer r.Body.Close()
	if err := DecodeJSON(w, r.Body, &payload, "LatestImageSystem::outputProduced()"); err != nil {
		return
	}

	// I don't want to hook up MongoDB to the LatestImageSystem, so we can't find
	// the worker's nickname.
	logFields := log.Fields{
		"worker":      r.Username,
		"remote_addr": r.RemoteAddr,
	}

	if len(payload.Paths) == 0 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, "List of paths may not be empty.")
		return
	}

	log.WithFields(logFields).WithField("image_count", len(payload.Paths)).Debug("LatestImageSystem: files were produced on worker")
	for _, path := range payload.Paths {
		logFields["path"] = path

		// Send the path to the channel to push it through the conversion & to the browser.
		select {
		case lis.imageCreated <- path:
			log.WithFields(logFields).Info("LatestImageSystem: output was queued for conversion")
		default:
			log.WithFields(logFields).Warning("LatestImageSystem: output was discarded, conversion queue is full")
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// Go starts the image watcher, if the path to watch isn't empty.
func (lis *LatestImageSystem) Go() {
	if lis.imageWatcher == nil {
		return
	}

	lis.imageWatcher.Go()
}

// Close gracefully shuts down the image watcher, if the path to watch isn't empty.
func (lis *LatestImageSystem) Close() {
	if lis.imageWatcher == nil {
		return
	}
	close(lis.imageCreated)
	lis.imageWatcher.Close()
}

// CreateImageWatcher creates a new ImageWatcher for the given directory.
// bufferSize is the size of the iw.ImageCreated channel.
func CreateImageWatcher(pathToWatch string, bufferSize int) *ImageWatcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Panicf("Unable to start FSNotify watcher: %s", err)
	}

	absPathToWatch, err := filepath.Abs(pathToWatch)
	if err != nil {
		log.Panicf("Unable to turn \"%s\" into an absolute path: %s", pathToWatch, err)
	}

	info, err := os.Stat(absPathToWatch)
	if err != nil {
		log.Fatalf("ImageWatcher: %s", err)
	}
	// Handle directory renames as remove + add.
	if !info.IsDir() {
		log.Fatalf("ImageWatcher: path to watch must be a directory: %s", absPathToWatch)
	}

	imageCreated := make(chan string, bufferSize)

	iw := &ImageWatcher{
		makeClosable(),
		absPathToWatch,
		watcher,
		imageCreated,
		imageCreated,
		new(sync.Mutex),
		imageMap{},
	}

	return iw
}

// Go starts the watcher in a separate gofunc.
func (iw *ImageWatcher) Go() {
	go iw.imageMapLoop()
	go iw.fswatchLoop()

	// TODO: do a recursive scan of the to-watch directory and report
	// on the last-created image that can be found there.
}

// fswatchLoop watches the file system recursively.
func (iw *ImageWatcher) fswatchLoop() {
	iw.closableAdd(1)
	defer iw.closableDone()

	log.Infof("ImageWatcher: monitoring %s", iw.pathToWatch)
	defer log.Debug("ImageWatcher: shutting down")

	iw.createPath(iw.pathToWatch)

	for {
		select {
		case <-iw.closable.doneChan:
			return
		case err := <-iw.watcher.Errors:
			if err != nil {
				log.Errorf("ImageWatcher: error %s", err)
			}
		case event := <-iw.watcher.Events:
			switch {
			case event.Op&fsnotify.Create == fsnotify.Create:
				// log.Debugf("ImageWatcher: Create %s", event.Name)
				iw.createPath(event.Name)
			case event.Op&fsnotify.Write == fsnotify.Write:
				// log.Debugf("ImageWatcher: Write %s", event.Name)
				iw.queueImage(event.Name)
			case event.Op&fsnotify.Remove == fsnotify.Remove:
				// log.Debugf("ImageWatcher: Remove %s", event.Name)
				iw.removePath(event.Name)
			case event.Op&fsnotify.Rename == fsnotify.Rename:
				// log.Debugf("ImageWatcher: Rename %s", event.Name)
				iw.renamePath(event.Name)
			}
		}
	}
}

// Close cleanly shuts down the watcher.
func (iw *ImageWatcher) Close() {
	log.Info("ImageWatcher: gracefully shutting down")
	close(iw.imageCreated)
	iw.watcher.Close()
	iw.closableCloseAndWait()
	log.Info("ImageWatcher: shut down")
}

func (iw *ImageWatcher) createPath(path string) {
	// Directories and non-directories have to be treated differently.
	info, err := os.Stat(path)
	if err != nil {
		log.Warningf("Unable to stat %s: %s", path, err)
		return
	}
	if !info.IsDir() {
		iw.queueImage(path)
		return
	}

	walkFunc := func(walkPath string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			return nil
		}
		if err != nil {
			return err
		}

		log.Debugf("ImageWatcher: monitoring %s", walkPath)
		iw.watcher.Add(walkPath)

		return nil
	}

	// This recurses into subdirectories too.
	filepath.Walk(path, walkFunc)
}

func (iw *ImageWatcher) removePath(path string) {
	// I want to ignore non-directories, but they have been removed, so there is
	// no telling what it was.

	// From a quick test on Linux, we get events for each removed directory,
	// and not just the top-level directory that was removed. This means we
	// don't have to recurse ourselves.
	iw.watcher.Remove(path)
}

func (iw *ImageWatcher) renamePath(path string) {
	// A rename is called on the _old_ path, so we can't determine if it was
	// a directory or a file.
	iw.watcher.Remove(path)

	// If it was a file rename, remove it from our image map.
	// The new path will get a Create event anyway.
	iw.imageMapLock.Lock()
	defer iw.imageMapLock.Unlock()

	_, found := iw.imageMap[path]
	if found {
		delete(iw.imageMap, path)
	}
}

func (iw *ImageWatcher) queueImage(path string) {
	// Ignore non-files.
	info, err := os.Stat(path)
	if err != nil {
		log.Warningf("ImageWatcher: Unable to stat %s: %s", path, err)
		return
	}
	if info.IsDir() {
		return
	}

	// Ignore non-image files.
	ext := strings.ToLower(filepath.Ext(path))
	isImage := imageExtensions[ext]
	if !isImage {
		log.Debugf("ImageWatcher: Ignoring file %s, it is not an image.", path)
		return
	}

	// log.Debugf("ImageWatcher: Someone wrote to an image: %s", path)

	iw.imageMapLock.Lock()
	defer iw.imageMapLock.Unlock()

	var image *imageFile
	image, found := iw.imageMap[path]
	now := UtcNow()

	if !found {
		// this is a new image, construct a imageFile struct for it.
		image = &imageFile{path, *now}
		iw.imageMap[path] = image
	} else {
		// Seen this image before, update its lastWrite.
		image.lastWrite = *now
	}
}

// imageMapLoop periodically checks iw.imageMap to see if there are old enough entries
// to send to the ImageCreated channel.
func (iw *ImageWatcher) imageMapLoop() {
	iw.closableAdd(1)
	defer iw.closableDone()

	// Checking the image map a few times per 'fileAgeThreshold' should provide
	// enough precision for this purpose.
	timer := Timer("ImageWatcher-maploop", fileAgeThreshold/2, 0, &iw.closable)
	for range timer {
		iw.imageMapIteration()
	}

	log.Debugf("ImageWatcher-mapLoop: shutting down.")
}

func (iw *ImageWatcher) imageMapIteration() {
	iw.imageMapLock.Lock()
	defer iw.imageMapLock.Unlock()

	// Files touched on or before this timestamp are considered "written"
	old := UtcNow().Add(-fileAgeThreshold)

	// We can't remove keys while iterating over the map.
	reportPaths := []string{}

	for _, imageFile := range iw.imageMap {
		if imageFile.lastWrite.After(old) {
			// The file needs time to ripen and mature.
			continue
		}
		reportPaths = append(reportPaths, imageFile.path)
	}

	// Remove all files we'll report.
	for _, key := range reportPaths {
		delete(iw.imageMap, key)
	}

	// Send to the channel after unlocking the map. Otherwise a blocking
	// channel would also block the filesystem watch loop.
	for _, path := range reportPaths {
		log.Debugf("ImageWatcher: image file written: %s", path)
		iw.imageCreated <- path
	}
}
