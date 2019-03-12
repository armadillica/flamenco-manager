package auth

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
	"context"

	jwt "github.com/dgrijalva/jwt-go"
)

type contextKey int

var tokenContextKey contextKey

// NewContext returns a copy of the context with a JWT token included.
func NewContext(ctx context.Context, token *jwt.Token) context.Context {
	return context.WithValue(ctx, tokenContextKey, token)
}

// FromContext returns the Token value stored in ctx, if any.
func FromContext(ctx context.Context) (*jwt.Token, bool) {
	token, ok := ctx.Value(tokenContextKey).(*jwt.Token)
	return token, ok
}

// SubjectFromContext returns the UserID stored in the token's subject field, if any.
func SubjectFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(tokenContextKey).(*jwt.Token)
	if !ok {
		return "", false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}
	subject := claims["sub"].(string)
	if subject == "" {
		return "", false
	}
	return subject, true
}
