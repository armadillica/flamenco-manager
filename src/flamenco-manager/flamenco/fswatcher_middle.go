package flamenco

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"

	"flamenco-manager/flamenco/filetools"

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
			basename := filepath.Base(path)
			ext := strings.ToLower(filepath.Ext(basename))
			outname = filepath.Join(storagePath, "latest-image.jpg")

			if ext == ".jpg" {
				log.Infof("ConvertAndForward: Copying %s to %s", path, outname)
				filetools.CopyFile(path, outname)
			} else {
				log.Infof("ConvertAndForward: Converting %s to %s", path, outname)
				cmd := exec.Command("convert", path, "-quality", "85", outname)

				var out bytes.Buffer
				cmd.Stdout = &out

				if err := cmd.Run(); err != nil {
					log.Errorf("ConvertAndForward: error converting %s: %s", path, err)
					log.Errorf("ConvertAndForward: conversion output: %s", out.String())
					continue
				}

			}

			output <- outname
		}
	}()

	return output
}
