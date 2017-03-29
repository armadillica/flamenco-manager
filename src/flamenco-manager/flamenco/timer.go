package flamenco

import (
	"time"

	log "github.com/Sirupsen/logrus"
)

// Timers and killable sleeps are checked with this period.
const timerCheck = 200 * time.Millisecond

// Timer is a generic timer for periodic signals.
func Timer(name string, sleepDuration, initialDelay time.Duration, closable *closable) <-chan struct{} {
	timerChan := make(chan struct{}, 1) // don't let the timer block

	go func() {
		closable.closableAdd(1)
		defer closable.closableDone()
		defer close(timerChan)

		nextPingAt := time.Now().Add(initialDelay)

		for {
			select {
			case <-closable.doneChan:
				log.Infof("Timer '%s' goroutine shutting down.", name)
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

// TimeoutAfter sends a 'true' to the channel after the given timeout.
//
// Send a 'false' to the channel yourself if you want to notify the receiver that
// a timeout didn't happen.
//
// The channel is buffered with size 2, so both your 'false' and this routine's 'true'
// write won't block.
func TimeoutAfter(duration time.Duration) chan bool {
	timeout := make(chan bool, 2)

	go func() {
		time.Sleep(duration)
		defer func() {
			// Recover from a panic. This panic can happen when the caller closed the
			// channel while we were sleeping.
			recover()
		}()
		timeout <- true
	}()

	return timeout
}
