package httpserver

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/armadillica/flamenco-manager/flamenco"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/acme/autocert"
)

// Constants for the HTTP servers.
const (
	ReadHeaderTimeout = 15 * time.Second
	ReadTimeout       = 600 * time.Second
)

// Server acts as a http.Server
type Server interface {
	Shutdown(ctx context.Context)
	ListenAndServe() error
	Done() <-chan struct{}
}

// Combined has one or two HTTP servers.
type Combined struct {
	httpServer  *http.Server
	httpsServer *http.Server

	tlsKey  string
	tlsCert string

	expectShutdown   bool
	mutex            sync.Mutex
	shutdownComplete chan struct{} // closed when ListenAndServe stops serving.
}

// New returns a new HTTP server that can handle HTTP, HTTPS, and both for ACME.
func New(config flamenco.Conf, handler http.Handler) Server {
	server := Combined{
		mutex:            sync.Mutex{},
		shutdownComplete: make(chan struct{}),
	}

	switch {
	case config.HasCustomTLS():
		logrus.WithFields(logrus.Fields{
			"tlscert":      config.TLSCert,
			"tlskey":       config.TLSKey,
			"listen_https": config.ListenHTTPS,
		}).Info("creating HTTPS-enabled server")
		server.httpsServer = &http.Server{
			Addr:              config.ListenHTTPS,
			Handler:           handler,
			ReadTimeout:       ReadTimeout,
			ReadHeaderTimeout: ReadHeaderTimeout,
		}
		server.tlsKey = config.TLSKey
		server.tlsCert = config.TLSCert

	case config.ACMEDomainName != "":
		logrus.WithFields(logrus.Fields{
			"acme_domain_name": config.ACMEDomainName,
			"listen":           config.Listen,
			"listen_https":     config.ListenHTTPS,
		}).Info("creating ACME/Let's Encrypt enabled server")
		mgr := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(config.ACMEDomainName),
			Cache:      autocert.DirCache("tlscerts"),
		}

		server.httpServer = &http.Server{
			Addr:    config.Listen,
			Handler: mgr.HTTPHandler(nil),
		}

		server.httpsServer = &http.Server{
			Addr:              config.ListenHTTPS,
			Handler:           handler,
			ReadTimeout:       ReadTimeout,
			ReadHeaderTimeout: ReadHeaderTimeout,
			TLSConfig:         mgr.TLSConfig(),
		}

	default:
		logrus.WithField("listen", config.Listen).Info("creating insecure server")
		server.httpServer = &http.Server{
			Addr:              config.Listen,
			Handler:           handler,
			ReadTimeout:       ReadTimeout,
			ReadHeaderTimeout: ReadHeaderTimeout,
		}
	}

	return &server
}

// Shutdown shuts down both HTTP and HTTPS server.
func (s *Combined) Shutdown(ctx context.Context) {
	s.mutex.Lock()
	s.expectShutdown = true
	s.mutex.Unlock()

	if s.httpServer != nil {
		s.httpServer.Shutdown(ctx)
	}
	if s.httpsServer != nil {
		s.httpsServer.Shutdown(ctx)
	}
}

// Done returns a channel that is closed when the server is done serving.
func (s *Combined) Done() <-chan struct{} {
	return s.shutdownComplete
}

func (s *Combined) mustBeFresh() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.expectShutdown {
		panic("this HTTP server was already shut down, unable to restart")
	}
}

// ListenAndServe listens on both HTTP and HTTPS servers.
func (s *Combined) ListenAndServe() error {
	s.mustBeFresh()
	defer close(s.shutdownComplete)

	var httpError, httpsError error
	wg := sync.WaitGroup{}

	if s.httpServer != nil {
		wg.Add(1)
		go func() {
			logger := logrus.WithField("listen", s.httpServer.Addr)
			logger.Debug("starting HTTP server")
			err := s.httpServer.ListenAndServe()

			s.mutex.Lock()
			defer s.mutex.Unlock()

			if !s.expectShutdown {
				logger.WithError(httpError).Error("HTTP server unexpectedly stopped")
				httpError = err
			}
			wg.Done()
		}()
	}

	if s.httpsServer != nil {
		wg.Add(1)
		go func() {
			logger := logrus.WithField("listen_https", s.httpsServer.Addr)
			logger.Debug("starting HTTPS server")
			err := s.httpsServer.ListenAndServeTLS(s.tlsCert, s.tlsKey)

			s.mutex.Lock()
			defer s.mutex.Unlock()

			if !s.expectShutdown {
				logger.WithError(err).Error("HTTPS server unexpectedly stopped")
				httpsError = err
			}
			wg.Done()
		}()
	}

	wg.Wait()

	// We can only return one error.
	if httpsError != nil {
		return httpsError
	}
	return httpError
}
