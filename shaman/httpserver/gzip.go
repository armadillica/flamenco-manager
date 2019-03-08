package httpserver

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
)

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
