package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/gcp"
	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// pickDefaultProfile resolves the profile the default verb targets: the given
// name verbatim, a lone profile automatically, or a numbered pick from in.
func pickDefaultProfile(providerName, name string, in io.Reader, out io.Writer) (string, error) {
	if name != "" {
		return name, nil
	}
	var listed []profile.Listed
	for _, p := range provider.All() {
		if p.Name() == providerName {
			listed, _ = p.ListProfiles(p.ProfilesDir())
		}
	}
	switch len(listed) {
	case 0:
		return "", fmt.Errorf("azrl: no %s profiles — create one with login first", providerName)
	case 1:
		fmt.Fprintf(out, "azrl: one profile — using %s\n", listed[0].Name)
		return listed[0].Name, nil
	}
	if useArrowPicker() {
		items := make([]pickItem, len(listed))
		for i, p := range listed {
			items[i] = pickItem{Label: p.Display(), Detail: p.Detail}
		}
		idx, perr := pickArrow(fmt.Sprintf("Make which %s profile the default?", providerName), items)
		if perr != nil {
			return "", perr
		}
		return listed[idx].Name, nil
	}
	fmt.Fprintf(out, "Make which %s profile the default?\n", providerName)
	for i, p := range listed {
		fmt.Fprintf(out, "  %d) %s  %s\n", i+1, p.Display(), p.Detail)
	}
	fmt.Fprint(out, "> ")
	sc := bufio.NewScanner(in)
	sc.Scan()
	n, err := strconv.Atoi(strings.TrimSpace(sc.Text()))
	if err != nil || n < 1 || n > len(listed) {
		return "", fmt.Errorf("azrl: pick 1-%d", len(listed))
	}
	return listed[n-1].Name, nil
}

// runDefault makes profile <name>'s identity the provider's NATIVE default —
// the account plain CLI commands use in unmapped directories. The profile is
// a template only: its conf targets an interactive native sign-in (or gh's
// native account switch) run through the browser bridge; tokens are never
// copied between stores. This is the user-invoked exception to the mirror
// boundary recorded in docs/ambient-identity-model.md (2026-07-08 amendment).
func runDefault(providerName, name string, in io.Reader, out io.Writer) error {
	name, err := pickDefaultProfile(providerName, name, in, out)
	if err != nil {
		return err
	}
	switch providerName {
	case "azure":
		c, err := profile.LoadConf(name, config.ProfilesDir())
		if err != nil {
			return err
		}
		tenant := c.TenantID
		if tenant == "" {
			tenant = c.Tenant
		}
		fmt.Fprintf(out, "azrl: signing the native az session in as %s (tenant %s)\n", name, tenant)
		return runBridge([]string{"az", "login", "--tenant", tenant}, out)
	case "gcp":
		c, err := gcp.LoadConf(name, config.GcpProfilesDir())
		if err != nil {
			return err
		}
		args := []string{"gcloud", "auth", "login"}
		if c.ExpectAccount != "" {
			args = append(args, c.ExpectAccount)
		}
		fmt.Fprintf(out, "azrl: signing the native gcloud session in as %s\n", name)
		return runBridge(args, out)
	case "github":
		c, err := github.LoadConf(name, config.GithubProfilesDir())
		if err != nil {
			return err
		}
		if c.User == "" {
			return fmt.Errorf("azrl: %s has no GH_USER — capture the session or edit the conf first", name)
		}
		// The account may already be known to the native gh: switch is
		// instant and needs no browser. Fall back to a bridged web login.
		code, err := bridgeRun([]string{"gh", "auth", "switch", "--hostname", c.Host, "--user", c.User}, out)
		if err != nil {
			return err
		}
		if code == 0 {
			return nil
		}
		fmt.Fprintf(out, "azrl: %s isn't signed in to the native gh — starting a web login\n", c.User)
		return runBridge([]string{"gh", "auth", "login", "--hostname", c.Host, "--web"}, out)
	case "aws":
		c, err := aws.LoadConf(name, config.AwsProfilesDir())
		if err != nil {
			return err
		}
		path, err := aws.SetDefaultProfile(c)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "azrl: %s [default] now targets %s/%s — signing in\n", path, c.AccountID, c.RoleName)
		return runBridge([]string{"aws", "sso", "login"}, out)
	}
	return fmt.Errorf("azrl: unknown provider %q", providerName)
}

func newDefaultCmd(providerName, short string) *cobra.Command {
	return &cobra.Command{
		Use:          "default [name]",
		Short:        short,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runDefault(providerName, name, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
}

func init() {
	RootCmd.AddCommand(newDefaultCmd("azure", "Make a profile's identity the native az default (interactive sign-in via the bridge)"))
}
