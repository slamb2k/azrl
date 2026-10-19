package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/provider"
)

// providerAliases maps every spelling a user or agent might type for a
// provider to its registered name: the CLI word (az, gh), the provider name,
// and the product name.
var providerAliases = map[string]string{
	"az": "azure", "azure": "azure",
	"gh": "github", "github": "github",
	"aws": "aws",
	"gcp": "gcp", "google": "gcp", "gcloud": "gcp",
}

// providerWord is the CLI group word for a provider name (azure → az).
func providerWord(name string) string {
	switch name {
	case "azure":
		return "az"
	case "github":
		return "gh"
	}
	return name
}

// selectProviders resolves an optional provider argument: none means every
// registered provider; one is matched through providerAliases, and an unknown
// spelling is a usage error.
func selectProviders(args []string) ([]provider.Provider, error) {
	all := provider.All()
	if len(args) == 0 {
		return all, nil
	}
	want, ok := providerAliases[strings.ToLower(args[0])]
	if ok {
		for _, p := range all {
			if p.Name() == want {
				return []provider.Provider{p}, nil
			}
		}
	}
	return nil, usageErr("azrl: unknown provider %q — use one of az, gh, aws, gcp", args[0])
}

// newListCmd is the inventory verb: every mapping, ambient default and
// unmapped profile. fixed pins it to one provider (the ghrl alias); empty
// takes an optional provider argument.
func newListCmd(fixed string) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "list [provider]",
		Short: "List mappings, ambient defaults and profiles (all providers, or one)",
		Long: "List the inventory: every directory mapping (MAPPINGS), each CLI's native\n" +
			"default identity (AMBIENT) and the profiles no directory maps to (UNMAPPED\n" +
			"PROFILES). Name a provider (az, gh, aws, gcp) to narrow it to one.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if fixed != "" {
				args = []string{fixed}
			}
			provs, err := selectProviders(args)
			if err != nil {
				return err
			}
			return runOverview(cmd, provs, asJSON)
		},
	}
	if fixed != "" {
		c.Use = "list"
		c.Short = "List mappings, the ambient default and profiles"
		c.Args = cobra.NoArgs
	}
	addJSONFlag(c, &asJSON, "the mappings / ambient / unmapped sections")
	return c
}

func init() { RootCmd.AddCommand(newListCmd("")) }

// groupRun makes a provider group runnable so an unknown verb is a usage
// error (exit 2) instead of cobra's silent help-and-exit-0; bare invocation
// still prints the group's help.
func groupRun(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageErr("azrl: unknown command %q for %q — see `%s --help`", args[0], cmd.CommandPath(), cmd.CommandPath())
	}
	return cmd.Help()
}
