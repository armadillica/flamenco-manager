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
