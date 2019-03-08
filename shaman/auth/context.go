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
