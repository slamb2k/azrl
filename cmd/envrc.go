package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/slamb2k/azrl/internal/profile"
)

// offerEnvrc offers to write a direnv .envrc so plain `az` in pwd follows the
// profile. It skips silently when one already exists, and a closed/non-tty
// stdin reads as a decline (never hangs).
func offerEnvrc(pwd string, out io.Writer, in io.Reader) {
	if profile.HasEnvrc(pwd) {
		return
	}
	fmt.Fprint(out, "azrl: also write .envrc so `az` in this dir follows this profile? [y/N] ")
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		fmt.Fprintln(out)
		return
	}
	if ans := strings.TrimSpace(sc.Text()); !strings.HasPrefix(strings.ToLower(ans), "y") {
		return
	}
	wrote, err := profile.WriteEnvrc(pwd)
	if err != nil {
		fmt.Fprintf(out, "azrl: could not write .envrc: %v\n", err)
		return
	}
	if wrote {
		fmt.Fprintf(out, "azrl: wrote %s — run `direnv allow`\n", profile.EnvrcPath(pwd))
	}
}
