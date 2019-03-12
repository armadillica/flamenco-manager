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
	"crypto/ecdsa"
	"path"
	"strings"
	"sync"
)

// KeyStore contains a private and public keys.
type KeyStore struct {
	// Private key used for generating JWTs. May be nil when no private key is loaded.
	MyPrivateKey *ecdsa.PrivateKey

	// Any key in this array is trusted as authoritative for received JWTs.
	TrustedPublicKeys []*ecdsa.PublicKey
}

var globalKeyStore *KeyStore
var globalKeyStoreMutex sync.Mutex

// newKeyStore loads all keys in the given directory.
func newKeyStore(keyDirectory string, mayLoadTestKeys bool) *KeyStore {
	privateFilename := path.Join(keyDirectory, "es256-private.pem")

	var myPrivateKey *ecdsa.PrivateKey
	if mayLoadTestKeys || !strings.Contains(privateFilename, "test") {
		myPrivateKey = ReadPrivateKey(privateFilename)
	}

	trustedPublicKeys := ReadAllPublicKeys(keyDirectory, mayLoadTestKeys)

	return &KeyStore{
		myPrivateKey,
		trustedPublicKeys,
	}
}

// LoadKeyStore loads all keys from a directory and stores them in the global key store.
func LoadKeyStore(keyDirectory string) {
	loadKeyStore(keyDirectory, false)
}

func loadKeyStore(keyDirectory string, mayLoadTestKeys bool) {
	keyStore := newKeyStore(keyDirectory, mayLoadTestKeys)

	globalKeyStoreMutex.Lock()
	defer globalKeyStoreMutex.Unlock()
	globalKeyStore = keyStore
}

// GetKeyStore returns a shallow copy of the global KeyStore.
// This allows the global keystore to be modified while it is in use.
// The returned KeyStore should be used immediately, no references/copies kept.
func GetKeyStore() KeyStore {
	// Prevent modification while we're copying the keystore.
	globalKeyStoreMutex.Lock()
	defer globalKeyStoreMutex.Unlock()

	if globalKeyStore == nil {
		globalKeyStore = &KeyStore{}
	}

	return *globalKeyStore
}
