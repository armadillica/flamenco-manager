package jwtauth

/* ***** BEGIN MIT LICENSE BLOCK *****
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
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
 * ***** END MIT LICENCE BLOCK *****
 */

import (
	"crypto/ecdsa"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// KeyStore contains a private and public keys.
type KeyStore struct {
	keyDirectory         string
	jwtPublicKeysURL     string
	mayLoadTestKeys      bool
	downloadKeysInterval time.Duration

	// Private key used for generating JWTs. May be nil when no private key is loaded.
	MyPrivateKey *ecdsa.PrivateKey

	// Any key in this array is trusted as authoritative for received JWTs.
	TrustedPublicKeys []*ecdsa.PublicKey

	shutdownChan chan interface{}
	wg           sync.WaitGroup
}

var globalKeyStore *KeyStore
var globalKeyStoreMutex sync.Mutex

// Time to wait after application startup before downloading the JWT public keys.
var downloadKeysInitialWait = 2 * time.Second

// newKeyStore loads all keys in the given directory.
func newKeyStore(config Config, keyDirectory string, mayLoadTestKeys bool) *KeyStore {
	ks := &KeyStore{
		keyDirectory:         keyDirectory,
		jwtPublicKeysURL:     config.PublicKeysURL,
		mayLoadTestKeys:      mayLoadTestKeys,
		downloadKeysInterval: config.DownloadKeysInterval,

		shutdownChan: make(chan interface{}),
		wg:           sync.WaitGroup{},
	}

	ks.load()

	return ks
}

// LoadKeyStore loads all keys from a directory and stores them in the global key store.
func LoadKeyStore(config Config, keyDirectory string) {
	loadKeyStore(config, keyDirectory, false)
}

func loadKeyStore(config Config, keyDirectory string, mayLoadTestKeys bool) {
	keyStore := newKeyStore(config, keyDirectory, mayLoadTestKeys)
	swapGlobalKeyStore(keyStore)
}

// ReloadKeyStore reloads the global keystore from disk.
// The keystore is locked while the reload happens.
func ReloadKeyStore() {
	globalKeyStoreMutex.Lock()
	defer globalKeyStoreMutex.Unlock()

	if globalKeyStore != nil {
		globalKeyStore.load()
	}
}

// GetKeyStore returns a shallow copy of the global KeyStore.
// This allows the global keystore to be modified while it is in use.
// The returned KeyStore should be used immediately, no references/copies kept.
func GetKeyStore() *KeyStore {
	// Prevent modification while we're copying the keystore.
	globalKeyStoreMutex.Lock()
	defer globalKeyStoreMutex.Unlock()

	if globalKeyStore == nil {
		globalKeyStore = &KeyStore{}
	}

	// Copy all fields, except the WaitGroup and the 'shutdownChan' channel,
	// as those should never be copied.
	return &KeyStore{
		globalKeyStore.keyDirectory,
		globalKeyStore.jwtPublicKeysURL,
		globalKeyStore.mayLoadTestKeys,
		globalKeyStore.downloadKeysInterval,
		globalKeyStore.MyPrivateKey,
		globalKeyStore.TrustedPublicKeys,
		nil,
		sync.WaitGroup{},
	}
}

// Atomically swaps 'globalKeyStore' and 'ks'.
func swapGlobalKeyStore(ks *KeyStore) *KeyStore {
	globalKeyStoreMutex.Lock()
	defer globalKeyStoreMutex.Unlock()

	ks, globalKeyStore = globalKeyStore, ks
	return ks
}

// GoDownloadLoop starts the background download loop for new keys.
// Use CloseKeyStore() after calling this.
func GoDownloadLoop() {
	ks := globalKeyStore

	if ks.jwtPublicKeysURL != "" {
		// No need to start a download loop when we cannot download.
		go ks.Go()
	} else {
		logger := packageLogger.WithField("jwtKeyDirectory", ks.keyDirectory)
		if len(ks.TrustedPublicKeys) == 0 {
			logger.Warning("No publicKeysURL setting configured and no JWT public keys found, authentication will not work.")
		} else {
			logger = logger.WithField("publicKeyCount", len(ks.TrustedPublicKeys))
			logger.Warning("No publicKeysURL setting configured, Flamenco Manager will not download fresh keys.")
		}
	}
}

// CloseKeyStore stops any background download process.
func CloseKeyStore() {
	ks := swapGlobalKeyStore(nil)
	if ks != nil {
		ks.Close()
	}
}

// Go starts the public key download loop in a background goroutine.
func (ks *KeyStore) Go() {
	ks.wg.Add(1)

	go func() {
		defer ks.wg.Done()

		// Refresh the JWT keys shortly after starting up.
		select {
		case <-ks.shutdownChan:
			return
		case <-time.After(downloadKeysInitialWait):
			ks.refreshPublicKeys()
		}

		if ks.downloadKeysInterval == 0 {
			packageLogger.Info("not starting JWT public key download loop, interval is 0")
			return
		}

		packageLogger.WithField("interval", ks.downloadKeysInterval).Info("starting JWT public key download loop")
		defer packageLogger.Info("shutting down JWT key download loop")

		for {
			select {
			case <-ks.shutdownChan:
				return
			case <-time.After(ks.downloadKeysInterval):
				ks.refreshPublicKeys()
			}
		}
	}()
}

// Close stops the key download loop.
func (ks *KeyStore) Close() {
	packageLogger.Debug("closing keystore")
	close(ks.shutdownChan)
	ks.wg.Wait()
}

func (ks *KeyStore) load() {
	privateFilename := path.Join(ks.keyDirectory, "es256-private.pem")
	if ks.mayLoadTestKeys || !strings.Contains(privateFilename, "test") {
		ks.MyPrivateKey = ReadPrivateKey(privateFilename)
	}
	ks.TrustedPublicKeys = ReadAllPublicKeys(ks.keyDirectory, ks.mayLoadTestKeys)

	ks.selfTest()
}

func (ks *KeyStore) refreshPublicKeys() {
	downloadedNewKeys := ks.downloadPublicKeys()
	if downloadedNewKeys {
		packageLogger.Debug("downloaded new keys to disk, going to load them to memory")
		ks.load()
	}
}

// selfTest logs an error when none of our public keys can validate our self-signed tokens.
func (ks *KeyStore) selfTest() {
	// We can only self-test when we have a private key.
	if ks.MyPrivateKey == nil {
		return
	}

	j := NewJWT(true)
	tokenString, err := j.generateToken(ks)
	if err != nil {
		packageLogger.WithError(err).Error("JWT self test: unable to generate token")
	}

	logger := packageLogger.WithField("jwtSelfTest", true)
	if _, err := j.validateWithKeystore(tokenString, ks, logger); err != nil {
		packageLogger.WithError(err).Error("JWT self test: unable to validate self-generated token")
	}
}

// downloadPublicKeys downloads public keys.
// Returns true when new keys were downloaded.
func (ks *KeyStore) downloadPublicKeys() (downloadedNew bool) {
	logger := packageLogger.WithFields(logrus.Fields{
		"url":          ks.jwtPublicKeysURL,
		"keyDirectory": ks.keyDirectory,
	})
	logger.Debug("downloading JWT public keys")

	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	req, err := http.NewRequest("GET", ks.jwtPublicKeysURL, nil)
	if err != nil {
		logger.WithError(err).Error("unable to create request to download JWT public keys")
		return false
	}

	// If we downloaded the keys before, include a 'last-modified' header.
	lastModFilename := path.Join(ks.keyDirectory, "downloaded-public.last-modified")
	keyFilename := path.Join(ks.keyDirectory, "downloaded-public.pem")

	keyStat, statErr := os.Stat(keyFilename)
	// At less than 90 bytes, the public key file is not usable.
	if statErr == nil && keyStat.Size() > 90 {
		if lastMod, err := ioutil.ReadFile(lastModFilename); err == nil && len(lastMod) > 0 {
			req.Header.Set("If-Modified-Since", string(lastMod))
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.WithError(err).Error("unable to download JWT public keys")
		return false
	}
	logger = logger.WithField("http_status", resp.StatusCode)
	if resp.StatusCode == http.StatusNotModified {
		logger.Info("JWT public keys were not modified")
		return false
	}
	if resp.StatusCode >= 300 {
		logger.Error("error status from JWT key server")
		return false
	}

	tempFilename := keyFilename + "~"
	pubKeys, err := os.Create(tempFilename)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"filename":      tempFilename,
			logrus.ErrorKey: err,
		}).Error("unable to create JWT key file")
		return false
	}

	if _, err := io.Copy(pubKeys, resp.Body); err != nil {
		logger.WithFields(logrus.Fields{
			"filename":      tempFilename,
			logrus.ErrorKey: err,
		}).Error("unable to download JWT key file")
		return false
	}

	if err := os.Rename(tempFilename, keyFilename); err != nil {
		logger.WithFields(logrus.Fields{
			"renameFrom":    tempFilename,
			"renameTo":      keyFilename,
			logrus.ErrorKey: err,
		}).Error("unable to rename downloaded JWT key file")
		return false
	}

	// Store the last-modified header so that we can check for modifications next time.
	os.Remove(lastModFilename)
	if err := ioutil.WriteFile(lastModFilename, []byte(resp.Header.Get("Last-Modified")), 0644); err != nil {
		logger.WithFields(logrus.Fields{
			"lastModFilename": lastModFilename,
			logrus.ErrorKey:   err,
		}).Warning("unable to save last modification time of public JWT keys")
		return true
	}

	logger.WithField("filename", keyFilename).Info("JWT public keys downloaded")
	return true
}
