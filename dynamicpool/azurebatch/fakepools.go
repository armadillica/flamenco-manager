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

import "github.com/armadillica/flamenco-manager/dynamicpool"

var fakePools = map[dynamicpool.PoolID]dynamicpool.PoolStatus{
	"fakeempty": {
		ID:              "fakeempty",
		State:           "active",
		AllocationState: "steady",
		ResizeError:     "",
		CurrentSize:     dynamicpool.PoolSize{DedicatedNodes: 0, LowPriorityNodes: 0},
		DesiredSize:     dynamicpool.PoolSize{DedicatedNodes: 0, LowPriorityNodes: 0},
		VMSize:          "standard_f16s",
	},
	"fakesteady": {
		ID:              "fakesteady",
		State:           "active",
		AllocationState: "steady",
		ResizeError:     "",
		CurrentSize:     dynamicpool.PoolSize{DedicatedNodes: 3, LowPriorityNodes: 40},
		DesiredSize:     dynamicpool.PoolSize{DedicatedNodes: 3, LowPriorityNodes: 40},
		VMSize:          "standard_f16s",
	},
	"fakeresizing": {
		ID:              "fakeresizing",
		State:           "active",
		AllocationState: "resizing",
		ResizeError:     "",
		CurrentSize:     dynamicpool.PoolSize{DedicatedNodes: 3, LowPriorityNodes: 40},
		DesiredSize:     dynamicpool.PoolSize{DedicatedNodes: 0, LowPriorityNodes: 200},
		VMSize:          "standard_f16s",
	},
	"fakeerror": {
		ID:              "fakeerror",
		State:           "active",
		AllocationState: "steady",
		ResizeError:     "You are asking more than what you paid for",
		CurrentSize:     dynamicpool.PoolSize{DedicatedNodes: 3, LowPriorityNodes: 40},
		DesiredSize:     dynamicpool.PoolSize{DedicatedNodes: 0, LowPriorityNodes: 200},
		VMSize:          "standard_f16s",
	},
	"fakedeleting": {
		ID:              "fakedeleting",
		State:           "deleting",
		AllocationState: "stopping",
		ResizeError:     "",
		CurrentSize:     dynamicpool.PoolSize{DedicatedNodes: 3, LowPriorityNodes: 40},
		DesiredSize:     dynamicpool.PoolSize{DedicatedNodes: 0, LowPriorityNodes: 0},
		VMSize:          "standard_f16s",
	},
}
