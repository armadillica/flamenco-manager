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

package fileserver

import (
	"context"
	"sync"

	"github.com/armadillica/flamenco-manager/shaman/filestore"
)

type receiverChannel chan struct{}

// FileServer deals with receiving and serving of uploaded files.
type FileServer struct {
	fileStore filestore.Storage

	receiverMutex    sync.Mutex
	receiverChannels map[string]receiverChannel

	ctx       context.Context
	ctxCancel context.CancelFunc
	wg        sync.WaitGroup
}

// New creates a new File Server and starts a monitoring goroutine.
func New(fileStore filestore.Storage) *FileServer {
	ctx, ctxCancel := context.WithCancel(context.Background())

	fs := &FileServer{
		fileStore,
		sync.Mutex{},
		map[string]receiverChannel{},
		ctx,
		ctxCancel,
		sync.WaitGroup{},
	}

	return fs
}

// Go starts goroutines for background operations.
// After Go() has been called, use Close() to stop those goroutines.
func (fs *FileServer) Go() {
	fs.wg.Add(1)
	go fs.receiveListenerPeriodicCheck()
}

// Close stops any goroutines started by this server, and waits for them to close.
func (fs *FileServer) Close() {
	fs.ctxCancel()
	fs.wg.Wait()
}
