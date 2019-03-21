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
	"errors"
	"net/http"
)

// AlwaysDeny is an always-denying Authenticator.
type AlwaysDeny struct{}

// AlwaysAllow is an always-allowing Authenticator.
type AlwaysAllow struct{}

var errNotImplemented = errors.New("not implemented")

// Wrap makes the wrapped handler uncallable because everything is rejected.
func (f AlwaysDeny) Wrap(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "JWT token denied", http.StatusUnauthorized)
		return
	})
}

// WrapFunc makes the wrapped handlerFunc uncallable because everything is rejected.
func (f AlwaysDeny) WrapFunc(handlerFunc func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return f.Wrap(http.HandlerFunc(handlerFunc))
}

// GenerateToken always returns an error and never generates a token.
func (f AlwaysDeny) GenerateToken() (string, error) {
	return "", errNotImplemented
}

// Wrap does nothing and allows all requests.
func (f AlwaysAllow) Wrap(handler http.Handler) http.Handler {
	return handler
}

// WrapFunc does nothing and allows all requests.
func (f AlwaysAllow) WrapFunc(handlerFunc func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(handlerFunc)
}

// GenerateToken always returns an error and never generates a token.
func (f AlwaysAllow) GenerateToken() (string, error) {
	return "", errNotImplemented
}
