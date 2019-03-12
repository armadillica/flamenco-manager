package fileserver

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
	"time"
)

// Returns a channel that is open while the given file is being received.
// The first to fully receive the file should close the channel, indicating to others
// that their upload can be aborted.
func (fs *FileServer) receiveListenerFor(checksum string, filesize int64) chan struct{} {
	fs.receiverMutex.Lock()
	defer fs.receiverMutex.Unlock()

	key := fmt.Sprintf("%s/%d", checksum, filesize)
	channel := fs.receiverChannels[key]
	if channel != nil {
		return channel
	}

	channel = make(receiverChannel)
	fs.receiverChannels[key] = channel

	go func() {
		// Wait until the channel closes.
		select {
		case <-channel:
		}

		fs.receiverMutex.Lock()
		defer fs.receiverMutex.Unlock()
		delete(fs.receiverChannels, key)
	}()

	return channel
}

func (fs *FileServer) receiveListenerPeriodicCheck() {
	defer fs.wg.Done()
	lastReportedChans := -1

	doCheck := func() {
		fs.receiverMutex.Lock()
		defer fs.receiverMutex.Unlock()

		numChans := len(fs.receiverChannels)
		if numChans == 0 {
			if lastReportedChans != 0 {
				packageLogger.Debug("no receive listener channels")
			}
		} else {
			packageLogger.WithField("num_receiver_channels", numChans).Debug("receiving files")
		}
		lastReportedChans = numChans
	}

	for {
		select {
		case <-fs.ctx.Done():
			packageLogger.Debug("stopping receive listener periodic check")
			return
		case <-time.After(1 * time.Minute):
			doCheck()
		}
	}
}
