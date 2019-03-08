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
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"path"
	"path/filepath"
	"strings"

	jwt "github.com/dgrijalva/jwt-go"
)

// ReadPrivateKey reads a PEM file as ECDSA private key.
func ReadPrivateKey(filePath string) *ecdsa.PrivateKey {
	logger := packageLogger.WithField("keyPath", filePath)

	keyBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		logger.WithError(err).Info("unable to read private key")
		return nil
	}

	key, err := jwt.ParseECPrivateKeyFromPEM(keyBytes)
	if err != nil {
		logger.WithError(err).Error("unable to parse private key")
		return nil
	}

	return key
}

// ReadPublicKey reads a PEM file as ECDSA public key.
func ReadPublicKey(filePath string) []*ecdsa.PublicKey {
	logger := packageLogger.WithField("keyPath", filePath)

	keyBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		logger.WithError(err).Warning("unable to read public key")
		return nil
	}

	keys := []*ecdsa.PublicKey{}
	var key *ecdsa.PublicKey
	for len(keyBytes) > 0 {
		key, keyBytes, err = ParseECPublicKeyFromPEM(keyBytes)
		if err != nil {
			logger.WithError(err).Error("unable to parse public key")
			return nil
		}

		keys = append(keys, key)
	}

	return keys
}

// ReadAllPublicKeys reads all *-public*.pem files in the root path as ECDSA public keys.
func ReadAllPublicKeys(keyDirectory string, mayLoadTestKeys bool) []*ecdsa.PublicKey {
	keys := make([]*ecdsa.PublicKey, 0)
	logger := packageLogger.WithField("keyDirectory", keyDirectory)

	keyPaths, err := filepath.Glob(path.Join(keyDirectory, "*-public*.pem"))
	if err != nil {
		logger.WithError(err).Error("unable to list public keys")
		return keys
	}
	if len(keyPaths) == 0 {
		logger.Warning("no public keys found")
		return keys
	}

	for _, keyPath := range keyPaths {
		if !mayLoadTestKeys && strings.Contains(keyPath, "test") {
			logger.WithField("keyPath", keyPath).Debug("skipping test key")
			continue
		}
		keysInFile := ReadPublicKey(keyPath)
		if keysInFile == nil {
			continue
		}
		keys = append(keys, keysInFile...)
	}

	logger.WithField("keyCount", len(keys)).Debug("loaded public keys")
	return keys
}

// ParseECPublicKeyFromPEM parses PEM encoded PKCS1 or PKCS8 public key
// This function is copied from jwt.ParseECPublicKeyFromPEM() and modified
// to support multiple keys in the same file.
// See https://github.com/dgrijalva/jwt-go/issues/317
func ParseECPublicKeyFromPEM(key []byte) (pkey *ecdsa.PublicKey, rest []byte, err error) {
	// Parse PEM block
	var block *pem.Block
	if block, rest = pem.Decode(key); block == nil {
		err = jwt.ErrKeyMustBePEMEncoded
		return
	}

	// Parse the key
	var parsedKey interface{}
	if parsedKey, err = x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		var cert *x509.Certificate
		if cert, err = x509.ParseCertificate(block.Bytes); err == nil {
			parsedKey = cert.PublicKey
		} else {
			return
		}
	}

	var ok bool
	if pkey, ok = parsedKey.(*ecdsa.PublicKey); !ok {
		err = jwt.ErrNotECPublicKey
		return
	}

	return
}
