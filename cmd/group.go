package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// newValidName builds a profile-name guard for a provider group: the shared
// profile.ValidateName rule, plus the reserved global-conf basename refused.
func newValidName(tool, reserved string) func(string) error {
	return func(name string) error {
		if name == "" {
			return fmt.Errorf("%s: a profile name is required", tool)
		}
		if err := profile.ValidateName(name); err != nil {
			return fmt.Errorf("%s: %w", tool, err)
		}
		if name == reserved {
			return fmt.Errorf("%s: refusing to use the global %s config", tool, reserved)
		}
		return nil
	}
}

// captureName resolves capture's optional name argument: the explicit name,
// else one derived from the current directory (profile.DefaultName).
func captureName(args []string) string {
	pwd, _ := os.Getwd()
	arg := ""
	if len(args) == 1 {
		arg = args[0]
	}
	return profile.DefaultName(arg, pwd)
}

// groupConfPath is the conf file path for a provider-group profile.
func groupConfPath(dir, name string) string {
	return dir + string(os.PathSeparator) + name + ".conf"
}

// newGroupRmCmd is the shared `rm` verb for a provider group. Like the Azure
// rm it confirms on a TTY and refuses without one unless --yes is passed.
func newGroupRmCmd(provFn func() provider.Provider, short string, valid func(string) error) *cobra.Command {
	var unlinkAll, yes bool
	var replace string
	c := &cobra.Command{
		Use:   "rm <name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := valid(name); err != nil {
				return withCode(ExitUsage, err)
			}
			prov := provFn()
			dir := prov.ProfilesDir()
			tool := prov.Scheme().Prefix
			if err := requireProfile(tool, prov, name); err != nil {
				return err
			}
			if err := refuseIfLinked(prov.Scheme(), dir, name, unlinkAll, replace); err != nil {
				return err
			}
			if err := confirmDestructive(cmd, tool, fmt.Sprintf("%s: remove profile %q and its isolated config dir?", tool, name), yes); err != nil {
				return err
			}
			if err := unlinkOrReplace(cmd, prov.Scheme(), dir, name, unlinkAll, replace); err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			removed, err := prov.Remove(name, dir, pwd)
			if err != nil {
				return err
			}
			for _, r := range removed {
				fmt.Fprintf(cmd.ErrOrStderr(), "removed %s\n", r)
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt (required without a TTY)")
	c.Flags().BoolVar(&unlinkAll, "unmap-all", false, "Remove every directory mapping before deleting the profile")
	c.Flags().StringVar(&replace, "replace", "", "Repoint every directory link at this profile before deleting")
	c.MarkFlagsMutuallyExclusive("unmap-all", "replace")
	return c
}

// offerGroupEnvrc offers to write a direnv .envrc so the plain native CLI in
// pwd follows the profile, via the shared offerEnvrcIn.
func offerGroupEnvrc(tool, cli string, writeEnvrc func(pwd, name string, isolate bool) (bool, error), pwd, name string, isolate bool, mode envrcMode, out io.Writer, in io.Reader) {
	offerEnvrcIn(tool, cli, pwd, func() (bool, error) { return writeEnvrc(pwd, name, isolate) }, mode, out, in)
}

// resolveGoverning returns the profile governing the cwd for providerName —
// the same ladder `whoami` renders: shell override, else the pointer walk-up,
// else the ambient identity's managed match. Lets `console`/`shell` run bare
// from any directory where a default identity is in effect.
func resolveGoverning(providerName string) (string, error) {
	if p, name, ok := strings.Cut(os.Getenv("AZRL_PROFILE"), ":"); ok && p == providerName && name != "" {
		return name, nil
	}
	for _, prov := range provider.All() {
		if prov.Name() != providerName {
			continue
		}
		pwd, _ := os.Getwd()
		name, err := prov.Resolve("", pwd)
		if errors.Is(err, profile.ErrInvalidName) {
			return "", err
		}
		if err == nil && name != "" {
			return name, nil
		}
		confdir := prov.ProfilesDir()
		amb, err := prov.Ambient()
		if err != nil || amb.Identity == "" {
			break
		}
		listed, err := prov.ListProfiles(confdir)
		if err != nil {
			break
		}
		var sts []provider.Status
		for _, l := range listed {
			if st, serr := prov.Status(l.Name, confdir); serr == nil {
				sts = append(sts, st)
			}
		}
		if name := provider.MatchProfile(sts, amb.Identity); name != "" {
			return name, nil
		}
	}
	return "", usageErr("azrl: no %s profile governs this directory — pass a name, map one (use <name>), or set a default (default <name>)", providerName)
}

// nameOrGoverning resolves the optional profile argument for verbs that can
// run bare: an explicit name wins, else the governing ladder decides.
func nameOrGoverning(providerName string, args []string) (string, error) {
	if len(args) == 1 {
		if err := profile.ValidateName(args[0]); err != nil {
			return "", withCode(ExitUsage, fmt.Errorf("azrl: %w", err))
		}
		return args[0], nil
	}
	return resolveGoverning(providerName)
}
