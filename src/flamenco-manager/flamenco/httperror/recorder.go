// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Altered by Sybren Stüvel for Flamenco.

package httperror

import (
	"errors"
	"net/http"
)

// FailingResponseRecorder is an implementation of http.ResponseWriter that
// causes an error when the body is being written. Headers are ok.
type FailingResponseRecorder struct {
	// Code is the HTTP response code set by WriteHeader.
	//
	// Note that if a Handler never calls WriteHeader or Write,
	// this might end up being 0, rather than the implicit
	// http.StatusOK. To get the implicit value, use the Result
	// method.
	Code int

	// HeaderMap contains the headers explicitly set by the Handler.
	//
	// To get the implicit headers set by the server (such as
	// automatic Content-Type), use the Result method.
	HeaderMap http.Header

	// Flushed is whether the Handler called Flush.
	Flushed     bool
	wroteHeader bool
}

// NewRecorder returns an initialized FailingResponseRecorder.
func NewFailingRecorder() *FailingResponseRecorder {
	return &FailingResponseRecorder{
		HeaderMap: make(http.Header),
		Code:      200,
	}
}

// DefaultRemoteAddr is the default remote address to return in RemoteAddr if
// an explicit DefaultRemoteAddr isn't set on FailingResponseRecorder.
const DefaultRemoteAddr = "1.2.3.4"

// Header returns the response headers.
func (rw *FailingResponseRecorder) Header() http.Header {
	m := rw.HeaderMap
	if m == nil {
		m = make(http.Header)
		rw.HeaderMap = m
	}
	return m
}

// writeHeader writes a header if it was not written yet and
// detects Content-Type if needed.
//
// bytes or str are the beginning of the response body.
// We pass both to avoid unnecessarily generate garbage
// in rw.WriteString which was created for performance reasons.
// Non-nil bytes win.
func (rw *FailingResponseRecorder) writeHeader(b []byte, str string) {
	if rw.wroteHeader {
		return
	}
	if len(str) > 512 {
		str = str[:512]
	}

	m := rw.Header()

	_, hasType := m["Content-Type"]
	hasTE := m.Get("Transfer-Encoding") != ""
	if !hasType && !hasTE {
		if b == nil {
			b = []byte(str)
		}
		m.Set("Content-Type", http.DetectContentType(b))
	}

	rw.WriteHeader(200)
}

// Write always errors out.
func (rw *FailingResponseRecorder) Write(buf []byte) (int, error) {
	return 0, errors.New("unable to write body")
}

// WriteString always errors after writing the header.
func (rw *FailingResponseRecorder) WriteString(str string) (int, error) {
	rw.writeHeader(nil, str)

	return 0, errors.New("unable to write body")
}

// WriteHeader sets rw.Code. After it is called, changing rw.Header
// will not affect rw.HeaderMap.
func (rw *FailingResponseRecorder) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.Code = code
	rw.wroteHeader = true
	if rw.HeaderMap == nil {
		rw.HeaderMap = make(http.Header)
	}
}

// Flush sets rw.Flushed to true.
func (rw *FailingResponseRecorder) Flush() {
	if !rw.wroteHeader {
		rw.WriteHeader(200)
	}
	rw.Flushed = true
}
