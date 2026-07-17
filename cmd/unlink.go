package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
	"github.com/spf13/cobra"
)

// runUnlink removes the cwd's directory→profile mapping for one provider.
// The profile itself — tokens and all — is untouched; only the edge dies.
func runUnlink(providerName string, out io.Writer) error {
	pwd, _ := os.Getwd()
	for _, p := range provider.All() {
		if p.Name() != providerName {
			continue
		}
		name, err := p.Scheme().Unlink(p.ProfilesDir(), pwd)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: unmapped %s from %s (profile kept)\n", providerName, pwd, name)
		if w := profile.EnvrcWarning(providerName, pwd); w != "" {
			fmt.Fprintln(out, w)
		}
		return nil
	}
	return fmt.Errorf("azrl: unknown provider %q", providerName)
}

func newUnlinkCmd(providerName, short string) *cobra.Command {
	return &cobra.Command{
		Use:          "unmap",
		Short:        short,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnlink(providerName, cmd.OutOrStdout())
		},
	}
}

func init() {
	RootCmd.AddCommand(newUnlinkCmd("azure", "Remove this directory's Azure profile mapping (keeps the profile)"))
}
