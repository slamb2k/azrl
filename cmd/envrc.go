package cmd

import (
	"fmt"
	"io"

	"github.com/slamb2k/azrl/internal/profile"
)

// offerEnvrc offers to write a direnv .envrc so plain `az` in pwd follows the
// profile. It skips silently when one already exists; mode decides whether it
// asks (a closed stdin reads as a decline), writes, or stays quiet.
func offerEnvrc(pwd string, mode envrcMode, out io.Writer, in io.Reader) {
	// .envrc must live beside the .azprofile it reads, which may be an ancestor.
	dir := pwd
	if d, ok := profile.LocateAzprofile(pwd); ok {
		dir = d
	}
	if profile.HasEnvrc(dir) {
		return
	}
	if !envrcConsent(mode, "azrl: also write .envrc so `az` in this dir follows this profile?", out, in) {
		return
	}
	wrote, err := profile.WriteEnvrc(dir)
	if err != nil {
		fmt.Fprintf(out, "azrl: could not write .envrc: %v\n", err)
		return
	}
	if !wrote {
		return
	}
	if ran, aerr := profile.DirenvAllow(dir); ran && aerr == nil {
		fmt.Fprintf(out, "azrl: wrote %s and ran `direnv allow` — `az` follows this profile from now on\n", profile.EnvrcPath(dir))
	} else {
		fmt.Fprintf(out, "azrl: wrote %s — run `direnv allow` (or export AZURE_CONFIG_DIR) to activate\n", profile.EnvrcPath(dir))
	}
}
