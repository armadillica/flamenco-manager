package websetup

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flamenco-manager/flamenco"
	"fmt"
	"hash"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
)

// ServerLinker manages linking this Manager to Flamenco Server
type ServerLinker struct {
	upstream   *url.URL
	key        []byte
	identifier string
	managerID  string
}

// End points at Flamenco Server
const (
	keyExchangeEndpoint       = "/api/flamenco/managers/link/exchange"
	linkStartRedirectEndpoint = "/flamenco/managers/link/choose"
)

// Errors
var (
	errMissingKey        = errors.New("key missing, secret key exchange was not performed")
	errMissingIdentifier = errors.New("identifier missing, secret key exchange was not performed")
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
func StartLinking(upstreamURL string) (*ServerLinker, error) {
	upstream, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %s: %s", upstreamURL, err)
	}

	linker := ServerLinker{
		upstream: upstream,
		key:      make([]byte, 32),
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

func (linker *ServerLinker) redirectURL() (*url.URL, error) {
	redirectURL, err := linker.upstream.Parse(linkStartRedirectEndpoint)
	if err != nil {
		return nil, err
	}

	// We have to add the identifier and compute the HMAC.
	identHMAC, err := linker.hmacObject()
	if err != nil {
		return nil, err
	}
	mac := identHMAC.Sum([]byte(linker.identifier))

	q := redirectURL.Query()
	q.Set("identifier", linker.identifier)
	q.Set("hmac", hex.EncodeToString(mac))
	redirectURL.RawQuery = q.Encode()

	return redirectURL, nil
}
