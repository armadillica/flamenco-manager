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
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/sirupsen/logrus"
)

var (
	errNoAuthHeader       = errors.New("No Authorization header")
	errNoBearerToken      = errors.New("Bearer token is required in Authorization header")
	errNoPrivateKeyLoaded = errors.New("No private key loaded")
	errTokenExpired       = errors.New("JTW token expired")
)

// JWT is a HTTP handler that authenticates JWT bearer tokens.
type JWT struct {
	friendly bool // whether JWT parse errors are included in the returned error.
}

// NewJWT returns an authentication wrapper for HTTP handlers.
func NewJWT(friendly bool) *JWT {
	return &JWT{friendly}
}

func (j *JWT) validate(tokenString string, logger *logrus.Entry) (*jwt.Token, error) {
	// TODO(Sybren): support multiple signing algorithms.
	signingMethod := jwt.SigningMethodES256

	// Validate the token signature by checking against all our keys.
	parts := strings.Split(tokenString, ".")
	checkSignature := func() error {
		if len(parts) != 3 {
			return jwt.NewValidationError("token is malformed", jwt.ValidationErrorMalformed)
		}
		headerAndPayload := strings.Join(parts[0:2], ".")
		signature := parts[2]
		keyStore := GetKeyStore()

		var err error
		for index, key := range keyStore.TrustedPublicKeys {
			if err = signingMethod.Verify(headerAndPayload, signature, key); err == nil {
				// We found a key for which the signature is valid.
				return nil
			}
			logger.WithFields(logrus.Fields{
				"keyIndex":      index,
				logrus.ErrorKey: err,
			}).Debug("token signature invalid for this key")
		}

		logger.Info("token signature invalid")
		return jwt.NewValidationError("signature is not valid for any of our keys", jwt.ValidationErrorSignatureInvalid)
	}
	if err := checkSignature(); err != nil {
		return nil, err
	}

	// Parse without validation, because the JWT library cannot do the multi-key validation
	// we just did. It can do key selection, but then the token should contain an identifier
	// for the used key. The way it's done above, this isn't necessary.
	parser := jwt.Parser{}
	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		logger.WithError(err).Warning("parsed JWT token not valid")
		return nil, err
	}

	alg := token.Method.Alg()
	if alg != signingMethod.Alg() {
		return nil, jwt.NewValidationError(fmt.Sprintf("signing method %v is invalid", alg), jwt.ValidationErrorSignatureInvalid)
	}

	if err := token.Claims.Valid(); err != nil {
		if jwtErr, ok := err.(*jwt.ValidationError); ok && (jwtErr.Errors&jwt.ValidationErrorExpired) != 0 {
			// The token is expired; get a new one.
			return nil, errTokenExpired
		}
		return nil, err
	}

	token.Signature = parts[2]
	token.Valid = true
	return token, nil
}

func (j *JWT) parseBearerToken(r *http.Request) (*jwt.Token, error) {
	logger := RequestLogger(r)

	// Get Authorization header
	header := r.Header.Get("Authorization")
	if header == "" {
		logger.Debug("no authorization header")
		return nil, errNoAuthHeader
	}

	// Get the Bearer token
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		logger.Debug("no bearer token in the authorization header")
		return nil, errNoBearerToken
	}
	tokenString := header[len(prefix):]

	return j.validate(tokenString, logger)
}

// Wrap a HTTP handler to provide Bearer token authentication.
func (j *JWT) Wrap(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := j.parseBearerToken(r)

		if err == errTokenExpired {
			http.Error(w, "JWT token expired", StatusTokenExpired)
			return
		}
		if err != nil {
			msg := "Bearer token authorization required"
			if j.friendly {
				msg = fmt.Sprintf("%s: %s", msg, err.Error())
			}
			http.Error(w, msg, http.StatusUnauthorized)
			return
		}

		ctx := NewContext(r.Context(), token)
		subject, ok := SubjectFromContext(ctx)
		if ok {
			w.Header().Set("x-user-id-from-token", subject)
		}

		authedRequest := r.WithContext(ctx)
		handler.ServeHTTP(w, authedRequest)
	})
}

// WrapFunc wraps a HTTP handler function to provide Bearer token authentication.
func (j *JWT) WrapFunc(handlerFunc func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return j.Wrap(http.HandlerFunc(handlerFunc))
}

// GenerateToken generates a new JWT token.
func (j *JWT) GenerateToken() (string, error) {
	keyStore := GetKeyStore()
	if keyStore.MyPrivateKey == nil {
		return "", errNoPrivateKeyLoaded
	}

	now := time.Now().UTC()

	// TODO: parameterise these claims.
	claims := jwt.StandardClaims{
		Audience:  "12345", // TODO: my own ID.
		ExpiresAt: now.Add(time.Hour * 24).Unix(),
		Subject:   "user-ID", // TODO: user ObjectID
		IssuedAt:  now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)

	/* Sign the token with our secret */
	tokenString, err := token.SignedString(keyStore.MyPrivateKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}
