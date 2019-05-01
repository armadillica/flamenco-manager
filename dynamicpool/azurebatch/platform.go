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
	"os"
	"path/filepath"

	"github.com/Azure/azure-sdk-for-go/services/batch/2018-12-01.8.0/batch"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/armadillica/flamenco-manager/dynamicpool"
	"github.com/sirupsen/logrus"
)

const azureCredentialsFile = "azure_credentials.json"

type platform struct {
	config AZConfig
}

// NewPlatform returns a Platform for managing Azure Batch pools.
func NewPlatform(config AZConfig) (dynamicpool.Platform, error) {
	// Find the Azure credentials file.
	fileloc := os.Getenv("AZURE_AUTH_LOCATION")
	if fileloc == "" {
		var err error
		if fileloc, err = filepath.Abs(azureCredentialsFile); err != nil {
			logrus.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"relativePath":  azureCredentialsFile,
			}).Error("unable to create absolute path")
			return nil, err
		}
		if err = os.Setenv("AZURE_AUTH_LOCATION", fileloc); err != nil {
			logrus.WithError(err).Error("unable to set AZURE_AUTH_LOCATION environment variable")
			return nil, err
		}
	}

	// Check the file location to see whether we can open the file.
	logger := logrus.WithField("filename", fileloc)
	file, err := os.Open(fileloc)
	if err != nil {
		return nil, err
	}
	file.Close()
	logger.Debug("checked Azure credentials file")

	return &platform{config: config}, nil
}

func (ap *platform) Name() string {
	return "Azure"
}

// ListPoolIDs returns a list of pool IDs.
func (ap *platform) ListPoolIDs(ctx context.Context) (poolIDs []dynamicpool.PoolID) {
	poolIDs = []dynamicpool.PoolID{}

	poolClient, err := getPoolClient(ap.config.BatchURL())
	if err != nil {
		logrus.WithError(err).Error("unable to construct Azure Batch pool client")
		return
	}

	resultPage, err := poolClient.List(ctx, "", "", "", nil, nil, nil, nil, nil)
	if err != nil {
		logrus.WithError(err).Error("unable to list existing pools")
		return
	}

	for resultPage.NotDone() {
		for _, foundPool := range resultPage.Values() {
			logrus.WithField("poolID", *foundPool.ID).Debug("found existing Azure Batch pool")
			poolID := dynamicpool.PoolID(*foundPool.ID)
			poolIDs = append(poolIDs, poolID)
		}
		err := resultPage.NextWithContext(ctx)
		if err != nil {
			logrus.WithError(err).Error("unable to get next page of pools")
			return
		}
	}
	logrus.WithField("poolCount", len(poolIDs)).Debug("done listing Azure Batch pools")
	return
}

// GetPool constructs a pool manager for interacting with a particular pool.
func (ap *platform) GetPool(poolID dynamicpool.PoolID) (dynamicpool.Pool, error) {
	return newPool(ap.config.BatchURL(), poolID), nil
}

func getPoolClient(batchURL string) (batch.PoolClient, error) {
	authorizer, err := auth.NewAuthorizerFromFileWithResource(azure.PublicCloud.BatchManagementEndpoint)
	if err != nil {
		return batch.PoolClient{}, err
	}

	poolClient := batch.NewPoolClient(batchURL)
	poolClient.Authorizer = authorizer
	// poolClient.RequestInspector = LogRequest()
	// poolClient.ResponseInspector = LogResponse()

	return poolClient, nil
}
