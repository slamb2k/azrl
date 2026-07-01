package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestGhrlRootPromotesGithubSubcommands(t *testing.T) {
	seedGhHome(t)
	root := GhrlRoot()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("ghrl list: %v", err)
	}
	if !strings.Contains(buf.String(), "work") || !strings.Contains(buf.String(), "github.com") {
		t.Fatalf("ghrl list output:\n%s", buf.String())
	}
}

func TestGhrlRootIncludesBrowserShim(t *testing.T) {
	root := GhrlRoot()
	var names []string
	for _, c := range root.Commands() {
		names = append(names, c.Name())
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"__browser", "login", "use", "switch", "rm", "status", "capture"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("ghrl root missing %q; has: %s", want, joined)
		}
	}
}
