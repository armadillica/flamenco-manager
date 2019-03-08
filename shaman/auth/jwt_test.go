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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

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
	loadKeyStore("./test-keys", true)
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
	loadKeyStore("./test-keys", true)

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
	request := httptest.NewRequest("POST", "http://shaman.local/files/sha256sum/4700", nil)
	wrapped.ServeHTTP(respRec, request)
	assert.False(t, httpFuncCalled)
	assert.Equal(t, http.StatusUnauthorized, respRec.Code)

	// 'Authenticate' with a non-Bearer header
	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://shaman.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", "Basic username:password")
	wrapped.ServeHTTP(respRec, request)
	assert.False(t, httpFuncCalled)
	assert.Equal(t, http.StatusUnauthorized, respRec.Code)

	// 'Authenticate' with a too-short header
	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://shaman.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", "Baa")
	wrapped.ServeHTTP(respRec, request)
	assert.False(t, httpFuncCalled)
	assert.Equal(t, http.StatusUnauthorized, respRec.Code)

	// 'Authenticate' with an invalid token.
	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://shaman.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", "Bearer 123.abc.327")
	wrapped.ServeHTTP(respRec, request)
	assert.False(t, httpFuncCalled)
	assert.Equal(t, http.StatusUnauthorized, respRec.Code)

	// Authenticate with a valid token
	tokenString, _ := auther.GenerateToken()
	assert.NotEmpty(t, tokenString)

	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://shaman.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	expectedTokenString = tokenString
	wrapped.ServeHTTP(respRec, request)
	assert.True(t, httpFuncCalled)
	assert.Equal(t, http.StatusOK, respRec.Code)

	// Authenticate with a valid token that's signed with the secondary key.
	tokenString = generateSecondKeyToken()
	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://shaman.local/files/sha256sum/4700", nil)
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
	loadKeyStore("./test-keys", true)

	// Generate a token with the wrong signing method.
	tokenString := generateHMACToken()
	respRec := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://shaman.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	wrapped.ServeHTTP(respRec, request)
	assert.Equal(t, http.StatusUnauthorized, respRec.Code)

	// Generate a token that's correctly signed but expired
	token := generateUnsignedToken(jwt.SigningMethodES256, -10*time.Minute)
	tokenString, err := token.SignedString(globalKeyStore.MyPrivateKey)
	assert.Nil(t, err)
	respRec = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "http://shaman.local/files/sha256sum/4700", nil)
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenString))
	wrapped.ServeHTTP(respRec, request)
	assert.Equal(t, StatusTokenExpired, respRec.Code)
}
