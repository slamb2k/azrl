package cmd

import (
	"bufio"
	"fmt"
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
// --cmd/--label and --clear do the same unattended, skipping discovery.
func newBrowserMapCmd(tool string, provFn func() provider.Provider, expectIdent func(name, dir string) string, validName func(string) error) *cobra.Command {
	var flagCmd, flagLabel string
	var flagClear, flagSteal bool
	c := &cobra.Command{
		Use:   "browser <name>",
		Short: "Map the profile to a local browser profile (Edge/Chrome)",
		Long: "Map the profile to a local browser profile (Edge/Chrome).\n\n" +
			"With no flags it discovers the browser profiles on your local machine and offers a picker.\n" +
			"--cmd/--label (or --clear) set the mapping directly, with no discovery and no prompt.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validName(name); err != nil {
				return withCode(ExitUsage, err)
			}
			prov := provFn()
			dir := prov.ProfilesDir()
			if err := requireProfile(tool, prov, name); err != nil {
				return err
			}
			if flagLabel != "" && flagCmd == "" {
				return usageErr("%s: --label needs --cmd", tool)
			}
			cmdKey, labelKey := browserpick.Keys(prov.Name())
			out := cmd.OutOrStdout()
			in := bufio.NewScanner(cmd.InOrStdin())
			s := prov.Scheme()
			// A browser profile has a single owner per provider: assigning one
			// that another profile already uses steals it — --steal (or a yes at
			// the TTY prompt) clears the other holders; no leaves everything.
			set := func(cmdVal, labelVal string) (bool, error) {
				if others := s.FindByKey(dir, cmdKey, cmdVal, name); len(others) != 0 {
					switch {
					case flagSteal:
					case !isInteractive():
						return false, usageErr("%s: that browser already opens for %s — pass --steal to move it", tool, strings.Join(others, ", "))
					default:
						fmt.Fprintf(out, "%s: that browser already opens for %s — %s [y/N]: ",
							tool, cliBold.Render(strings.Join(others, ", ")), cliParent.Render("steal it?"))
						if !in.Scan() || !strings.EqualFold(strings.TrimSpace(in.Text()), "y") {
							fmt.Fprintln(out, cliDim.Render(tool+": unchanged"))
							return false, nil
						}
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
			clear := func() error {
				if _, err := set("", ""); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s %s: cleared browser mapping for %s\n", cliGood.Render("✓"), tool, cliBold.Render(name))
				return nil
			}
			setManual := func(c, label string) error {
				ok, err := set(c, label)
				if err != nil || !ok {
					return err
				}
				fmt.Fprintf(out, "%s %s: %s now opens with: %s\n", cliGood.Render("✓"), tool, cliBold.Render(name), cliValue.Render(c))
				return nil
			}
			switch {
			case flagClear:
				return clear()
			case flagCmd != "":
				return setManual(flagCmd, flagLabel)
			case !isInteractive():
				return usageErr("%s: no TTY for the picker — pass --cmd <command> [--label <text>] or --clear", tool)
			}

			var found []browserpick.Profile
			if g, err := config.LoadGlobal(config.ProfilesDir()); err == nil {
				if ps, derr := browserpick.Discover(g); derr == nil {
					found = ps
				} else {
					fmt.Fprintln(out, cliDim.Render(fmt.Sprintf("%s: discovery failed (%v) — manual entry only", tool, derr)))
				}
			} else {
				fmt.Fprintln(out, cliDim.Render(fmt.Sprintf("%s: discovery unavailable (%v) — manual entry only", tool, err)))
			}
			if ident := expectIdent(name, dir); ident != "" {
				sort.SliceStable(found, func(i, j int) bool {
					return found[i].Email == ident && found[j].Email != ident
				})
			}
			var picked string // "m", "0", or a 1-based number as text
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
			switch ans := picked; {
			case ans == "0":
				return clear()
			case ans == "m":
				fmt.Fprint(out, cliDim.Render("command: "))
				if !in.Scan() || strings.TrimSpace(in.Text()) == "" {
					return fmt.Errorf("%s: no command entered", tool)
				}
				return setManual(strings.TrimSpace(in.Text()), "")
			default:
				n, err := strconv.Atoi(ans)
				if err != nil || n < 1 || n > len(found) {
					return fmt.Errorf("%s: invalid selection %q", tool, ans)
				}
				p := found[n-1]
				if ok, err := set(p.Command(), p.Label()); err != nil {
					return err
				} else if ok {
					fmt.Fprintf(out, "%s %s: %s now opens with %s\n", cliGood.Render("✓"), tool, cliBold.Render(name), cliValue.Render(p.Label()))
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&flagCmd, "cmd", "", "Browser launch command to map (skips discovery and the picker)")
	c.Flags().StringVar(&flagLabel, "label", "", "Display label for --cmd (e.g. \"Edge · Work\")")
	c.Flags().BoolVar(&flagClear, "clear", false, "Remove the profile's browser mapping")
	c.Flags().BoolVar(&flagSteal, "steal", false, "Take the browser from any other profile that already uses it")
	c.MarkFlagsMutuallyExclusive("cmd", "clear")
	return c
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
