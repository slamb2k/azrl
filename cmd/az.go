package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

func azureProvider() provider.Provider { return azure.NewProvider() }

func newAzureUseCmd() *cobra.Command {
	var envrc *envrcFlags
	c := &cobra.Command{
		Use:   "use <name>",
		Short: "Map the current directory to an existing Azure profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validAzureName(name); err != nil {
				return withCode(ExitUsage, err)
			}
			if err := requireProfile("azrl", azureProvider(), name); err != nil {
				return err
			}
			pwd, _ := os.Getwd()
			if err := profile.Use(name, config.ProfilesDir(), pwd); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "azrl: linked %s/.azprofile -> profile %q\n", pwd, name)
			offerEnvrc(pwd, envrc.mode(), cmd.ErrOrStderr(), cmd.InOrStdin())
			return nil
		},
	}
	envrc = addEnvrcFlags(c)
	return c
}

func newAzureCaptureCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "capture [name]",
		Short: "Record the current az session as a profile (metadata only, no login, no mapping)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pwd, _ := os.Getwd()
			name := captureName(args)
			if err := validAzureName(name); err != nil {
				return withCode(ExitUsage, err)
			}
			if asJSON {
				stdout, restore := jsonStdout(cmd)
				defer restore()
				c, created, err := captureSession(name, pwd, false, cmd.ErrOrStderr())
				if err != nil {
					return err
				}
				return writeJSON(stdout, captureResult{Provider: "azure", Profile: name, Identity: c.ExpectUser, Detail: c.Tenant, Created: created})
			}
			_, _, err := captureSession(name, pwd, false, cmd.ErrOrStderr())
			return err
		},
	}
	addJSONFlag(c, &asJSON, "the captured profile")
	return c
}

// azureSubcommands builds a fresh set of the Azure verbs (cobra commands bind
// to one parent, so `azrl az …` and the hidden top-level aliases each get
// their own).
func azureSubcommands() []*cobra.Command {
	return []*cobra.Command{
		newAzureLoginCmd(),
		newAzureUseCmd(),
		newGroupRmCmd(azureProvider, "Remove an Azure profile and its isolated config dir", validAzureName),
		newAzureCaptureCmd(),
		newAzureBrowserCmd(),
		newShellCmd("azure", "Open a subshell acting as an Azure profile (no mapping)"),
		newConsoleCmd("azure", "Open the Azure portal as a profile's tenant"),
		newUnlinkCmd("azure", "Remove this directory's Azure profile mapping (keeps the profile)"),
		newEnvCmd(azureProvider, "azrl", "azure", validAzureName),
		newDefaultCmd("azure", "Make a profile's identity the native az default (interactive sign-in via the bridge)"),
	}
}

// newAzGroupCmd is the `az` parent: Azure is a provider like the others.
func newAzGroupCmd() *cobra.Command {
	g := &cobra.Command{
		Use:     "az",
		Aliases: []string{"azure"},
		Short:   "Manage Azure tenants (login, use, capture, …)",
		RunE:    groupRun,
	}
	g.AddCommand(azureSubcommands()...)
	return g
}

func init() {
	RootCmd.AddCommand(newAzGroupCmd())
	// The pre-group spellings (`azrl login`, `azrl use`, …) stay as hidden
	// aliases: muscle memory, scripts and the TUI's handoffs keep working.
	for _, c := range azureSubcommands() {
		c.Hidden = true
		RootCmd.AddCommand(c)
	}
}
