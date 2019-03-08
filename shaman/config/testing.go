package config

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
	"os"
	"path"
	"time"
)

// CreateTestConfig creates a configuration + cleanup function.
func CreateTestConfig() (conf Config, cleanup func()) {
	tempDir, err := ioutil.TempDir("", "shaman-test-")
	if err != nil {
		panic(err)
	}

	conf = Config{
		TestTempDir:   tempDir,
		FileStorePath: path.Join(tempDir, "file-store"),
		CheckoutPath:  path.Join(tempDir, "checkout"),

		JWTPublicKeysURL: "http://localhost:51234/api/flamenco/jwt/public-keys",

		GarbageCollect: GarbageCollect{
			Period:            8 * time.Hour,
			MaxAge:            31 * 24 * time.Hour,
			ExtraCheckoutDirs: []string{},
		},
	}

	cleanup = func() {
		if err := os.RemoveAll(tempDir); err != nil {
			panic(err)
		}
	}
	return
}
