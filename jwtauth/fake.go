package jwtauth

import (
	"errors"
	"net/http"
)

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

// Fake is an always-denying Authenticator.
type Fake struct{}

var errNotImplemented = errors.New("not implemented")

// Wrap makes the wrapped handler uncallable because everything is rejected.
func (f Fake) Wrap(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "JWT token denied", http.StatusUnauthorized)
		return
	})
}

// WrapFunc makes the wrapped handlerFunc uncallable because everything is rejected.
func (f Fake) WrapFunc(handlerFunc func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return f.Wrap(http.HandlerFunc(handlerFunc))
}

// GenerateToken always returns an error and never generates a token.
func (f Fake) GenerateToken() (string, error) {
	return "", errNotImplemented
}
