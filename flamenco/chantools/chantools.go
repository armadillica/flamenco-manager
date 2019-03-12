// Package chantools was obtained from https://github.com/theepicsnail/goChanTools
// and subsequently altered for our needs.
package chantools

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

import "sync"

// OneToManyChan can broadcase a single channel into many channels.
type OneToManyChan struct {
	lock     *sync.Mutex
	inChan   <-chan string
	outChans map[chan<- string]bool
}

// NewOneToManyChan constructs a one-to-many broadcaster
func NewOneToManyChan(srcChan <-chan string) *OneToManyChan {
	o := &OneToManyChan{
		new(sync.Mutex),
		srcChan,
		make(map[chan<- string]bool),
	}
	o.start()
	return o
}

func (o *OneToManyChan) start() {
	go func() {
		for message := range o.inChan {
			o.broadcast(message)
		}

		o.lock.Lock()
		defer o.lock.Unlock()

		for ch := range o.outChans {
			close(ch)
		}
	}()
}

func (o *OneToManyChan) broadcast(message string) {
	o.lock.Lock()
	defer o.lock.Unlock()

	for ch := range o.outChans {
		go func(message string, ch chan<- string) {
			ch <- message
		}(message, ch)
	}
}

// AddOutputChan adds the given channel to the list of channels to broadcast to.
func (o *OneToManyChan) AddOutputChan(ch chan<- string) {
	o.lock.Lock()
	defer o.lock.Unlock()

	o.outChans[ch] = true
}

// RemoveOutputChan removes the given channel from the list of channels to broadcast to.
func (o *OneToManyChan) RemoveOutputChan(ch chan<- string) {
	o.lock.Lock()
	defer o.lock.Unlock()

	delete(o.outChans, ch)
}
