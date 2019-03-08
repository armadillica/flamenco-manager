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
