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
	"net/http"

	"github.com/armadillica/flamenco-manager/shaman/filestore"
)

var responseForStatus = map[filestore.FileStatus]int{
	filestore.StatusUploading:    420, // Enhance Your Calm
	filestore.StatusStored:       http.StatusOK,
	filestore.StatusDoesNotExist: http.StatusNotFound,
}

func (fs *FileServer) checkFile(ctx context.Context, w http.ResponseWriter, checksum string, filesize int64) {
	_, status := fs.fileStore.ResolveFile(checksum, filesize, filestore.ResolveEverything)
	code, ok := responseForStatus[status]
	if !ok {
		packageLogger.WithField("fileStoreStatus", status).Error("no HTTP status code implemented")
		code = http.StatusInternalServerError
	}
	w.WriteHeader(code)
}
