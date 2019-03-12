package shaman

/* ***** BEGIN MIT LICENSE BLOCK *****
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
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
 * ***** END MIT LICENCE BLOCK *****
 */

import "fmt"

var byteSizeSuffixes = []string{"B", "KiB", "MiB", "GiB", "TiB"}

func humanizeByteSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	roundedDown := float64(size)
	lastIndex := len(byteSizeSuffixes) - 1

	for index, suffix := range byteSizeSuffixes {
		if roundedDown > 1024.0 && index < lastIndex {
			roundedDown /= 1024.0
			continue
		}
		return fmt.Sprintf("%.1f %s", roundedDown, suffix)
	}

	// This line should never be reached, but at least in that
	// case we should at least return something correct.
	return fmt.Sprintf("%d B", size)
}
