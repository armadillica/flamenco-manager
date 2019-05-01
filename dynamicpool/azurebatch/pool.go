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

package azurebatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/armadillica/flamenco-manager/dynamicpool"
	"github.com/azure/azure-sdk-for-go/profiles/latest/batch/batch"
)

type pool struct {
	batchURL string
	poolID   dynamicpool.PoolID
}

// newPool constructs a Pool manager for interacting with a particular pool.
func newPool(batchURL string, poolID dynamicpool.PoolID) dynamicpool.Pool {
	return &pool{
		batchURL: batchURL,
		poolID:   poolID,
	}
}

// CurrentStatus returns the last-known pool status.
func (pool *pool) CurrentStatus(ctx context.Context) (dynamicpool.PoolStatus, error) {
	client, err := getPoolClient(pool.batchURL)
	if err != nil {
		return dynamicpool.PoolStatus{}, err
	}

	result, err := client.Get(ctx, string(pool.poolID), "", "", nil, nil, nil, nil, "", "", nil, nil)
	if err != nil {
		return dynamicpool.PoolStatus{}, err
	}

	resizeErrors := []string{}
	if result.ResizeErrors != nil {
		var description string
		for _, resizeErr := range *result.ResizeErrors {
			switch {
			case resizeErr.Code != nil && resizeErr.Message != nil:
				description = fmt.Sprintf("%s: %s", *resizeErr.Code, *resizeErr.Message)
			case resizeErr.Code != nil:
				description = *resizeErr.Code
			case resizeErr.Message != nil:
				description = *resizeErr.Message
			default:
				description = "-unknown error-"
			}
			resizeErrors = append(resizeErrors, description)
		}
	}

	status := dynamicpool.PoolStatus{
		ID:              pool.poolID,
		State:           string(result.State),
		AllocationState: string(result.AllocationState),
		ResizeError:     strings.Join(resizeErrors, "; "),
		CurrentSize: dynamicpool.PoolSize{
			DedicatedNodes:   derefInt(result.CurrentDedicatedNodes),
			LowPriorityNodes: derefInt(result.CurrentLowPriorityNodes),
		},
		DesiredSize: dynamicpool.PoolSize{
			DedicatedNodes:   derefInt(result.TargetDedicatedNodes),
			LowPriorityNodes: derefInt(result.TargetLowPriorityNodes),
		},
		VMSize: derefString(result.VMSize, ""),
	}

	return status, nil
}

// ScaleTo attempts to scale the pool to the indicated pool size.
func (pool *pool) ScaleTo(ctx context.Context, poolSize dynamicpool.PoolSize) error {
	client, err := getPoolClient(pool.batchURL)
	if err != nil {
		return err
	}

	dedNodes := int32(poolSize.DedicatedNodes)
	lowNodes := int32(poolSize.LowPriorityNodes)

	poolResizeParameter := batch.PoolResizeParameter{
		TargetDedicatedNodes:   &dedNodes,
		TargetLowPriorityNodes: &lowNodes,
	}
	_, err = client.Resize(ctx, string(pool.poolID), poolResizeParameter, nil, nil, nil, nil, "", "", nil, nil)
	if err != nil {
		return err
	}

	return nil
}
