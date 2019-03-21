/* (c) 2019, Blender Foundation - Sybren A. Stüvel
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

package checkout

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/armadillica/flamenco-manager/shaman/config"
	"github.com/armadillica/flamenco-manager/shaman/filestore"
)

// Manager creates checkouts and provides info about missing files.
type Manager struct {
	checkoutBasePath string
	fileStore        filestore.Storage

	wg sync.WaitGroup
}

// ResolvedCheckoutInfo contains the result of validating the Checkout ID and parsing it into a final path.
type ResolvedCheckoutInfo struct {
	// The absolute path on our filesystem.
	absolutePath string
	// The path relative to the Manager.checkoutBasePath. This is what is
	// sent back to the client.
	RelativePath string
}

// Errors returned by the Checkout Manager.
var (
	ErrCheckoutAlreadyExists = errors.New("A checkout with this ID already exists")
	ErrInvalidCheckoutID     = errors.New("The Checkout ID is invalid")
)

// NewManager creates and returns a new Checkout Manager.
func NewManager(conf config.Config, fileStore filestore.Storage) *Manager {
	logger := packageLogger.WithField("checkoutDir", conf.CheckoutPath)
	logger.Info("opening checkout directory")

	err := os.MkdirAll(conf.CheckoutPath, 0777)
	if err != nil {
		logger.WithError(err).Fatal("unable to create checkout directory")
	}

	return &Manager{conf.CheckoutPath, fileStore, sync.WaitGroup{}}
}

// Close waits for still-running touch() calls to finish, then returns.
func (m *Manager) Close() {
	packageLogger.Info("shutting down Checkout manager")
	m.wg.Wait()
}

func (m *Manager) pathForCheckoutID(checkoutID string) (ResolvedCheckoutInfo, error) {
	if !isValidCheckoutID(checkoutID) {
		return ResolvedCheckoutInfo{}, ErrInvalidCheckoutID
	}

	// When changing the number of path components the checkout ID is turned into,
	// be sure to also update the EraseCheckout() function for this.

	// We're expecting ObjectIDs as checkoutIDs, which means most variation
	// is in the last characters.
	lastBitIndex := len(checkoutID) - 2
	relativePath := path.Join(checkoutID[lastBitIndex:], checkoutID)

	return ResolvedCheckoutInfo{
		absolutePath: path.Join(m.checkoutBasePath, relativePath),
		RelativePath: relativePath,
	}, nil
}

// PrepareCheckout creates the root directory for a specific checkout.
// Returns the path relative to the checkout root directory.
func (m *Manager) PrepareCheckout(checkoutID string) (ResolvedCheckoutInfo, error) {
	checkoutPaths, err := m.pathForCheckoutID(checkoutID)
	if err != nil {
		return ResolvedCheckoutInfo{}, err
	}

	logger := logrus.WithFields(logrus.Fields{
		"checkoutPath": checkoutPaths.absolutePath,
		"checkoutID":   checkoutID,
	})

	if stat, err := os.Stat(checkoutPaths.absolutePath); !os.IsNotExist(err) {
		if err == nil {
			if stat.IsDir() {
				logger.Debug("checkout path exists")
			} else {
				logger.Error("checkout path exists but is not a directory")
			}
			// No error stat'ing this path, indicating it's an existing checkout.
			return ResolvedCheckoutInfo{}, ErrCheckoutAlreadyExists
		}
		// If it's any other error, it's really a problem on our side.
		logger.WithError(err).Error("unable to stat checkout directory")
		return ResolvedCheckoutInfo{}, err
	}

	if err := os.MkdirAll(checkoutPaths.absolutePath, 0777); err != nil {
		logger.WithError(err).Fatal("unable to create checkout directory")
	}

	logger.WithField("relPath", checkoutPaths.RelativePath).Info("created checkout directory")
	return checkoutPaths, nil
}

// EraseCheckout removes the checkout directory structure identified by the ID.
func (m *Manager) EraseCheckout(checkoutID string) error {
	checkoutPaths, err := m.pathForCheckoutID(checkoutID)
	if err != nil {
		return err
	}

	logger := logrus.WithFields(logrus.Fields{
		"checkoutPath": checkoutPaths.absolutePath,
		"checkoutID":   checkoutID,
	})
	if err := os.RemoveAll(checkoutPaths.absolutePath); err != nil {
		logger.WithError(err).Error("unable to remove checkout directory")
		return err
	}

	// Try to remove the parent path as well, to not keep the dangling two-letter dirs.
	// Failure is fine, though, because there is no guarantee it's empty anyway.
	os.Remove(path.Dir(checkoutPaths.absolutePath))
	logger.Info("removed checkout directory")
	return nil
}

// SymlinkToCheckout creates a symlink at symlinkPath to blobPath.
// It does *not* do any validation of the validity of the paths!
func (m *Manager) SymlinkToCheckout(blobPath, checkoutPath, symlinkRelativePath string) error {
	symlinkPath := path.Join(checkoutPath, symlinkRelativePath)
	logger := logrus.WithFields(logrus.Fields{
		"blobPath":    blobPath,
		"symlinkPath": symlinkPath,
	})

	blobPath, err := filepath.Abs(blobPath)
	if err != nil {
		logger.WithError(err).Error("unable to make blobPath absolute")
		return err
	}

	logger.Debug("creating symlink")

	// This is expected to fail sometimes, because we don't create parent directories yet.
	// We only create those when we get a failure from symlinking.
	err = os.Symlink(blobPath, symlinkPath)
	if err == nil {
		return err
	}
	if !os.IsNotExist(err) {
		logger.WithError(err).Error("unable to create symlink")
		return err
	}

	logger.Debug("creating parent directory")

	dir := path.Dir(symlinkPath)
	if err := os.MkdirAll(dir, 0777); err != nil {
		logger.WithError(err).Error("unable to create parent directory")
		return err
	}

	if err := os.Symlink(blobPath, symlinkPath); err != nil {
		logger.WithError(err).Error("unable to create symlink, after creating parent directory")
		return err
	}

	// Change the modification time of the blob to mark it as 'referenced' just now.
	m.wg.Add(1)
	go func() {
		touch(blobPath)
		m.wg.Done()
	}()

	return nil
}

// touch changes the modification time of the blob to mark it as 'referenced' just now.
func touch(blobPath string) error {
	if blobPath == "" {
		return os.ErrInvalid
	}
	now := time.Now()

	logger := logrus.WithField("file", blobPath)
	logger.Debug("touching")

	err := os.Chtimes(blobPath, now, now)
	logLevel := logrus.DebugLevel
	if err != nil {
		logger = logger.WithError(err)
		logLevel = logrus.WarnLevel
	}

	duration := time.Now().Sub(now)
	logger = logger.WithField("duration", duration)
	if duration < 1*time.Second {
		logger.Log(logLevel, "done touching")
	} else {
		logger.Log(logLevel, "done touching but took a long time")
	}

	return err
}
