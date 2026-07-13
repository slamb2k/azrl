package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// deprecatedStatusCmd is a hidden stub: the top-level `status` was split into
// `whoami` (what governs this directory) and `profiles` (the everywhere
// overview). Same pattern as the removed `init`/`switch` verbs.
var deprecatedStatusCmd = &cobra.Command{
	Use:                "status",
	Hidden:             true,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("azrl: 'status' was split — use 'azrl whoami' (this directory) or 'azrl profiles' (everywhere)")
	},
}

func init() { RootCmd.AddCommand(deprecatedStatusCmd) }
