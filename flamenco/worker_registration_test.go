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

package flamenco

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
	check "gopkg.in/check.v1"
)

type WorkerRegistrationTestSuite struct{}

var _ = check.Suite(&WorkerRegistrationTestSuite{})

func (s *WorkerRegistrationTestSuite) TestEmptyString(t *check.C) {
	conf := Conf{Base: Base{WorkerRegistrationSecret: ""}}
	auther := NewWorkerRegistrationAuthoriser(&conf)
	_, ok := auther.(OpenRegAuth)
	assert.True(t, ok, "empty PSK should result in open registration")

	wasCalled := false
	httpFunc := func(w http.ResponseWriter, r *http.Request) {
		wasCalled = true
	}
	wrapped := auther.WrapFunc(httpFunc)

	respRec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/register-worker", nil)
	wrapped.ServeHTTP(respRec, req)

	assert.True(t, wasCalled, "wrapped function was not called")
}

func (s *WorkerRegistrationTestSuite) TestPSKString(t *check.C) {
	conf := Conf{Base: Base{WorkerRegistrationSecret: "je moeder op je hoofd"}}
	auther := NewWorkerRegistrationAuthoriser(&conf)
	psk, ok := auther.(*PSKRegAuth)
	assert.True(t, ok, "non-PSK should result in secure registration")
	assert.Equal(t, string(psk.preSharedSecret), "je moeder op je hoofd")
}

func (s *WorkerRegistrationTestSuite) TestRejectWithoutToken(t *check.C) {
	conf := Conf{Base: Base{WorkerRegistrationSecret: "je moeder op je hoofd"}}
	auther := NewWorkerRegistrationAuthoriser(&conf)

	httpFunc := func(w http.ResponseWriter, r *http.Request) {
		assert.Fail(t, "unexpectedly called")
	}
	wrapped := auther.WrapFunc(httpFunc)

	// Test without token.
	respRec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/register-worker", nil)
	wrapped.ServeHTTP(respRec, req)
	assert.Equal(t, http.StatusForbidden, respRec.Code)

	// Test with token signed with bad secret.
	respRec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/register-worker", nil)
	token := generateHMACToken("the wrong key")
	req.Header.Set("Authorization", "Bearer "+token)
	wrapped.ServeHTTP(respRec, req)
	assert.Equal(t, http.StatusForbidden, respRec.Code)
}

func generateHMACToken(secret string) string {
	now := time.Now().UTC()
	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		jwt.StandardClaims{
			ExpiresAt: now.Add(10 * time.Second).Unix(),
			IssuedAt:  now.Unix(),
		})
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		panic(fmt.Sprintf("error signing token: %v", err))
	}
	return tokenString
}

func (s *WorkerRegistrationTestSuite) TestAcceptGoodToken(t *check.C) {
	conf := Conf{Base: Base{WorkerRegistrationSecret: "je moeder op je hoofd"}}
	auther := NewWorkerRegistrationAuthoriser(&conf)

	wasCalled := false
	httpFunc := func(w http.ResponseWriter, r *http.Request) {
		wasCalled = true
	}
	wrapped := auther.WrapFunc(httpFunc)

	respRec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/register-worker", nil)
	token := generateHMACToken("je moeder op je hoofd")
	req.Header.Set("Authorization", "Bearer "+token)
	wrapped.ServeHTTP(respRec, req)

	assert.True(t, wasCalled, "wrapped function was not called")
}
