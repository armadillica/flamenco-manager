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
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/armadillica/flamenco-manager/flamenco"

	log "github.com/sirupsen/logrus"
)

var (
	// ErrNoInterface is returned when no network interfaces with a real IP-address were found.
	ErrNoInterface = errors.New("No network interface found")
)

func networkInterfaces(includeLinkLocal, includeLocalhost bool) ([]net.IP, error) {
	log.Debug("Iterating over all network interfaces.")

	interfaces, err := net.Interfaces()
	if err != nil {
		return []net.IP{}, err
	}

	usableAddresses := make([]net.IP, 0)
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for k := range addrs {
			var ip net.IP
			switch a := addrs[k].(type) {
			case *net.IPAddr:
				ip = a.IP.To16()
			case *net.IPNet:
				ip = a.IP.To16()
			default:
				log.Debugf("    - skipping unknown type %v", addrs[k])
				continue
			}

			if ip.IsMulticast() {
				log.Debugf("    - skipping multicast %v", ip)
				continue
			}
			if ip.IsUnspecified() {
				log.Debugf("    - skipping unspecified %v", ip)
				continue
			}
			if !includeLinkLocal && ip.IsLinkLocalUnicast() {
				log.Debugf("    - skipping link-local %v", ip)
				continue
			}
			if !includeLocalhost && ip.IsLoopback() {
				log.Debugf("    - skipping localhost %v", ip)
				continue
			}

			log.Debugf("    - usable %v", ip)
			usableAddresses = append(usableAddresses, ip)
		}
	}

	if len(usableAddresses) == 0 {
		return usableAddresses, ErrNoInterface
	}

	return usableAddresses, nil
}

func availableURLs(config *flamenco.Conf, includeLocal bool) ([]*url.URL, error) {
	var schema string
	if config.HasTLS() {
		schema = "https"
	} else {
		schema = "http"
	}

	var (
		host, port string
		portnum    int
		err        error
	)

	if config.Listen == "" {
		panic("Empty config.Listen")
	}

	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*10)
	defer ctxCancel()

	// Figure out which port we're supposted to listen on.
	if host, port, err = net.SplitHostPort(config.Listen); err != nil {
		return nil, fmt.Errorf("Unable to split host and port in address '%s'", config.Listen)
	}
	if portnum, err = net.DefaultResolver.LookupPort(ctx, "listen", port); err != nil {
		return nil, fmt.Errorf("Unable to look up port '%s'", port)
	}

	// If the host is empty or ::0/0.0.0.0, show a list of URLs to connect to.
	listenSpecificHost := false
	var ip net.IP
	if host != "" {
		ip = net.ParseIP(host)
		if ip == nil {
			addrs, erresolve := net.DefaultResolver.LookupHost(ctx, host)
			if erresolve != nil {
				return nil, fmt.Errorf("Unable to resolve listen host '%v': %s", host, erresolve)
			}
			if len(addrs) > 0 {
				ip = net.ParseIP(addrs[0])
			}
		}
		if ip != nil && !ip.IsUnspecified() {
			listenSpecificHost = true
		}
	}

	if listenSpecificHost {
		log.Debugf("Listening on host %v", ip)
		// We can just construct a URL here, since we know it's a specific host anyway.

		link := fmt.Sprintf("%s://%s:%d/", schema, host, portnum)
		myURL, errparse := url.Parse(link)
		if errparse != nil {
			return nil, fmt.Errorf("Unable to parse listen URL %s: %s", link, errparse)
		}
		return []*url.URL{myURL}, nil
	}

	log.Debugf("Not listening on any specific host '%v'", host)

	addrs, err := networkInterfaces(false, includeLocal)
	if err == ErrNoInterface {
		addrs, err = networkInterfaces(true, includeLocal)
	}
	if err != nil {
		return nil, err
	}

	log.Debugf("Iterating network interfaces to find possible URLs for Flamenco Manager.")

	links := make([]*url.URL, 0)
	for _, addr := range addrs {
		var strAddr string
		if ipv4 := addr.To4(); ipv4 != nil {
			strAddr = ipv4.String()
		} else {
			strAddr = fmt.Sprintf("[%s]", addr)
		}

		link := fmt.Sprintf("%s://%s:%d/", schema, strAddr, portnum)
		myURL, err := url.Parse(link)
		if err != nil {
			log.Warningf("Skipping address %s, as it results in an unparseable URL %s: %s", addr, link, err)
			continue
		}
		links = append(links, myURL)
	}

	return links, nil
}

// Returns the URL based on the host & port for this HTTP request.
func ourURL(config *flamenco.Conf, r *http.Request) (*url.URL, error) {
	var scheme string
	if config.HasTLS() {
		scheme = "https"
	} else {
		scheme = "http"
	}
	// r.Host includes the port number.
	return url.Parse(fmt.Sprintf("%s://%s/", scheme, r.Host))
}

func urlConfigOptions(config *flamenco.Conf, r *http.Request) []URLConfigOptions {
	// Figure out the available URLs, and determine which one is configured right now.
	ownURLs, err := availableURLs(config, false)
	if err != nil {
		log.Errorf("Unable to find URLs to reach this Manager on: %s", err)
	}

	// Figure out which URL is now in use for this HTTP request.
	setupURL, err := ourURL(config, r)
	var setupURLString string
	if err != nil {
		log.Errorf("Unable to find URL currently in use by web config: %s", err)
	} else {
		setupURLString = setupURL.String()
	}

	urls := make([]URLConfigOptions, len(ownURLs))
	for idx, url := range ownURLs {
		stringURL := url.String()

		urls[idx].URL = stringURL
		urls[idx].IsCurrentInConfig = stringURL == config.OwnURL
		urls[idx].IsUsedForSetup = stringURL == setupURLString
	}

	return urls
}
