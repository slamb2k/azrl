package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/aws"
)

// validAwsName guards AWS profile names the same way validGhName guards GitHub
// ones, reserving the aws global-conf basename.
func validAwsName(name string) error {
	if name == "" {
		return fmt.Errorf("aws: a profile name is required")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("aws: invalid profile name %q", name)
	}
	if name == "aws" {
		return fmt.Errorf("aws: refusing to use the global aws config")
	}
	return nil
}

// awsConfPath is the conf file path for an AWS profile.
func awsConfPath(dir, name string) string {
	return dir + string(os.PathSeparator) + name + ".conf"
}

func newAwsLoginCmd() *cobra.Command {
	var startURL, region, accountID, roleName string
	var isolate, device bool
	c := &cobra.Command{
		Use:   "login [name]",
		Short: "Sign in to an AWS account via SSO (browser pops on your local machine)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prov := aws.NewProvider()
			dir := prov.ProfilesDir()
			name, err := resolveLoginTarget(cmd, prov, args, "azrl aws")
			if err != nil {
				return err
			}
			if err := validAwsName(name); err != nil {
				return err
			}
			conf, err := aws.LoadConf(name, dir)
			if err != nil {
				// New profile: seed a conf from the flags.
				conf = aws.Conf{SSOStartURL: startURL, SSORegion: region, AccountID: accountID, RoleName: roleName, Isolate: isolate}
				if werr := conf.Write(awsConfPath(dir, name)); werr != nil {
					return werr
				}
			} else if isolate && !conf.Isolate {
				conf.Isolate = true
				if serr := aws.SetIsolate(dir, name, true); serr != nil {
					return serr
				}
			}
			if err := aws.SyncConfig(name, dir, conf); err != nil {
				return err
			}
			cmd.Printf("aws: signing in to %s as profile %q\n", conf.SSOStartURL, name)
			if err := aws.Login(dir, name, conf.Isolate, device); err != nil {
				return err
			}
			if conf.ExpectAccount != "" || conf.ExpectARN != "" {
				id, err := aws.CallerIdentity(dir, name, conf.Isolate)
				if err != nil {
					return fmt.Errorf("aws: could not verify signed-in identity: %w", err)
				}
				if err := aws.AssertAccount(id, conf.ExpectAccount, conf.ExpectARN); err != nil {
					return err
				}
			}
			pwd, _ := os.Getwd()
			return aws.Scheme().Touch(name, dir, pwd)
		},
	}
	c.Flags().StringVar(&startURL, "sso-start-url", "", "IAM Identity Center portal URL (https://<org>.awsapps.com/start)")
	c.Flags().StringVar(&region, "sso-region", "", "Region the SSO portal lives in")
	c.Flags().StringVar(&accountID, "account-id", "", "AWS account ID to assume into")
	c.Flags().StringVar(&roleName, "role-name", "", "Permission-set role to assume")
	c.Flags().BoolVar(&isolate, "isolate", false, "Scope this profile to its own config/credentials files")
	c.Flags().BoolVar(&device, "use-device-code", false, "Use the device-code flow instead of the PKCE loopback")
	return c
}

func newAwsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured AWS profiles and their SSO portals",
		RunE: func(cmd *cobra.Command, args []string) error {
			prov := aws.NewProvider()
			profs, err := prov.ListProfiles(prov.ProfilesDir())
			if err != nil {
				return err
			}
			for _, p := range profs {
				cmd.Printf("%-24s %s\n", p.Display(), p.Detail)
			}
			return nil
		},
	}
}

func newAwsUseCmd() *cobra.Command {
	var isolate bool
	c := &cobra.Command{
		Use:   "use <name>",
		Short: "Pin the current directory to an AWS profile and sync ~/.aws/config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validAwsName(name); err != nil {
				return err
			}
			prov := aws.NewProvider()
			dir := prov.ProfilesDir()
			conf, err := aws.LoadConf(name, dir)
			if err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			if err := prov.Use(name, dir, pwd); err != nil {
				return err
			}
			if isolate && !conf.Isolate {
				conf.Isolate = true
				if serr := aws.SetIsolate(dir, name, true); serr != nil {
					return serr
				}
			}
			if err := aws.SyncConfig(name, dir, conf); err != nil {
				cmd.Printf("aws: pinned %s/.awsprofile -> %q (config sync skipped: %v)\n", pwd, name, err)
				return nil
			}
			cmd.Printf("aws: pinned %s/.awsprofile -> %q and synced ~/.aws/config\n", pwd, name)
			offerAwsEnvrc(pwd, name, conf.Isolate, cmd.OutOrStdout(), cmd.InOrStdin())
			return nil
		},
	}
	c.Flags().BoolVar(&isolate, "isolate", false, "Scope this profile to its own config/credentials files")
	return c
}

func newAwsRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove an AWS profile and its isolated config dir",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validAwsName(name); err != nil {
				return err
			}
			prov := aws.NewProvider()
			pwd, _ := os.Getwd()
			removed, err := prov.Remove(name, prov.ProfilesDir(), pwd)
			if err != nil {
				return err
			}
			for _, r := range removed {
				cmd.Printf("removed %s\n", r)
			}
			return nil
		},
	}
}

func newAwsCaptureCmd() *cobra.Command {
	var startURL, region, accountID, roleName, expectAccount, expectARN string
	c := &cobra.Command{
		Use:   "capture <name>",
		Short: "Record an AWS SSO account into a profile from its portal details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validAwsName(name); err != nil {
				return err
			}
			prov := aws.NewProvider()
			dir := prov.ProfilesDir()
			conf := aws.Conf{
				SSOStartURL: startURL, SSORegion: region, AccountID: accountID,
				RoleName: roleName, ExpectAccount: expectAccount, ExpectARN: expectARN,
			}
			if err := conf.Write(awsConfPath(dir, name)); err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			if err := aws.Scheme().Touch(name, dir, pwd); err != nil {
				return err
			}
			cmd.Printf("aws: captured %s into profile %q\n", startURL, name)
			return nil
		},
	}
	c.Flags().StringVar(&startURL, "sso-start-url", "", "IAM Identity Center portal URL")
	c.Flags().StringVar(&region, "sso-region", "", "Region the SSO portal lives in")
	c.Flags().StringVar(&accountID, "account-id", "", "AWS account ID")
	c.Flags().StringVar(&roleName, "role-name", "", "Permission-set role")
	c.Flags().StringVar(&expectAccount, "expect-account", "", "Account ID to assert after sign-in")
	c.Flags().StringVar(&expectARN, "expect-arn", "", "Permission-set role prefix to assert (AWSReservedSSO_<permset>)")
	return c
}

func newAwsStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the ambient and repo-pinned AWS profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			if p := os.Getenv("AWS_PROFILE"); p != "" {
				cmd.Printf("ambient AWS_PROFILE: %s\n", p)
			} else {
				cmd.Println("ambient AWS_PROFILE: (unset)")
			}
			prov := aws.NewProvider()
			pwd, _ := os.Getwd()
			if pin, err := prov.Resolve("", pwd); err == nil {
				cmd.Printf("this dir is pinned to: %s\n", pin)
			} else {
				cmd.Println("this dir has no .awsprofile pin")
			}
			return nil
		},
	}
}

// offerAwsEnvrc offers to write a direnv .envrc so plain `aws` in pwd follows the
// profile. A closed/non-tty stdin reads as a decline (never hangs).
func offerAwsEnvrc(pwd, name string, isolate bool, out io.Writer, in io.Reader) {
	fmt.Fprint(out, "aws: also write .envrc so `aws` in this dir follows this profile? [y/N] ")
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		fmt.Fprintln(out)
		return
	}
	if ans := strings.TrimSpace(sc.Text()); !strings.HasPrefix(strings.ToLower(ans), "y") {
		return
	}
	wrote, err := aws.WriteEnvrc(pwd, name, isolate)
	if err != nil {
		fmt.Fprintf(out, "aws: could not write .envrc: %v\n", err)
		return
	}
	if wrote {
		fmt.Fprintf(out, "aws: wrote %s/.envrc — run `direnv allow` to activate\n", pwd)
	}
}

// awsSubcommands builds a fresh set of the AWS subcommands.
func awsSubcommands() []*cobra.Command {
	return []*cobra.Command{
		newAwsLoginCmd(), newAwsListCmd(), newAwsUseCmd(),
		newAwsRmCmd(), newAwsCaptureCmd(), newAwsStatusCmd(),
	}
}

// newAwsGroupCmd is the `aws` parent for the unified azrl binary.
func newAwsGroupCmd() *cobra.Command {
	g := &cobra.Command{
		Use:   "aws",
		Short: "Manage AWS accounts (login, use, capture, …)",
	}
	g.AddCommand(awsSubcommands()...)
	return g
}

func init() { RootCmd.AddCommand(newAwsGroupCmd()) }
