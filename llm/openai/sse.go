package openai

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3/option"
)

// sseHeartbeatFilter is a request middleware that sanitizes server-sent-event
// responses so non-compliant keep-alive lines do not break the SDK's decoder.
//
// Some OpenAI-compatible providers (e.g. Xiaomi MiMo) emit comment lines like
// ": PROCESSING" while the model is still thinking. The SDK's SSE decoder skips
// the comment itself but still dispatches the trailing blank line as an event
// with empty data, which then fails JSON parsing with "unexpected end of JSON
// input". The filter drops such heartbeats and the empty events they would
// otherwise produce, leaving compliant streams untouched.
func sseHeartbeatFilter(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	resp, err := next(req)
	if err != nil || resp == nil {
		return resp, err
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		resp.Body = newSSEFilter(resp.Body)
	}
	return resp, nil
}

// sseFilter rewrites a server-sent-events stream so comment/heartbeat lines are
// dropped and the empty events they would otherwise produce are suppressed.
type sseFilter struct {
	src      *bufio.Reader
	closer   io.Closer
	out      bytes.Buffer
	sawField bool // a field line buffered since the last dispatched blank line
	eof      bool
	err      error
}

func newSSEFilter(body io.ReadCloser) *sseFilter {
	return &sseFilter{src: bufio.NewReader(body), closer: body}
}

func (f *sseFilter) Read(p []byte) (int, error) {
	for f.out.Len() == 0 && !f.eof {
		line, err := f.src.ReadBytes('\n')
		if len(line) > 0 {
			f.process(line)
		}
		if err != nil {
			f.eof = true
			if err != io.EOF {
				f.err = err
			}
		}
	}
	if f.out.Len() == 0 {
		if f.err != nil {
			return 0, f.err
		}
		return 0, io.EOF
	}
	return f.out.Read(p)
}

func (f *sseFilter) process(line []byte) {
	switch trimmed := bytes.TrimRight(line, "\r\n"); {
	case len(trimmed) == 0:
		// Blank line = event boundary. Only forward it when a field line was
		// buffered, so a comment-only heartbeat never becomes an empty event.
		if f.sawField {
			f.out.Write(line)
			f.sawField = false
		}
	case trimmed[0] == ':':
		// Comment / keep-alive line: drop it.
	default:
		f.sawField = true
		f.out.Write(line)
	}
}

func (f *sseFilter) Close() error { return f.closer.Close() }
