package cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/browsercapture"
	"github.com/slamb2k/azrl/internal/config"
)

var browserPaste bool

// browserHold returns the auth window a loopback tunnel is held open, from
// AZRL_BROWSER_HOLD seconds (default 180).
func browserHold() time.Duration {
	if s := os.Getenv("AZRL_BROWSER_HOLD"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return 180 * time.Second
}

// browserShimCmd is the smart browser shim tools invoke to open a URL. It
// classifies the URL and either relays it to the local browser (device/plain)
// or tunnels its loopback callback port back to the VM (OAuth loopback).
var browserShimCmd = &cobra.Command{
	Use:    "__browser [url]",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		g, err := config.LoadGlobal(config.ProfilesDir())
		// Without global config we cannot reach the laptop over SSH — fall back
		// to printing the URL for the user to open locally.
		forcePaste := browserPaste || err != nil
		msg, rerr := browsercapture.Run(args[0], g, forcePaste, browserHold())
		if rerr != nil {
			return rerr
		}
		if msg != "" {
			fmt.Fprintln(cmd.OutOrStdout(), msg)
		}
		return nil
	},
}

func init() {
	browserShimCmd.Flags().BoolVar(&browserPaste, "paste", false, "print a paste line instead of bridging over SSH")
	RootCmd.AddCommand(browserShimCmd)
}
