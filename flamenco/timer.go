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
	"time"

	log "github.com/sirupsen/logrus"
)

// Timers and killable sleeps are checked with this period.
const timerCheck = 200 * time.Millisecond

// Timer is a generic timer for periodic signals.
func Timer(name string, sleepDuration, initialDelay time.Duration, closable *closable) <-chan struct{} {
	timerChan := make(chan struct{}, 1) // don't let the timer block
	closable.closableAdd(1)

	go func() {
		defer closable.closableDone()
		defer close(timerChan)

		nextPingAt := time.Now().Add(initialDelay)

		for {
			select {
			case <-closable.doneChan:
				log.WithField("timer_name", name).Debug("timer shutting down")
				return
			default:
				// Only sleep a little bit, so that we can check 'done' quite often.
				// log.Debugf("Timer '%s' sleeping a bit.", name)
				time.Sleep(timerCheck)
			}

			now := time.Now()
			if nextPingAt.Before(now) {
				// Timeout occurred
				nextPingAt = now.Add(sleepDuration)
				timerChan <- struct{}{}
			}
		}
	}()

	return timerChan
}

// UtcNow returns the current time & date in UTC.
func UtcNow() *time.Time {
	now := time.Now().UTC()
	return &now
}
