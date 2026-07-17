package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/provider"
)

// newValidName builds a profile-name guard for a provider group: non-empty,
// no "/", and the reserved global-conf basename refused.
func newValidName(tool, reserved string) func(string) error {
	return func(name string) error {
		if name == "" {
			return fmt.Errorf("%s: a profile name is required", tool)
		}
		if strings.Contains(name, "/") {
			return fmt.Errorf("%s: invalid profile name %q", tool, name)
		}
		if name == reserved {
			return fmt.Errorf("%s: refusing to use the global %s config", tool, reserved)
		}
		return nil
	}
}

// groupConfPath is the conf file path for a provider-group profile.
func groupConfPath(dir, name string) string {
	return dir + string(os.PathSeparator) + name + ".conf"
}

// newGroupListCmd is the shared `list` verb for a provider group.
func newGroupListCmd(provFn func() provider.Provider, short string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			prov := provFn()
			profs, err := prov.ListProfiles(prov.ProfilesDir())
			if err != nil {
				return err
			}
			pairs := make([][2]string, len(profs))
			for i, p := range profs {
				pairs[i] = [2]string{p.Display(), p.Detail}
			}
			printList(cmd.OutOrStdout(), pairs)
			return nil
		},
	}
}

// newGroupRmCmd is the shared `rm` verb for a provider group.
func newGroupRmCmd(provFn func() provider.Provider, short string, valid func(string) error) *cobra.Command {
	var unlinkAll bool
	var replace string
	c := &cobra.Command{
		Use:   "rm <name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := valid(name); err != nil {
				return err
			}
			prov := provFn()
			dir := prov.ProfilesDir()
			if err := refuseIfLinked(prov.Scheme(), dir, name, unlinkAll, replace); err != nil {
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
				cmd.Printf("removed %s\n", r)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&unlinkAll, "unmap-all", false, "Remove every directory mapping before deleting the profile")
	c.Flags().StringVar(&replace, "replace", "", "Repoint every directory link at this profile before deleting")
	c.MarkFlagsMutuallyExclusive("unmap-all", "replace")
	return c
}

// newGroupStatusCmd is the shared `status` verb for a provider group. When
// ambientEnv is set, the env-var line is printed before the pin lines.
func newGroupStatusCmd(provFn func() provider.Provider, short, pointerName, ambientEnv string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ambientEnv != "" {
				if p := os.Getenv(ambientEnv); p != "" {
					cmd.Printf("ambient %s: %s\n", ambientEnv, p)
				} else {
					cmd.Printf("ambient %s: (unset)\n", ambientEnv)
				}
			}
			prov := provFn()
			pwd, _ := os.Getwd()
			if pin, err := prov.Resolve("", pwd); err == nil {
				cmd.Printf("this dir is pinned to: %s\n", pin)
			} else {
				cmd.Printf("this dir has no %s pin\n", pointerName)
			}
			return nil
		},
	}
}

// offerGroupEnvrc offers to write a direnv .envrc so the plain native CLI in
// pwd follows the profile. A closed/non-tty stdin reads as a decline (never
// hangs).
func offerGroupEnvrc(tool, cli string, writeEnvrc func(pwd, name string, isolate bool) (bool, error), pwd, name string, isolate bool, out io.Writer, in io.Reader) {
	fmt.Fprintf(out, "%s: also write .envrc so `%s` in this dir follows this profile? [y/N] ", tool, cli)
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		fmt.Fprintln(out)
		return
	}
	if ans := strings.TrimSpace(sc.Text()); !strings.HasPrefix(strings.ToLower(ans), "y") {
		return
	}
	wrote, err := writeEnvrc(pwd, name, isolate)
	if err != nil {
		fmt.Fprintf(out, "%s: could not write .envrc: %v\n", tool, err)
		return
	}
	if wrote {
		fmt.Fprintf(out, "%s: wrote %s/.envrc — run `direnv allow` to activate\n", tool, pwd)
	}
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
		if name, err := prov.Resolve("", pwd); err == nil && name != "" {
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
	return "", fmt.Errorf("azrl: no %s profile governs this directory — pass a name, map one (use <name>), or set a default (default <name>)", providerName)
}

// nameOrGoverning resolves the optional profile argument for verbs that can
// run bare: an explicit name wins, else the governing ladder decides.
func nameOrGoverning(providerName string, args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	return resolveGoverning(providerName)
}
