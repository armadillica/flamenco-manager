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

package jwtauth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"runtime"
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var testKeyPath string

func init() {
	_, myFilename, _, _ := runtime.Caller(0)
	testKeyPath = path.Join(path.Dir(myFilename), "test-keys")
}

func TestNewJWT(t *testing.T) {
	auther := NewJWT(false)
	assert.Equal(t, false, auther.friendly)
}

func generateUnsignedToken(method jwt.SigningMethod, expiresAfter time.Duration) *jwt.Token {
	now := time.Now().UTC()
	claims := jwt.StandardClaims{
		Audience:  "12345", // TODO: my own ID.
		ExpiresAt: now.Add(expiresAfter).Unix(),
		Subject:   "user-ID", // TODO: user ObjectID
		IssuedAt:  now.Unix(),
	}
	token := jwt.NewWithClaims(method, claims)
	if token == nil {
		panic("nil token")
	}
	return token
}

func generateHMACToken() string {
	token := generateUnsignedToken(jwt.SigningMethodHS256, time.Hour*24)
	tokenString, err := token.SignedString([]byte("my faked private key"))
	if err != nil {
		panic(fmt.Sprintf("error signing token: %v", err))
	}
	return tokenString
}

func generateSecondKeyToken() string {
	privateKey := ReadPrivateKey("./test-keys/test-private-2.pem")

	token := generateUnsignedToken(jwt.SigningMethodES256, time.Hour*24)
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		panic(fmt.Sprintf("error signing token: %v", err))
	}
	if tokenString == "" {
		panic("no error, but also no token")
	}
	return tokenString
}

func httpPanic(w http.ResponseWriter, r *http.Request) {
	panic("this function should not be called")
}

func TestGenerateKey(t *testing.T) {
	auther := NewJWT(true)

	// Without loading keys, generating a token should be impossibru.
	tokenString, err := auther.GenerateToken()
	assert.Equal(t, err, errNoPrivateKeyLoaded)
	assert.Empty(t, tokenString)

	// After loading it should be fine, even without recreating the auther.
	loadKeyStore(Config{}, testKeyPath, true)
	tokenString, err = auther.GenerateToken()
	assert.Nil(t, err)
	assert.NotEmpty(t, tokenString)

	token, err := auther.validate(tokenString, logrus.WithField("testing", "testing"))
	assert.Nil(t, err)
	assert.True(t, token.Valid)

	// Any modification should be noticed.
	invalidTokenString := tokenString[:30] + "3" + tokenString[31:]
	token, err = auther.validate(invalidTokenString, logrus.WithField("testing", "testing"))
	assert.NotNil(t, err)
	assert.Nil(t, token)
}

func TestRequest(t *testing.T) {
	auther := NewJWT(true)
	loadKeyStore(Config{}, testKeyPath, true)

	httpFuncCalled := false
	expectedTokenString := ""
	httpFunc := func(w http.ResponseWriter, r *http.Request) {
		httpFuncCalled = true
		w.Write([]byte("That's all good!"))

		token, ok := FromContext(r.Context())
		assert.True(t, ok)
		assert.True(t, token.Valid)
		assert.Equal(t, expectedTokenString, token.Raw)
	}
	wrapped := auther.WrapFunc(httpFunc)

	// Unauthenticated call, so httpFunc should not be called.
	respRec := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://flamanager.local/files/sha256sum/4700", nil)
	wrapped.ServeHTTP(respRec, request)
	assert.False(t, httpFuncCalled)
	assert.Equal(t, http.StatusUnauthorized, respRec.Code)

	// 'Authenticate' with a non-Bearer header
	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://flamanager.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", "Basic username:password")
	wrapped.ServeHTTP(respRec, request)
	assert.False(t, httpFuncCalled)
	assert.Equal(t, http.StatusUnauthorized, respRec.Code)

	// 'Authenticate' with a too-short header
	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://flamanager.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", "Baa")
	wrapped.ServeHTTP(respRec, request)
	assert.False(t, httpFuncCalled)
	assert.Equal(t, http.StatusUnauthorized, respRec.Code)

	// 'Authenticate' with an invalid token.
	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://flamanager.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", "Bearer 123.abc.327")
	wrapped.ServeHTTP(respRec, request)
	assert.False(t, httpFuncCalled)
	assert.Equal(t, http.StatusUnauthorized, respRec.Code)

	// Authenticate with a valid token
	tokenString, _ := auther.GenerateToken()
	assert.NotEmpty(t, tokenString)

	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://flamanager.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	expectedTokenString = tokenString
	wrapped.ServeHTTP(respRec, request)
	assert.True(t, httpFuncCalled)
	assert.Equal(t, http.StatusOK, respRec.Code)

	// Authenticate with a valid token that's signed with the secondary key.
	tokenString = generateSecondKeyToken()
	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://flamanager.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	expectedTokenString = tokenString
	httpFuncCalled = false
	wrapped.ServeHTTP(respRec, request)
	assert.True(t, httpFuncCalled)
	assert.Equal(t, http.StatusOK, respRec.Code)

}

func TestWrongTokens(t *testing.T) {
	auther := NewJWT(true)
	wrapped := auther.WrapFunc(httpPanic)
	loadKeyStore(Config{}, "./test-keys", true)

	// Generate a token with the wrong signing method.
	tokenString := generateHMACToken()
	respRec := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://flamanager.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	wrapped.ServeHTTP(respRec, request)
	assert.Equal(t, http.StatusUnauthorized, respRec.Code)

	// Generate a token that's correctly signed but expired
	token := generateUnsignedToken(jwt.SigningMethodES256, -10*time.Minute)
	tokenString, err := token.SignedString(globalKeyStore.MyPrivateKey)
	assert.Nil(t, err)
	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://flamanager.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	wrapped.ServeHTTP(respRec, request)
	assert.Equal(t, StatusTokenExpired, respRec.Code)
}
