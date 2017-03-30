// Package chantools was obtained from https://github.com/theepicsnail/goChanTools
// and subsequently altered for our needs.
package chantools

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
			o.lock.Lock()
			defer o.lock.Unlock()

			for ch := range o.outChans {
				go func(message string, ch chan<- string) {
					ch <- message
				}(message, ch)
			}
		}

		o.lock.Lock()
		defer o.lock.Unlock()

		for ch := range o.outChans {
			close(ch)
		}
	}()
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
