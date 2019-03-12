package auth

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadKeyStoreNonExistant(t *testing.T) {
	loadKeyStore("./non-existant-dir", true)
	assert.NotNil(t, globalKeyStore)
	assert.Nil(t, globalKeyStore.MyPrivateKey)
	assert.NotNil(t, globalKeyStore.TrustedPublicKeys)
	assert.Equal(t, 0, len(globalKeyStore.TrustedPublicKeys))
}

func TestLoadKeyStore(t *testing.T) {
	// Test keys should be loaded.
	loadKeyStore("./test-keys", true)
	assert.NotNil(t, globalKeyStore)
	assert.NotNil(t, globalKeyStore.MyPrivateKey)
	assert.NotNil(t, globalKeyStore.TrustedPublicKeys)

	// test-public-2.pem contains two public keys, which both should be loaded.
	assert.Equal(t, 3, len(globalKeyStore.TrustedPublicKeys))

	// Test keys should not be loaded.
	LoadKeyStore("./test-keys")
	assert.NotNil(t, globalKeyStore)
	assert.Nil(t, globalKeyStore.MyPrivateKey)
	assert.NotNil(t, globalKeyStore.TrustedPublicKeys)
	assert.Equal(t, 0, len(globalKeyStore.TrustedPublicKeys))
}

func TestGetKeyStore(t *testing.T) {
	// Test keys should be loaded.
	loadKeyStore("./test-keys", true)

	store := GetKeyStore()
	assert.Equal(t, store.MyPrivateKey, globalKeyStore.MyPrivateKey)
	assert.Equal(t, store.TrustedPublicKeys, globalKeyStore.TrustedPublicKeys)

	// If the global keystore changes, the returned keystore should not.
	globalKeyStore.MyPrivateKey = nil
	globalKeyStore.TrustedPublicKeys = nil

	assert.NotNil(t, store.MyPrivateKey)
	assert.NotNil(t, store.TrustedPublicKeys)
}
