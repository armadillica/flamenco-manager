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

// Package dynamicpool handles scalable pools of virtual machines for running Flamenco Workers.
package dynamicpool

import "context"

// PoolID is a unique identifier for a pool of workers.
type PoolID string

// PlatformStatus stores the last-known status of each pool for a single platform.
type PlatformStatus map[PoolID]PoolStatus

// PoolSize indicates the size (in machines/nodes) of a single pool.
type PoolSize struct {
	// The number of dedicated machines (i.e. not suddenly disappearing but more expensive).
	DedicatedNodes int `json:"dedicatedNodes"`
	// The number of low-priority machines (i.e. can disappear but cheaper).
	LowPriorityNodes int `json:"lowPriorityNodes"`
}

// PoolStatus reflects the current status of the pool
// Heavily influenced by https://docs.microsoft.com/en-us/rest/api/batchservice/pool/get#cloudpool
type PoolStatus struct {
	ID PoolID `json:"ID"`

	// State of the pool, either "active" or "deleting".
	State string `json:"state"`
	// AllocationState indicates whether the pool is "resizing", "steady", or "stopping".
	AllocationState string `json:"allocationState"`
	// ResizeError contains one or more error messages from the last resize operation.
	ResizeError string `json:"resizeError,omitempty"`

	CurrentSize PoolSize `json:"currentSize"`
	DesiredSize PoolSize `json:"desiredSize"`

	// VMSize indicates the size of the virtual machines in this pool.
	VMSize string `json:"vmSize,omitempty"`
}

// Platform represents a single account on some computing platform.
// It is assumed that each supported platform is used at most once (e.g. no multi-account support).
type Platform interface {
	// Name returns the name of the platform.
	Name() string

	// ListPoolIDs returns a list of pool IDs.
	ListPoolIDs(ctx context.Context) []PoolID

	// GetPool constructs a Pool manager for interacting with a particular pool.
	GetPool(poolID PoolID) (Pool, error)
}

// Pool manages a single pool; it can get info about the pool and resize the pool.
type Pool interface {
	// CurrentStatus returns the last-known pool status.
	CurrentStatus(ctx context.Context) (PoolStatus, error)

	// ScaleTo attempts to scale the pool to the indicated pool size.
	ScaleTo(ctx context.Context, poolSize PoolSize) error
}

// Copy returns a copy of the PlatformStatus.
func (ps PlatformStatus) Copy() PlatformStatus {
	copy := PlatformStatus{}
	for poolID, status := range ps {
		copy[poolID] = status
	}
	return copy
}
