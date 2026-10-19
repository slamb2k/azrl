package cmd

import (
	"encoding/json"
	"io"

	"github.com/spf13/cobra"
)

// writeJSON emits v as indented JSON followed by a newline.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// jsonStdout reserves stdout for a --json document: it returns the command's
// current stdout and routes everything else the command prints (progress,
// notes, cmd.Printf) to stderr until restore is called, so `$(azrl … --json)`
// captures only the document.
func jsonStdout(cmd *cobra.Command) (stdout io.Writer, restore func()) {
	stdout = cmd.OutOrStdout()
	cmd.SetOut(cmd.ErrOrStderr())
	return stdout, func() { cmd.SetOut(nil) }
}

// captureResult is the --json document every provider's capture prints.
type captureResult struct {
	Provider string `json:"provider"`
	Profile  string `json:"profile"`
	Identity string `json:"identity,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Created  bool   `json:"created"`
}

// addJSONFlag registers the shared --json flag.
func addJSONFlag(c *cobra.Command, target *bool, what string) {
	c.Flags().BoolVar(target, "json", false, "Print "+what+" as JSON on stdout (progress goes to stderr)")
}

// loginResult is the --json document every provider's login prints once the
// sign-in has succeeded. Profile is empty for Azure's tenant-less sign-in
// into the default ~/.azure.
type loginResult struct {
	Provider string `json:"provider"`
	Profile  string `json:"profile,omitempty"`
	Identity string `json:"identity,omitempty"`
	Created  bool   `json:"created"`
}

// addLoginJSON gives a login command --json: the login body records which
// profile it signed in (and whether it created it) in res, and on success
// the wrapper prints res, filling the identity from the provider's
// disk-only Status (or Ambient, for a profile-less sign-in).
func addLoginJSON(c *cobra.Command, providerName string, res *loginResult) {
	var asJSON bool
	addJSONFlag(c, &asJSON, "the signed-in profile and identity")
	run := c.RunE
	c.RunE = func(cmd *cobra.Command, args []string) error {
		*res = loginResult{Provider: providerName}
		if !asJSON {
			return run(cmd, args)
		}
		stdout, restore := jsonStdout(cmd)
		defer restore()
		if err := run(cmd, args); err != nil {
			return err
		}
		if provs, err := selectProviders([]string{providerName}); err == nil {
			p := provs[0]
			if res.Profile != "" {
				if st, serr := p.Status(res.Profile, p.ProfilesDir()); serr == nil {
					res.Identity = st.Identity
				}
			} else if amb, aerr := p.Ambient(); aerr == nil {
				res.Identity = amb.Identity
			}
		}
		return writeJSON(stdout, res)
	}
}
