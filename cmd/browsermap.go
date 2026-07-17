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
			} else {
				fmt.Fprintf(out, "%s: discovery unavailable (%v) — manual entry only\n", tool, err)
			}
			if ident := expectIdent(name, dir); ident != "" {
				sort.SliceStable(found, func(i, j int) bool {
					return found[i].Email == ident && found[j].Email != ident
				})
			}
			var picked string // "m", "0", or a 1-based number as text
			in := bufio.NewScanner(cmd.InOrStdin())
			if useArrowPicker() {
				items := make([]pickItem, 0, len(found)+2)
				for _, p := range found {
					email := p.Email
					if email == "" {
						email = "(not signed in)"
					}
					items = append(items, pickItem{Label: p.Label(), Detail: email})
				}
				items = append(items,
					pickItem{Label: "enter command manually"},
					pickItem{Label: "clear mapping"})
				idx, perr := pickArrow(fmt.Sprintf("Browser for profile %q", name), items)
				if perr != nil {
					return perr
				}
				switch idx {
				case len(found):
					picked = "m"
				case len(found) + 1:
					picked = "0"
				default:
					picked = fmt.Sprintf("%d", idx+1)
				}
			} else {
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
				if !in.Scan() {
					return fmt.Errorf("%s: no selection", tool)
				}
				picked = strings.TrimSpace(in.Text())
			}
			s := prov.Scheme()
			// A browser profile has a single owner per provider: assigning one
			// that another profile already uses asks to steal it — yes moves
			// the mapping (the other profile is cleared), no leaves everything.
			set := func(cmdVal, labelVal string) (bool, error) {
				if others := s.FindByKey(dir, cmdKey, cmdVal, name); len(others) != 0 {
					fmt.Fprintf(out, "%s: that browser already opens for %s — steal it? [y/N]: ",
						tool, strings.Join(others, ", "))
					if !in.Scan() || !strings.EqualFold(strings.TrimSpace(in.Text()), "y") {
						fmt.Fprintf(out, "%s: unchanged\n", tool)
						return false, nil
					}
					for _, o := range others {
						if err := s.SetKey(o, dir, cmdKey, ""); err != nil {
							return false, err
						}
						if err := s.SetKey(o, dir, labelKey, ""); err != nil {
							return false, err
						}
					}
				}
				if err := s.SetKey(name, dir, cmdKey, cmdVal); err != nil {
					return false, err
				}
				return true, s.SetKey(name, dir, labelKey, labelVal)
			}
			switch ans := picked; {
			case ans == "0":
				if _, err := set("", ""); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s: cleared browser mapping for %q\n", tool, name)
			case ans == "m":
				fmt.Fprint(out, "command: ")
				if !in.Scan() || strings.TrimSpace(in.Text()) == "" {
					return fmt.Errorf("%s: no command entered", tool)
				}
				c := strings.TrimSpace(in.Text())
				if ok, err := set(c, ""); err != nil {
					return err
				} else if ok {
					fmt.Fprintf(out, "%s: %q now opens with: %s\n", tool, name, c)
				}
			default:
				n, err := strconv.Atoi(ans)
				if err != nil || n < 1 || n > len(found) {
					return fmt.Errorf("%s: invalid selection %q", tool, ans)
				}
				p := found[n-1]
				if ok, err := set(p.Command(), p.Label()); err != nil {
					return err
				} else if ok {
					fmt.Fprintf(out, "%s: %q now opens with %s\n", tool, name, p.Label())
				}
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
