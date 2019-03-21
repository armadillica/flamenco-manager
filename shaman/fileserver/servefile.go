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

package fileserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"

	"github.com/armadillica/flamenco-manager/shaman/filestore"
)

// serveFile only serves stored files (not 'uploading' or 'checking')
func (fs *FileServer) serveFile(ctx context.Context, w http.ResponseWriter, checksum string, filesize int64) {
	path, status := fs.fileStore.ResolveFile(checksum, filesize, filestore.ResolveStoredOnly)
	if status != filestore.StatusStored {
		http.Error(w, "File Not Found", http.StatusNotFound)
		return
	}

	logger := packageLogger.WithField("path", path)

	stat, err := os.Stat(path)
	if err != nil {
		logger.WithError(err).Error("unable to stat file")
		http.Error(w, "File Not Found", http.StatusNotFound)
		return
	}
	if stat.Size() != filesize {
		logger.WithFields(logrus.Fields{
			"realSize":     stat.Size(),
			"expectedSize": filesize,
		}).Error("file size in storage is corrupt")
		http.Error(w, "File Size Incorrect", http.StatusInternalServerError)
		return
	}

	infile, err := os.Open(path)
	if err != nil {
		logger.WithError(err).Error("unable to read file")
		http.Error(w, "File Not Found", http.StatusNotFound)
		return
	}

	filesizeStr := strconv.FormatInt(filesize, 10)
	w.Header().Set("Content-Type", "application/binary")
	w.Header().Set("Content-Length", filesizeStr)
	w.Header().Set("ETag", fmt.Sprintf("'%s-%s'", checksum, filesizeStr))
	w.Header().Set("X-Shaman-Checksum", checksum)

	written, err := io.Copy(w, infile)
	if err != nil {
		logger.WithError(err).Error("unable to copy file to writer")
		// Anything could have been sent by now, so just close the connection.
		return
	}
	logger.WithField("written", written).Debug("file send to writer")
}
