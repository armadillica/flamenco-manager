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

import "fmt"

// AZConfig contains the Azure-specific configuration we need to manage Batch pools.
// Note that credentials should be saved to `azure_credentials.json`
type AZConfig struct {
	// Physical location of the resource group, such as 'westeurope' or 'eastus'.
	Location string ` yaml:"location,omitempty"`

	// Name of the Azure Batch account that contains the Flamenco Worker VM pool.
	BatchAccountName string `yaml:"batch_account_name,omitempty"`
}

// BatchURL returns the URL we need for interacting with the Azure Batch account.
func (azc AZConfig) BatchURL() string {
	return fmt.Sprintf("https://%s.%s.batch.azure.com", azc.BatchAccountName, azc.Location)
}
