package cmd

import (
	"fmt"
	"io"

	"github.com/slamb2k/azrl/internal/profile"
)

// offerEnvrc is the Azure .envrc offer: the file must live beside the
// .azprofile it reads (which may be an ancestor), and an existing .envrc is
// left alone without asking.
func offerEnvrc(pwd string, mode envrcMode, out io.Writer, in io.Reader) {
	dir := pwd
	if d, ok := profile.LocateAzprofile(pwd); ok {
		dir = d
	}
	if profile.HasEnvrc(dir) {
		return
	}
	offerEnvrcIn("azrl", "az", dir, func() (bool, error) { return profile.WriteEnvrc(dir) }, mode, out, in)
}

// offerEnvrcIn is the one .envrc offer every provider uses: mode decides
// whether it asks (a closed stdin reads as a decline), writes, or stays
// quiet; after writing it runs `direnv allow` when direnv is installed.
func offerEnvrcIn(tool, cli, dir string, write func() (bool, error), mode envrcMode, out io.Writer, in io.Reader) {
	if !envrcConsent(mode, fmt.Sprintf("%s: also write .envrc so `%s` in this dir follows this profile?", tool, cli), out, in) {
		return
	}
	wrote, err := write()
	if err != nil {
		fmt.Fprintf(out, "%s: could not write .envrc: %v\n", tool, err)
		return
	}
	if !wrote {
		return
	}
	path := profile.EnvrcPath(dir)
	if ran, aerr := profile.DirenvAllow(dir); ran && aerr == nil {
		fmt.Fprintf(out, "%s: wrote %s and ran `direnv allow` — `%s` follows this profile from now on\n", tool, path, cli)
	} else {
		fmt.Fprintf(out, "%s: wrote %s — run `direnv allow` to activate\n", tool, path)
	}
}
