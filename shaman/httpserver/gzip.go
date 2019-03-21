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

package httpserver

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
)

// Errors returned by DecompressedReader
var (
	ErrContentEncodingNotSupported = errors.New("Content-Encoding not supported")
)

// wrapperCloserReader is a ReadCloser that closes both a wrapper and the wrapped reader.
type wrapperCloserReader struct {
	wrapped io.ReadCloser
	wrapper io.ReadCloser
}

func (cr *wrapperCloserReader) Close() error {
	errWrapped := cr.wrapped.Close()
	errWrapper := cr.wrapper.Close()

	if errWrapped != nil {
		return errWrapped
	}
	return errWrapper
}

func (cr *wrapperCloserReader) Read(p []byte) (n int, err error) {
	return cr.wrapper.Read(p)
}

// DecompressedReader returns a reader that decompresses the body.
// The compression scheme is determined by the Content-Encoding header.
// Closing the returned reader is the caller's responsibility.
func DecompressedReader(request *http.Request) (io.ReadCloser, error) {
	var wrapper io.ReadCloser
	var err error

	switch request.Header.Get("Content-Encoding") {
	case "gzip":
		wrapper, err = gzip.NewReader(request.Body)
	case "identity", "":
		return request.Body, nil
	default:
		return nil, ErrContentEncodingNotSupported
	}

	return &wrapperCloserReader{
		wrapped: request.Body,
		wrapper: wrapper,
	}, err
}

// CompressBuffer GZip-compresses the payload into a buffer, and returns it.
func CompressBuffer(payload []byte) *bytes.Buffer {
	var bodyBuf bytes.Buffer
	compressor := gzip.NewWriter(&bodyBuf)
	compressor.Write(payload)
	compressor.Close()
	return &bodyBuf
}
