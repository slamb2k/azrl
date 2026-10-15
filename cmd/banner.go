package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/ui"
)

// bannerOut is where the run banner lands — always stderr, so stdout stays
// clean for --json, env's shell code, and anything a script captures.
var bannerOut io.Writer = os.Stderr

// showRunBanner prints the AZRL crest on stderr when azrl runs as a plain
// command (from a script or a shell). Skipped for bare invocation (the TUI
// draws its own), hidden plumbing ($BROWSER shims, shell completion), and
// when AZRL_NO_BANNER is set. Colour follows stderr: a terminal gets the
// gradient crest, a redirected stderr gets the plain glyphs.
func showRunBanner(root *cobra.Command, args []string) {
	if os.Getenv("AZRL_NO_BANNER") != "" {
		return
	}
	target, _, err := root.Find(args)
	if err != nil || target == root {
		return
	}
	for c := target; c != nil; c = c.Parent() {
		if c.Hidden || c.Name() == "completion" || c.Name() == "help" {
			return
		}
	}
	width := 0
	tty := term.IsTerminal(os.Stderr.Fd())
	if tty {
		ensureStderrColour()
		if w, _, err := term.GetSize(os.Stderr.Fd()); err == nil {
			width = w
		}
	}
	b := ui.BannerForWidth(width)
	if !tty {
		b = ansi.Strip(b)
	}
	fmt.Fprintln(bannerOut, b)
}
