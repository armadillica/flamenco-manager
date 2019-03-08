package checkout

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

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

/* Checkout Definition files contain a line for each to-be-checked-out file.
 * Each line consists of three fields: checksum, file size, path in the checkout.
 */

// FileInvalidError is returned when there is an invalid line in a checkout definition file.
type FileInvalidError struct {
	lineNumber int // base-1 line number that's bad
	innerErr   error
	reason     string
}

func (cfie FileInvalidError) Error() string {
	return fmt.Sprintf("invalid line %d: %s", cfie.lineNumber, cfie.reason)
}

// DefinitionLine is a single line in a checkout definition file.
type DefinitionLine struct {
	Checksum string
	FileSize int64
	FilePath string
}

// DefinitionReader reads and parses a checkout definition
type DefinitionReader struct {
	ctx     context.Context
	channel chan *DefinitionLine
	reader  *bufio.Reader

	Err        error
	LineNumber int
}

var (
	// This is a wider range than used in SHA256 sums, but there is no harm in accepting a few more ASCII letters.
	validChecksumRegexp = regexp.MustCompile("^[a-zA-Z0-9]{16,}$")
)

// NewDefinitionReader creates a new DefinitionReader for the given reader.
func NewDefinitionReader(ctx context.Context, reader io.Reader) *DefinitionReader {
	return &DefinitionReader{
		ctx:     ctx,
		channel: make(chan *DefinitionLine),
		reader:  bufio.NewReader(reader),
	}
}

// Read spins up a new goroutine for parsing the checkout definition.
// The returned channel will receive definition lines.
func (fr *DefinitionReader) Read() <-chan *DefinitionLine {
	go func() {
		defer close(fr.channel)
		defer logrus.Debug("done reading request")

		for {
			line, err := fr.reader.ReadString('\n')
			if err != nil && err != io.EOF {
				fr.Err = FileInvalidError{
					lineNumber: fr.LineNumber,
					innerErr:   err,
					reason:     fmt.Sprintf("I/O error: %v", err),
				}
				return
			}
			if err == io.EOF && line == "" {
				return
			}

			if contextError := fr.ctx.Err(); contextError != nil {
				fr.Err = fr.ctx.Err()
				return
			}

			fr.LineNumber++
			logrus.WithFields(logrus.Fields{
				"line":   line,
				"number": fr.LineNumber,
			}).Debug("read line")

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			definitionLine, err := fr.parseLine(line)
			if err != nil {
				fr.Err = err
				return
			}

			fr.channel <- definitionLine
		}
	}()

	return fr.channel
}

func (fr *DefinitionReader) parseLine(line string) (*DefinitionLine, error) {

	parts := strings.SplitN(strings.TrimSpace(line), " ", 3)
	if len(parts) != 3 {
		return nil, FileInvalidError{
			lineNumber: fr.LineNumber,
			reason: fmt.Sprintf("line should consist of three space-separated parts, not %d: %v",
				len(parts), line),
		}
	}

	checksum := parts[0]
	if !validChecksumRegexp.MatchString(checksum) {
		return nil, FileInvalidError{fr.LineNumber, nil, "invalid checksum"}
	}

	fileSize, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, FileInvalidError{fr.LineNumber, err, "invalid file size"}
	}

	filePath := strings.TrimSpace(parts[2])
	if path.IsAbs(filePath) {
		return nil, FileInvalidError{fr.LineNumber, err, "no absolute paths allowed"}
	}
	if filePath != path.Clean(filePath) || strings.Contains(filePath, "..") {
		return nil, FileInvalidError{fr.LineNumber, err, "paths must be clean and not have any .. in them."}
	}

	return &DefinitionLine{
		Checksum: parts[0],
		FileSize: fileSize,
		FilePath: filePath,
	}, nil
}
