package cmd

import (
	"bufio"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// envrcMode is how a mapping verb treats the direnv .envrc offer.
type envrcMode int

const (
	envrcAsk envrcMode = iota // prompt [y/N] on the given reader
	envrcYes                  // write it without asking
	envrcNo                   // skip silently
)

// envrcFlags is the --envrc/--no-envrc pair every verb that maps a directory
// carries, so an unattended caller decides up front instead of answering a
// prompt.
type envrcFlags struct{ yes, no bool }

func addEnvrcFlags(c *cobra.Command) *envrcFlags {
	f := &envrcFlags{}
	c.Flags().BoolVar(&f.yes, "envrc", false, "Also write a direnv .envrc so the plain CLI in this dir follows the profile (no prompt)")
	c.Flags().BoolVar(&f.no, "no-envrc", false, "Never offer the .envrc")
	c.MarkFlagsMutuallyExclusive("envrc", "no-envrc")
	return f
}

// mode resolves the flags: an explicit flag wins; otherwise ask on a TTY and
// skip without one (a non-TTY caller is never prompted).
func (f *envrcFlags) mode() envrcMode {
	switch {
	case f.yes:
		return envrcYes
	case f.no:
		return envrcNo
	case isInteractive():
		return envrcAsk
	default:
		return envrcNo
	}
}

// readYes reads one line and reports whether it starts with y/Y. EOF is no.
func readYes(in io.Reader) bool {
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(sc.Text())), "y")
}

// envrcConsent applies mode to one offer: ask prints the question and reads
// the answer from in.
func envrcConsent(mode envrcMode, question string, out io.Writer, in io.Reader) bool {
	switch mode {
	case envrcYes:
		return true
	case envrcNo:
		return false
	}
	io.WriteString(out, question+" [y/N] ")
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		io.WriteString(out, "\n")
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(sc.Text())), "y")
}
