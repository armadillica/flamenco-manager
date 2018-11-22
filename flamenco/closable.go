package flamenco

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

// closable offers a way to cleanly shut down a running goroutine.
type closable struct {
	doneChan     chan struct{}
	doneWg       *sync.WaitGroup
	isClosed     bool
	closingMutex *sync.Mutex
}

// makeClosable constructs a new closable struct
func makeClosable() closable {
	return closable{
		make(chan struct{}),
		new(sync.WaitGroup),
		false,
		new(sync.Mutex),
	}
}

// closableAdd(delta) should be combined with 'delta' calls to closableDone()
func (closable *closable) closableAdd(delta int) {
	closable.closingMutex.Lock()
	defer closable.closingMutex.Unlock()

	log.Debugf("Closable: doneWg.Add(%d) ok", delta)
	closable.doneWg.Add(delta)
}

// closableDone marks one "thing" as "done"
func (closable *closable) closableDone() {
	closable.closingMutex.Lock()
	defer closable.closingMutex.Unlock()

	log.Debug("Closable: doneWg.Done() ok")
	closable.doneWg.Done()
}

// _closableMaybeClose only closes the channel if it wasn't closed yet.
func (closable *closable) _closableMaybeClose() {
	closable.closableClosingLock()
	defer closable.closableClosingUnlock()

	if !closable.isClosed {
		closable.isClosed = true
		close(closable.doneChan)
	}
}

// closableCloseAndWait marks the goroutine as "done",
// and waits for all things added with closableAdd() to be "done" too.
func (closable *closable) closableCloseAndWait() {
	closable._closableMaybeClose()
	log.Debug("Closable: waiting for shutdown to finish.")
	closable.doneWg.Wait()
}

// closableCloseNoWait marks the goroutine as "done",
// but does not waits for all things added with closableAdd() to be "done" too.
func (closable *closable) closableCloseNotWait() {
	closable._closableMaybeClose()
	log.Debug("Closable: marking as closed but NOT waiting shutdown to finish.")
}

func (closable *closable) closableClosingLock() {
	closable.closingMutex.Lock()
}

func (closable *closable) closableClosingUnlock() {
	closable.closingMutex.Unlock()
}
