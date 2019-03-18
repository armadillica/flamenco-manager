package flamenco

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

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/gorilla/mux"
	"github.com/kardianos/osext"
	log "github.com/sirupsen/logrus"
	"gopkg.in/mgo.v2/bson"
)

// IsoFormat is used for timestamp parsing
const IsoFormat = "2006-01-02T15:04:05-0700"

var (
	errMissingVariable   = errors.New("missing variable")
	errMalformedObjectID = errors.New("malformed Object ID")
)

// DecodeJSON decodes JSON from an io.Reader, and writes a Bad Request status if it fails.
func DecodeJSON(w http.ResponseWriter, r io.Reader, document interface{},
	logprefix string) error {
	dec := json.NewDecoder(r)

	if err := dec.Decode(document); err != nil {
		log.WithError(err).Warningf("%s Unable to decode JSON", logprefix)
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
		log.WithError(err).Errorf("%s: Unable to marshal JSON", logprefix)
		return err
	}

	logger := log.WithField("url", url.String())
	// TODO Sybren: enable GZip compression.
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(payloadBytes))
	if err != nil {
		logger.WithError(err).Errorf("%s: Unable to create request", logprefix)
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	if tweakrequest != nil {
		tweakrequest(req)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.WithError(err).Warningf("%s: Unable to POST", logprefix)
		return err
	}
	logger = logger.WithField("http_status", resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		logger.WithError(err).Warningf("%s: Error POSTing", logprefix)
		return err
	}

	if resp.StatusCode >= 300 {
		if resp.StatusCode != 404 {
			logger = logger.WithField("body", string(body))
		}
		logger.Warningf("%s: Error POSTing", logprefix)
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
	if _, err := os.Stat(fileToFind); err == nil {
		logger.Debug("Found templates in current working directory")
		return ""
	}

	// Find relative to this source file.
	_, myFilename, _, _ := runtime.Caller(0)
	topSourceDir := path.Dir(path.Dir(myFilename))
	if _, err := os.Stat(path.Join(topSourceDir, fileToFind)); err == nil {
		logger.Debug("Found templates in source directory")
		return topSourceDir
	}

	// Find relative to executable folder.
	exedirname, err := osext.ExecutableFolder()
	if err != nil {
		logger.WithError(err).Error("unable to determine the executable's directory")
		return ""
	}

	if _, err := os.Stat(filepath.Join(exedirname, fileToFind)); os.IsNotExist(err) {
		cwd, err := os.Getwd()
		if err != nil {
			logger.WithError(err).Error("unable to determine current working directory")
		}
		logger.WithFields(log.Fields{
			"cwd":        cwd,
			"exedirname": exedirname,
		}).Error("unable to find file")
		return ""
	}

	// Append a slash so that we can later just concatenate strings.
	log.WithField("exedirname", exedirname).Debug("found file")
	return exedirname + string(os.PathSeparator)
}

// ObjectIDFromRequest parses the request variable value as Object ID.
func ObjectIDFromRequest(w http.ResponseWriter, r *http.Request, variableName string) (bson.ObjectId, error) {
	vars := mux.Vars(r)
	taskID, found := vars[variableName]
	if !found {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "missing %s\n", variableName)
		return "", errors.New("missing variable")
	}

	if !bson.IsObjectIdHex(taskID) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Invalid ObjectID used for %s: %s\n", variableName, taskID)
		return "", errMalformedObjectID
	}

	return bson.ObjectIdHex(taskID), nil
}
