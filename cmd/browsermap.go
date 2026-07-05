package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/browserpick"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/gcp"
	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// newBrowserMapCmd builds the `browser <name>` verb for one provider group:
// discover the laptop's Edge/Chrome profiles, offer a numbered pick (identity
// matches first) plus manual entry and clear, and write the mapping keys.
func newBrowserMapCmd(tool string, provFn func() provider.Provider, expectIdent func(name, dir string) string, validName func(string) error) *cobra.Command {
	return &cobra.Command{
		Use:   "browser <name>",
		Short: "Map the profile to a local browser profile (Edge/Chrome)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validName(name); err != nil {
				return err
			}
			prov := provFn()
			dir := prov.ProfilesDir()
			if _, err := os.Stat(filepath.Join(dir, name+".conf")); err != nil {
				return fmt.Errorf("%s: no profile %q", tool, name)
			}
			cmdKey, labelKey := browserpick.Keys(prov.Name())
			out := cmd.OutOrStdout()
			var found []browserpick.Profile
			if g, err := config.LoadGlobal(config.ProfilesDir()); err == nil {
				if ps, derr := browserpick.Discover(g); derr == nil {
					found = ps
				} else {
					fmt.Fprintf(out, "%s: discovery failed (%v) — manual entry only\n", tool, derr)
				}
			}
			if ident := expectIdent(name, dir); ident != "" {
				sort.SliceStable(found, func(i, j int) bool {
					return found[i].Email == ident && found[j].Email != ident
				})
			}
			for i, p := range found {
				email := p.Email
				if email == "" {
					email = "(not signed in)"
				}
				fmt.Fprintf(out, "%2d) %-28s %s\n", i+1, p.Label(), email)
			}
			fmt.Fprintln(out, " m) enter command manually")
			fmt.Fprintln(out, " 0) clear mapping")
			fmt.Fprint(out, "select: ")
			in := bufio.NewScanner(cmd.InOrStdin())
			if !in.Scan() {
				return fmt.Errorf("%s: no selection", tool)
			}
			s := prov.Scheme()
			set := func(cmdVal, labelVal string) error {
				if err := s.SetKey(name, dir, cmdKey, cmdVal); err != nil {
					return err
				}
				return s.SetKey(name, dir, labelKey, labelVal)
			}
			switch ans := strings.TrimSpace(in.Text()); {
			case ans == "0":
				if err := set("", ""); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s: cleared browser mapping for %q\n", tool, name)
			case ans == "m":
				fmt.Fprint(out, "command: ")
				if !in.Scan() || strings.TrimSpace(in.Text()) == "" {
					return fmt.Errorf("%s: no command entered", tool)
				}
				c := strings.TrimSpace(in.Text())
				if err := set(c, ""); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s: %q now opens with: %s\n", tool, name, c)
			default:
				n, err := strconv.Atoi(ans)
				if err != nil || n < 1 || n > len(found) {
					return fmt.Errorf("%s: invalid selection %q", tool, ans)
				}
				p := found[n-1]
				if err := set(p.Command(), p.Label()); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s: %q now opens with %s\n", tool, name, p.Label())
			}
			return nil
		},
	}
}

func azureExpectIdent(name, dir string) string {
	c, err := profile.LoadConf(name, dir)
	if err != nil {
		return ""
	}
	return c.ExpectUser
}

func gcpExpectIdent(name, dir string) string {
	c, err := gcp.LoadConf(name, dir)
	if err != nil {
		return ""
	}
	return c.ExpectAccount
}

func ghExpectIdent(name, dir string) string {
	c, err := github.LoadConf(name, dir)
	if err != nil {
		return ""
	}
	return c.User // a login, not an email — matches only when they coincide
}

func noExpectIdent(string, string) string { return "" }

func newAzureBrowserCmd() *cobra.Command {
	return newBrowserMapCmd("azrl", func() provider.Provider { return azure.NewProvider() }, azureExpectIdent, validAzureName)
}

func newGhBrowserCmd() *cobra.Command {
	return newBrowserMapCmd("ghrl", func() provider.Provider { return github.NewProvider() }, ghExpectIdent, validGhName)
}

func newAwsBrowserCmd() *cobra.Command {
	return newBrowserMapCmd("azrl aws", func() provider.Provider { return aws.NewProvider() }, noExpectIdent, validAwsName)
}

func newGcpBrowserCmd() *cobra.Command {
	return newBrowserMapCmd("azrl gcp", func() provider.Provider { return gcp.NewProvider() }, gcpExpectIdent, validGcpName)
}

func init() { RootCmd.AddCommand(newAzureBrowserCmd()) }
