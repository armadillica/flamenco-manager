package flamenco

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
