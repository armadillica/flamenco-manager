package auth

/* ***** BEGIN GPL LICENSE BLOCK *****
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License
 * as published by the Free Software Foundation; either version 2
 * of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software Foundation,
 * Inc., 59 Temple Place - Suite 330, Boston, MA  02111-1307, USA.
 *
 * ***** END GPL LICENCE BLOCK *****
 *
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
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
