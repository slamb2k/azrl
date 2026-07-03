package github

import (
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

// chunkReader yields the input in small pieces, including a trailing prompt
// with no newline — the forwarder must not line-buffer.
type chunkReader struct {
	parts []string
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.parts) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.parts[0])
	c.parts[0] = c.parts[0][n:]
	if c.parts[0] == "" {
		c.parts = c.parts[1:]
	}
	return n, nil
}

func TestForwardAndCopyCode(t *testing.T) {
	r := &chunkReader{parts: []string{
		"! First copy your one-time c",
		"ode: ABCD-1234\n",
		"Press Enter to open github.com in your browser... ",
	}}
	var out strings.Builder
	forwardAndCopyCode(r, &out)
	s := out.String()
	// Raw output forwarded intact, including the newline-less prompt.
	if !strings.Contains(s, "one-time code: ABCD-1234") ||
		!strings.Contains(s, "Press Enter to open github.com in your browser... ") {
		t.Fatalf("output not forwarded verbatim:\n%q", s)
	}
	// The OSC 52 write carries the base64 of the code, exactly once.
	osc := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("ABCD-1234")) + "\x07"
	if strings.Count(s, osc) != 1 {
		t.Fatalf("expected exactly one OSC 52 write:\n%q", s)
	}
	if !strings.Contains(s, "copied to your local clipboard") {
		t.Fatalf("missing confirmation note:\n%q", s)
	}
}

func TestForwardAndCopyCodeNoMatchIsQuiet(t *testing.T) {
	var out strings.Builder
	forwardAndCopyCode(&chunkReader{parts: []string{"no codes here\n"}}, &out)
	if strings.Contains(out.String(), "\x1b]52") {
		t.Fatalf("no code should mean no clipboard write: %q", out.String())
	}
}
