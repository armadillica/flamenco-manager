package flamenco

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/kardianos/osext"
	log "github.com/sirupsen/logrus"
)

// IsoFormat is used for timestamp parsing
const IsoFormat = "2006-01-02T15:04:05-0700"

// DecodeJSON decodes JSON from an io.Reader, and writes a Bad Request status if it fails.
func DecodeJSON(w http.ResponseWriter, r io.Reader, document interface{},
	logprefix string) error {
	dec := json.NewDecoder(r)

	if err := dec.Decode(document); err != nil {
		log.Warningf("%s Unable to decode JSON: %s", logprefix, err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Unable to decode JSON: %s\n", err)
		return err
	}

	return nil
}

// SendJSON sends a JSON document to some URL via HTTP.
// :param tweakrequest: can be used to tweak the request before sending it, for
//    example by adding authentication headers. May be nil.
// :param responsehandler: is called when a non-error response has been read.
//    May be nil.
func SendJSON(logprefix, method string, url *url.URL,
	payload interface{},
	tweakrequest func(req *http.Request),
	responsehandler func(resp *http.Response, body []byte) error,
) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("%s: Unable to marshal JSON: %s", logprefix, err)
		return err
	}

	// TODO Sybren: enable GZip compression.
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Errorf("%s: Unable to create request: %s", logprefix, err)
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	if tweakrequest != nil {
		tweakrequest(req)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Warningf("%s: Unable to POST to %s: %s", logprefix, url, err)
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		log.Warningf("%s: Error %d POSTing to %s: %s",
			logprefix, resp.StatusCode, url, err)
		return err
	}

	if resp.StatusCode >= 300 {
		suffix := ""
		if resp.StatusCode != 404 {
			suffix = fmt.Sprintf("\n    body:\n%s", body)
		}
		log.Warningf("%s: Error %d POSTing to %s%s",
			logprefix, resp.StatusCode, url, suffix)
		return fmt.Errorf("%s: Error %d POSTing to %s", logprefix, resp.StatusCode, url)
	}

	if responsehandler != nil {
		return responsehandler(resp, body)
	}

	return nil
}

// TemplatePathPrefix returns the filename prefix to find template files.
// Templates are searched for relative to the current working directory as well as relative
// to the currently running executable.
func TemplatePathPrefix(fileToFind string) string {
	logger := log.WithField("file_to_find", fileToFind)

	// Find as relative path, i.e. relative to CWD.
	_, err := os.Stat(fileToFind)
	if err == nil {
		logger.Debug("Found templates in current working directory")
		return ""
	}

	// Find relative to executable folder.
	exedirname, err := osext.ExecutableFolder()
	if err != nil {
		logger.WithError(err).Fatal("unable to determine the executable's directory")
	}

	if _, err := os.Stat(filepath.Join(exedirname, fileToFind)); os.IsNotExist(err) {
		cwd, err := os.Getwd()
		if err != nil {
			logger.WithError(err).Fatal("unable to determine current working directory")
		}
		logger.WithFields(log.Fields{
			"cwd":        cwd,
			"exedirname": exedirname,
		}).Fatal("unable to find file")
	}

	// Append a slash so that we can later just concatenate strings.
	log.WithField("exedirname", exedirname).Debug("found file")
	return exedirname + string(os.PathSeparator)
}
