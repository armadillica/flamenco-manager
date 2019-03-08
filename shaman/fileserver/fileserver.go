package fileserver

/* ***** BEGIN GPL LICENSE BLOCK *****
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License
 * as published by the Free Software Foundation; either version 2
 * of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software Foundation,
 * Inc., 59 Temple Place - Suite 330, Boston, MA  02111-1307, USA.
 *
 * ***** END GPL LICENCE BLOCK *****
 *
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
 */

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
