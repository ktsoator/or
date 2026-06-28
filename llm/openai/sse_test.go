package openai

import (
	"io"
	"strings"
	"testing"
)

func TestSSEFilterDropsHeartbeats(t *testing.T) {
	// A MiMo-style stream: a ": PROCESSING" comment heartbeat (with its trailing
	// blank line) precedes the real data events. Without filtering, the blank
	// line after the comment surfaces as an empty event and breaks JSON parsing.
	input := strings.Join([]string{
		": PROCESSING\n",
		"\n",
		": PROCESSING\n",
		"\n",
		"data: {\"id\":\"1\"}\n",
		"\n",
		"data: [DONE]\n",
		"\n",
	}, "")

	got := readAllFiltered(t, input)

	want := strings.Join([]string{
		"data: {\"id\":\"1\"}\n",
		"\n",
		"data: [DONE]\n",
		"\n",
	}, "")
	if got != want {
		t.Fatalf("filtered stream mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestSSEFilterPreservesCompliantStream(t *testing.T) {
	// A well-behaved stream must pass through byte-for-byte, including multi-line
	// events and CRLF line endings.
	input := "event: message\r\ndata: {\"a\":1}\r\n\r\ndata: [DONE]\n\n"
	if got := readAllFiltered(t, input); got != input {
		t.Fatalf("compliant stream altered:\n got: %q\nwant: %q", got, input)
	}
}

// readAllFiltered runs input through the SSE filter using a tiny read buffer so
// the test also exercises partial reads across the buffered output.
func readAllFiltered(t *testing.T, input string) string {
	t.Helper()
	f := newSSEFilter(io.NopCloser(strings.NewReader(input)))
	defer f.Close()

	var out strings.Builder
	buf := make([]byte, 8)
	for {
		n, err := f.Read(buf)
		out.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read returned unexpected error: %v", err)
		}
	}
	return out.String()
}
