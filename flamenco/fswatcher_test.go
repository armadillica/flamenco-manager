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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	auth "github.com/abbot/go-http-auth"
	"github.com/stretchr/testify/assert"

	check "gopkg.in/check.v1"
	"gopkg.in/jarcoal/httpmock.v1"
)

type LISQueueingTestSuite struct {
	lis *LatestImageSystem
}

var _ = check.Suite(&LISQueueingTestSuite{})

func (s *LISQueueingTestSuite) SetUpTest(c *check.C) {
	httpmock.Activate()

	// Construct a LatestImageSystem without any middleware, so we can test filling up the queue.
	s.lis = &LatestImageSystem{
		imageCreated: make(chan string, 3), // mimick imageQueueSize = 3
	}
}

func (s *LISQueueingTestSuite) TearDownTest(c *check.C) {
	s.lis.Close()
	httpmock.DeactivateAndReset()
}

func (s *LISQueueingTestSuite) outputProduced(t *check.C, path string) {
	respRec := httptest.NewRecorder()
	request, _ := http.NewRequest("POST", "/output-produced",
		strings.NewReader("{\"paths\":[\""+path+"\"]}"))
	ar := &auth.AuthenticatedRequest{Request: *request, Username: "unit-test-worker"}

	s.lis.outputProduced(respRec, ar)
	assert.Equal(t, respRec.Code, http.StatusNoContent)
}

func (s *LISQueueingTestSuite) TestImageQueueing(t *check.C) {
	// We should be able to call this more often than the queue size allows.
	for idx := 0; idx <= 7; idx++ {
		s.outputProduced(t, fmt.Sprintf("/path/to/img-%03d.jpg", idx))
	}

	// The queue should contain the first "imageQueueSize" items now.
	assert.Equal(t, "/path/to/img-000.jpg", <-s.lis.imageCreated)
	assert.Equal(t, "/path/to/img-001.jpg", <-s.lis.imageCreated)
	assert.Equal(t, "/path/to/img-002.jpg", <-s.lis.imageCreated)
	select {
	case filename := <-s.lis.imageCreated:
		assert.Fail(t, "not expecting queued image %q", filename)
	default:
		// the channel is empty, as we expected.
	}
}

func (s *LISQueueingTestSuite) TestOutputNoBody(t *check.C) {
	respRec := httptest.NewRecorder()
	request, _ := http.NewRequest("POST", "/output-produced", nil)
	ar := &auth.AuthenticatedRequest{Request: *request, Username: "unit-test-worker"}

	s.lis.outputProduced(respRec, ar)
	assert.Equal(t, respRec.Code, http.StatusBadRequest)
}
