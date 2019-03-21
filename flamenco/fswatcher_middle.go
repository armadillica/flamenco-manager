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

package flamenco

import (
	"bytes"
	"os/exec"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

// ConvertAndForward copies each image it reads from 'images', converts it to a browser-
// friendly file, and forwards the new filename to the returned channel. It always converts
// to JPEG, even when the file is a browser-supported format (like PNG), so that the HTML
// can always refer to /static/latest-image.jpg to show the latest render.
func ConvertAndForward(images <-chan string, storagePath string) <-chan string {
	output := make(chan string)

	go func() {
		var outname string

		for path := range images {
			outname = filepath.Join(storagePath, "latest-image.jpg")

			log.Infof("ConvertAndForward: Converting %s to %s", path, outname)
			cmd := exec.Command("convert", path,
				"-quality", "85",
				"-resize", "1920x1080>", // convert to 2MPixels max, but never enlarge.
				outname)

			var out bytes.Buffer
			cmd.Stdout = &out

			if err := cmd.Run(); err != nil {
				log.Errorf("ConvertAndForward: error converting %s: %s", path, err)
				log.Errorf("ConvertAndForward: conversion output: %s", out.String())
				continue
			}

			output <- outname
		}
	}()

	return output
}
