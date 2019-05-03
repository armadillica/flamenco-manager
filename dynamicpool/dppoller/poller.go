/* (c) 2019, Blender Foundation
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

// Package dppoller contains a Dynamic Pool Poller that locally stores
// the status of dynamic pools and periodically refreshes.
package dppoller

import (
	"context"
	"sync"
	"time"

	"github.com/armadillica/flamenco-manager/dynamicpool"
	"github.com/armadillica/flamenco-manager/dynamicpool/azurebatch"
	"github.com/sirupsen/logrus"
)

// Poller uses the configured pool platforms to regularly poll for updated information about the pools.
type Poller struct {
	context       context.Context
	contextCancel context.CancelFunc
	wg            sync.WaitGroup
	mutex         sync.Mutex

	pollPeriod   time.Duration
	lastQuery    time.Time
	lastPoll     time.Time
	forceRefresh chan bool

	isActive     bool
	isRefreshing bool

	azure       *dynamicpool.Platform
	azureStatus dynamicpool.PlatformStatus
}

// NewPoller returns a poller for the given configuration.
// If the config is nil or has no platform-specific configuration
// an inactive poller is returned.
func NewPoller(config *Config) *Poller {
	ctx, cancel := context.WithCancel(context.Background())

	poller := Poller{
		context:       ctx,
		contextCancel: cancel,
		wg:            sync.WaitGroup{},

		pollPeriod:   250 * time.Millisecond, // the initial wait should be rather short.
		lastQuery:    time.Now(),
		forceRefresh: make(chan bool, 5),
	}

	if config == nil {
		return &poller
	}

	if config.Azure != nil {
		plat, err := azurebatch.NewPlatform(*config.Azure)
		if err != nil {
			logrus.WithError(err).Error("unable to create Azure platform")
		} else {
			poller.azure = &plat
			poller.isActive = true
		}
	}

	return &poller
}

// Go starts the poller in a goroutine. A poller can only be started once.
func (p *Poller) Go() {
	if !p.isActive {
		logrus.Debug("no dynamic pool platform configured, not going to poll")
		return
	}

	p.wg.Add(1)
	go func() {
		defer logrus.Debug("dynamic pool poller shutting down")
		defer p.setActive(false)
		defer p.wg.Done()

		for {
			select {
			case <-p.context.Done():
				return
			case <-p.forceRefresh:
				p.poll()
			case <-time.After(p.pollPeriod):
				p.poll()
			// Limit how long we wait between polls, so that a change in
			// pollPeriod can actually shorten the waiting time:
			case <-time.After(5 * time.Second):
				p.mutex.Lock()
				timeSinceLast := time.Since(p.lastPoll)
				p.mutex.Unlock()

				if timeSinceLast >= p.pollPeriod {
					p.poll()
				}
			}

			p.determinePollPeriod()
		}
	}()
}

// Close stops the poller and waits for its main goroutine to stop.
// A closed poller cannot be reused.
func (p *Poller) Close() {
	p.contextCancel()
	close(p.forceRefresh)
	p.wg.Wait()
}

// Done returns a channel that's closed when the poller is closed.
func (p *Poller) Done() <-chan struct{} {
	return p.context.Done()
}

// AzureStatus returns the status of the Azure Batch pools.
func (p *Poller) AzureStatus() dynamicpool.PlatformStatus {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.lastQuery = time.Now()
	return p.azureStatus.Copy()
}

// IsRefreshing returns true iff the poller is currently communicating with the remote platforms.
func (p *Poller) IsRefreshing() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.isRefreshing
}

// RefreshNow forces the poller to refresh.
func (p *Poller) RefreshNow() {
	logrus.Debug("refreshing dynamic pools now")
	p.forceRefresh <- true
}

// IsActive returns true if the poller is active (e.g. has configuration for at least one platform).
func (p *Poller) IsActive() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return p.isActive
}

func (p *Poller) setActive(isActive bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.isActive = isActive
}

func (p *Poller) poll() {
	p.mutex.Lock()
	p.isRefreshing = true
	p.mutex.Unlock()

	newStatus := p.refreshPlatform(*p.azure)

	p.mutex.Lock()
	p.azureStatus = newStatus
	p.isRefreshing = false
	p.lastPoll = time.Now()
	p.mutex.Unlock()
}

func (p *Poller) refreshPlatform(platform dynamicpool.Platform) dynamicpool.PlatformStatus {
	logger := logrus.WithField("platform", platform.Name())
	logger.Debug("refreshing dynamic pool information")

	poolIDs := platform.ListPoolIDs(p.context)

	newStatus := dynamicpool.PlatformStatus{}
	mutex := sync.Mutex{}
	wg := sync.WaitGroup{}

	// Get the status of each pool in parallel.
	for _, poolID := range poolIDs {
		wg.Add(1)

		go func(poolID dynamicpool.PoolID) {
			defer wg.Done()

			logger := logger.WithField("poolID", poolID)
			logger.Debug("fetching dynamic pool status")

			pool, err := platform.GetPool(poolID)
			if err != nil {
				logger.WithError(err).Error("unable to get pool manager")
				return
			}

			poolStatus, err := pool.CurrentStatus(p.context)
			if err != nil {
				logger.WithError(err).Error("unable to get pool status")
				return
			}

			mutex.Lock()
			defer mutex.Unlock()
			newStatus[poolID] = poolStatus
		}(poolID)
	}
	wg.Wait()

	return newStatus
}

func (p *Poller) determinePollPeriod() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	timeSinceQuery := time.Since(p.lastQuery)
	switch {
	case timeSinceQuery < 1*time.Minute:
		p.pollPeriod = 15 * time.Second
	case timeSinceQuery < 5*time.Minute:
		p.pollPeriod = 1 * time.Minute
	case timeSinceQuery < 15*time.Minute:
		p.pollPeriod = 5 * time.Minute
	default:
		p.pollPeriod = 10 * time.Minute
	}

	logrus.WithFields(logrus.Fields{
		"timeSinceQuery": timeSinceQuery,
		"timeSincePoll":  time.Since(p.lastPoll),
		"pollPeriod":     p.pollPeriod,
	}).Debug("determined new dynamic pool poll period")
}
