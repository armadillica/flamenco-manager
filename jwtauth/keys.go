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
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	jwt "github.com/dgrijalva/jwt-go"
)

// ReadPrivateKey reads a PEM file as ECDSA private key.
func ReadPrivateKey(filePath string) *ecdsa.PrivateKey {
	logger := packageLogger.WithField("keyPath", filePath)

	keyBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		level := logrus.WarnLevel
		if os.IsNotExist(err) {
			level = logrus.DebugLevel
		}
		logger.WithError(err).Log(level, "unable to read private key")
		return nil
	}

	key, err := jwt.ParseECPrivateKeyFromPEM(keyBytes)
	if err != nil {
		logger.WithError(err).Error("unable to parse private key")
		return nil
	}
	logger.Info("loaded JWT private key")

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
		logger.Debug("no public keys found")
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

	logger.WithField("keyCount", len(keys)).Info("loaded JWT public keys")
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
