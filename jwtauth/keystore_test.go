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
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	httpmock "gopkg.in/jarcoal/httpmock.v1"
)

func TestLoadKeyStoreNonExistant(t *testing.T) {
	config, cleanup := CreateTestConfig()
	defer cleanup()

	loadKeyStore(config, "./non-existant-dir", true)
	defer CloseKeyStore()

	assert.NotNil(t, globalKeyStore)
	assert.Nil(t, globalKeyStore.MyPrivateKey)
	assert.NotNil(t, globalKeyStore.TrustedPublicKeys)
	assert.Equal(t, 0, len(globalKeyStore.TrustedPublicKeys))
}

func TestLoadKeyStore(t *testing.T) {
	config, cleanup := CreateTestConfig()
	defer cleanup()

	// Test keys should be loaded.
	loadKeyStore(config, testKeyPath, true)
	defer CloseKeyStore()

	assert.NotNil(t, globalKeyStore)
	assert.NotNil(t, globalKeyStore.MyPrivateKey)
	assert.NotNil(t, globalKeyStore.TrustedPublicKeys)

	// test-public-2.pem contains two public keys, which both should be loaded.
	assert.Equal(t, 3, len(globalKeyStore.TrustedPublicKeys))

	// Test keys should not be loaded.
	LoadKeyStore(config, testKeyPath)
	defer CloseKeyStore()

	assert.NotNil(t, globalKeyStore)
	assert.Nil(t, globalKeyStore.MyPrivateKey)
	assert.NotNil(t, globalKeyStore.TrustedPublicKeys)
	assert.Equal(t, 0, len(globalKeyStore.TrustedPublicKeys))
}

func TestGetKeyStore(t *testing.T) {
	config, cleanup := CreateTestConfig()
	defer cleanup()

	// Test keys should be loaded.
	loadKeyStore(config, testKeyPath, true)
	defer CloseKeyStore()

	store := GetKeyStore()
	assert.Equal(t, store.MyPrivateKey, globalKeyStore.MyPrivateKey)
	assert.Equal(t, store.TrustedPublicKeys, globalKeyStore.TrustedPublicKeys)

	// If the global keystore changes, the returned keystore should not.
	globalKeyStore.MyPrivateKey = nil
	globalKeyStore.TrustedPublicKeys = nil

	assert.NotNil(t, store.MyPrivateKey)
	assert.NotNil(t, store.TrustedPublicKeys)
}

func TestDownloadKeys(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	config, cleanup := CreateTestConfig()
	defer cleanup()

	keyPath := path.Join(config.TestTempDir, "downloaded-public.pem")
	lastModPath := path.Join(config.TestTempDir, "downloaded-public.last-modified")
	defer os.Remove(keyPath)
	defer os.Remove(lastModPath)

	getRequestPerformed := make(chan struct{})
	httpmock.RegisterResponder("GET", config.PublicKeysURL,
		func(req *http.Request) (*http.Response, error) {
			defer close(getRequestPerformed)

			resp := httpmock.NewStringResponse(200, "public-keys")
			resp.Header.Set("Last-Modified", "je moeder")
			return resp, nil
		},
	)

	downloadKeysInitialWait = 10 * time.Millisecond
	loadKeyStore(config, config.TestTempDir, true)
	globalKeyStore.Go()

	select {
	case <-getRequestPerformed:
	case <-time.After(250 * time.Millisecond):
		assert.Fail(t, "GET request to public keys URL not performed within timeout")
	}
	CloseKeyStore()
	assert.Nil(t, globalKeyStore)

	// Check that the public keys have been saved successfully.
	assert.FileExists(t, keyPath)
	assert.FileExists(t, lastModPath)
	keys, err := ioutil.ReadFile(keyPath)
	assert.Nil(t, err)
	assert.Equal(t, "public-keys", string(keys))

	lastMod, err := ioutil.ReadFile(lastModPath)
	assert.Nil(t, err)
	assert.Equal(t, "je moeder", string(lastMod))
}
