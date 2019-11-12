/* (c) 2019, Blender Foundation - Sybren A. Stüvel
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
 */

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

	closable.doneWg.Add(delta)
}

// closableDone marks one "thing" as "done"
func (closable *closable) closableDone() {
	closable.closingMutex.Lock()
	defer closable.closingMutex.Unlock()

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
