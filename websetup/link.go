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

package websetup

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"net/http"
	"net/url"

	"github.com/armadillica/flamenco-manager/flamenco"

	log "github.com/sirupsen/logrus"
)

// ServerLinker manages linking this Manager to Flamenco Server
type ServerLinker struct {
	upstream   *url.URL // base URL of Flamenco Server, like https://cloud.blender.org/
	localURL   *url.URL // our own URL, based on the URL the user visited to initate linking
	key        []byte   // the secret key we'll send to Flamenco Server
	identifier string   // the identifier we got back from Flamenco Server during key exchange
	managerID  string   // our ObjectID at Flamenco Server
}

// End points at Flamenco Server
const (
	keyExchangeEndpoint       = "/api/flamenco/managers/link/exchange"
	authTokenResetEndpoint    = "/api/flamenco/managers/link/reset-token"
	linkStartRedirectEndpoint = "/flamenco/managers/link/choose"
)

// Errors
var (
	errMissingKey        = errors.New("key missing, secret key exchange was not performed")
	errMissingIdentifier = errors.New("identifier missing, secret key exchange was not performed")
	errMissingLocalURL   = errors.New("local URL is not known")
	errMissingManagerID  = errors.New("manager ID is missing, restart the linking process")
)

// LinkRequired returns true iff (re)linking to Flamenco Server is required.
func LinkRequired(config *flamenco.Conf) bool {
	// Check upstream server URL.
	if config.Flamenco == nil {
		log.Debug("Flamenco Server URL is nil, linking is required.")
		return true
	}

	// Check existence of credentials.
	if config.ManagerID == "" || config.ManagerSecret == "" {
		log.Debug("Credentials incomplete, linking is required.")
		return true
	}

	// Check the validity of the credentials.
	strURL := "/api/flamenco/managers/" + config.ManagerID
	getURL, err := config.Flamenco.Parse(strURL)
	if err != nil {
		log.Warningf("Error parsing '%s' as URL; unable to check credentials: %s", strURL, err)
		return true
	}

	req, err := http.NewRequest("GET", getURL.String(), nil)
	if err != nil {
		log.Warningf("Unable to create GET request: %s", err)
		return true
	}
	req.SetBasicAuth(config.ManagerSecret, "")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Warningf("Unable to GET %s: %s", getURL, err)
		return true
	}
	if resp.StatusCode != http.StatusOK {
		log.Warningf("HTTP status %d while fetching manager %s, linking required",
			resp.StatusCode, config.ManagerID)
		return true
	}
	log.Debugf("Credentials are still valid, no need to link.")

	return false
}

// StartLinking starts the linking process by generating a secret key.
func StartLinking(upstreamURL string, localURL *url.URL) (*ServerLinker, error) {
	upstream, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %s: %s", upstreamURL, err)
	}

	linker := ServerLinker{
		upstream: upstream,
		key:      make([]byte, 32),
		localURL: localURL,
	}

	_, err = rand.Read(linker.key)
	if err != nil {
		return nil, fmt.Errorf("Unable to generate secret key: %v", err)
	}

	return &linker, nil
}

// HasIdentifier returns true iff an identifier was received from Server.
func (linker *ServerLinker) HasIdentifier() bool {
	return linker.identifier != ""
}

// HasManagerID returns true iff a Manager ID was received from Server.
func (linker *ServerLinker) HasManagerID() bool {
	return linker.managerID != ""
}

// ExchangeKey sends the locally generated key to Server and receives an identifier.
func (linker *ServerLinker) ExchangeKey() error {
	requestURL, err := linker.upstream.Parse(keyExchangeEndpoint)
	if err != nil {
		return fmt.Errorf("error constructing key exchange URL %s: %s", keyExchangeEndpoint, err)
	}
	log.Infof("Exchanging secret key with Flamenco Server %s", linker.upstream)

	payload := keyExchangeRequest{
		KeyHex: linker.keyHex(),
	}

	responseHandler := func(resp *http.Response, body []byte) error {
		// Parse the JSON we received.
		var keyResp keyExchangeResponse
		if err = json.Unmarshal(body, &keyResp); err != nil {
			return fmt.Errorf("unable to decode key exchange response from %s: %s", requestURL, err)
		}

		log.Infof("Key exchange with Flamenco Server succesful")
		linker.identifier = keyResp.Identifier

		return nil
	}

	err = flamenco.SendJSON("Key exchange", "POST", requestURL, &payload, nil, responseHandler)
	if err != nil {
		return err
	}

	return nil
}

// KeyHex returns the secret key in hexadecimal notation.
func (linker *ServerLinker) keyHex() string {
	return hex.EncodeToString(linker.key)
}

func (linker *ServerLinker) hmacObject() (hash.Hash, error) {
	if len(linker.key) == 0 {
		return nil, errMissingKey
	}
	if linker.identifier == "" {
		return nil, errMissingIdentifier
	}

	return hmac.New(sha256.New, linker.key), nil
}

// Returns the URL to Flamenco Server to let the user choose a Manager there (or create one).
func (linker *ServerLinker) redirectURL() (*url.URL, error) {
	var err error

	if linker.localURL == nil {
		return nil, errMissingLocalURL
	}

	redirectURL, err := linker.upstream.Parse(linkStartRedirectEndpoint)
	if err != nil {
		return nil, err
	}

	// Construct query string
	q := redirectURL.Query()
	q.Set("identifier", linker.identifier)
	returnURL, err := linker.localURL.Parse(linkReturnURL)
	if err != nil {
		return nil, err
	}
	returnStr := returnURL.String()
	q.Set("return", returnStr)

	// Compute the HMAC
	identHMAC, err := linker.hmacObject()
	if err != nil {
		return nil, err
	}
	if _, err = identHMAC.Write([]byte(linker.identifier + "-" + returnStr)); err != nil {
		return nil, err
	}
	mac := identHMAC.Sum(nil)
	q.Set("hmac", hex.EncodeToString(mac))

	redirectURL.RawQuery = q.Encode()
	return redirectURL, nil
}

// resetAuthToken uses the link info to fetch a new authentication token.
func (linker *ServerLinker) resetAuthToken() (string, error) {
	if linker.identifier == "" {
		return "", errMissingIdentifier
	}
	if linker.managerID == "" {
		return "", errMissingManagerID
	}

	requestURL, err := linker.upstream.Parse(authTokenResetEndpoint)
	if err != nil {
		return "", fmt.Errorf("error constructing token reset URL %s: %s", authTokenResetEndpoint, err)
	}
	log.Infof("Requesting new auth token from Flamenco Server %s", linker.upstream)

	paddingBytes := make([]byte, 32)
	if _, err = rand.Read(paddingBytes); err != nil {
		return "", fmt.Errorf("error generating random padding: %s", err)
	}
	payload := authTokenResetRequest{
		ManagerID:  linker.managerID,
		Identifier: linker.identifier,
		Padding:    hex.EncodeToString(paddingBytes),
	}

	// Compute the HMAC.
	identHMAC, err := linker.hmacObject()
	if err != nil {
		return "", err
	}
	if _, err = identHMAC.Write([]byte(payload.Padding + "-" + payload.Identifier + "-" + payload.ManagerID)); err != nil {
		return "", err
	}
	payload.HMAC = hex.EncodeToString(identHMAC.Sum(nil))

	// Send the request and handle the response.
	var receivedToken string
	responseHandler := func(resp *http.Response, body []byte) error {
		// Parse the JSON we received.
		var tokenResp authTokenResetResponse
		if err = json.Unmarshal(body, &tokenResp); err != nil {
			return fmt.Errorf("unable to decode auth token response from %s: %s", requestURL, err)
		}

		if tokenResp.Token == "" {
			return fmt.Errorf("received empty authentication token from %s", requestURL)
		}

		log.Infof("Authentication token reset at Flamenco Server succesful")
		receivedToken = tokenResp.Token
		return nil
	}

	err = flamenco.SendJSON("Token reset", "POST", requestURL, &payload, nil, responseHandler)
	if err != nil {
		return "", err
	}

	return receivedToken, nil
}
