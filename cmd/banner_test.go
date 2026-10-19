package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runBanner captures what showRunBanner writes for root + args.
func runBanner(t *testing.T, root *cobra.Command, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	old := bannerOut
	bannerOut = &buf
	t.Cleanup(func() { bannerOut = old })
	showRunBanner(root, args)
	return buf.String()
}

func TestRunBannerShownOnPlainCommands(t *testing.T) {
	t.Setenv("AZRL_NO_BANNER", "")
	for _, args := range [][]string{{"whoami"}, {"list", "--json"}, {"gh", "login", "work"}, {"az", "env", "work"}, {"env", "work"}, {"login", "work"}} {
		out := runBanner(t, RootCmd, args...)
		if !strings.Contains(out, "╚═╝") {
			t.Fatalf("azrl %v: banner missing on stderr; got %q", args, out)
		}
		if strings.Contains(out, "\x1b[") {
			t.Fatalf("azrl %v: non-TTY stderr must get plain glyphs", args)
		}
	}
	if out := runBanner(t, GhrlRoot(), "list"); !strings.Contains(out, "╚═╝") {
		t.Fatalf("ghrl list: banner missing; got %q", out)
	}
}

func TestRunBannerSkipped(t *testing.T) {
	t.Setenv("AZRL_NO_BANNER", "")
	for _, args := range [][]string{
		{},                                 // bare: the TUI draws its own crest
		{"--version"},                      // root flags only
		{"__browser", "https://x"},         // hidden $BROWSER shim
		{"__browser-capture", "https://x"}, // hidden capture shim
		{"__complete", "gh", ""},           // shell completion
		{"completion", "bash"},             // completion script
		{"help", "login"},                  // help
		{"no-such-verb"},                   // unknown: cobra's error stands alone
	} {
		if out := runBanner(t, RootCmd, args...); out != "" {
			t.Fatalf("azrl %v: banner must be skipped; got %q", args, out)
		}
	}
	t.Setenv("AZRL_NO_BANNER", "1")
	if out := runBanner(t, RootCmd, "whoami"); out != "" {
		t.Fatalf("AZRL_NO_BANNER must silence the banner; got %q", out)
	}
}
