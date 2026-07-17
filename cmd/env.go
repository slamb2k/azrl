package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/provider"
)

// newEnvCmd builds `env [name]` for one provider: it prints export lines for
// the profile's environment (the same set `shell` gives a subshell) so
// `eval "$(azrl env <name>)"` makes the CURRENT shell act as the profile —
// no subshell, no mapping, nothing on disk. `--off` prints the matching
// unsets. All interactive output (picker, notes) goes to stderr so command
// substitution captures only shell code.
func newEnvCmd(provFn func() provider.Provider, label, providerName string, valid func(string) error) *cobra.Command {
	var off bool
	c := &cobra.Command{
		Use:          "env [name]",
		Short:        "Print exports so the current shell acts as a profile (eval it; --off reverts)",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()
			if off {
				fmt.Fprintf(out, "unset %s\n", strings.Join(shellOwnedKeys(providerName), " "))
				fmt.Fprintln(errOut, cliDim.Render(fmt.Sprintf("%s: shell override cleared — back to the mapping/ambient resolution", label)))
				return nil
			}
			prov := provFn()
			name := ""
			if len(args) == 1 {
				if err := valid(args[0]); err != nil {
					return err
				}
				name = args[0]
			} else {
				if profs, err := prov.ListProfiles(prov.ProfilesDir()); err != nil || len(profs) == 0 {
					return fmt.Errorf("%s: no profiles yet — run '%s login <name>' first", label, label)
				}
				// Route the picker to stderr: stdout must stay pure shell code
				// for eval. Restored before the exports are printed.
				cmd.SetOut(errOut)
				picked, _, err := resolveLoginTarget(cmd, prov, args, label, valid)
				cmd.SetOut(out)
				if err != nil {
					return err
				}
				name = picked
			}
			env, err := shellEnv(providerName, name)
			if err != nil {
				return err
			}
			for _, kv := range env {
				k, v, _ := strings.Cut(kv, "=")
				fmt.Fprintf(out, "export %s=%s\n", k, shellQuote(v))
			}
			fmt.Fprintln(errOut, cliAccentBlue.Render(fmt.Sprintf("%s: this shell now acts as %s:%s", label, providerName, name))+cliDim.Render(fmt.Sprintf(" — '%s env --off' reverts", label)))
			return nil
		},
	}
	c.Flags().BoolVar(&off, "off", false, "Print unsets that clear the override from the current shell")
	return c
}

// shellQuote single-quotes v for POSIX shells (' → '\” inside).
func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// printApplyHint notes, after a sign-in, when the profile does NOT govern the
// cwd: login only fills the profile's token container (PAT-002 — the ambient
// default is never touched), so plain CLIs in this shell still act as the
// ambient identity until an applier is used. Self-suppressing when the
// profile already governs (mapped here, shell override, or ambient match).
func printApplyHint(cmd *cobra.Command, label, providerName, name string) {
	if g, err := resolveGoverning(providerName); err == nil && g == name {
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), cliDim.Render(fmt.Sprintf(
		"%s: note — tokens live in the profile; plain CLIs HERE still act as the ambient default.\n"+
			"      apply with: %s use %s (this dir) · shell (subshell) · env (this shell) · default (everywhere)",
		label, label, name)))
}
