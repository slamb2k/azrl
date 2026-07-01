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

// newBrowserShimCmd builds the smart browser shim tools invoke to open a URL. It
// classifies the URL and either relays it to the local browser (device/plain) or
// tunnels its loopback callback port back to the VM (OAuth loopback).
func newBrowserShimCmd() *cobra.Command {
	var paste bool
	c := &cobra.Command{
		Use:    "__browser [url]",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := config.LoadGlobal(config.ProfilesDir())
			// Without global config we cannot reach the laptop over SSH — fall back
			// to printing the URL for the user to open locally.
			forcePaste := paste || err != nil
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
	c.Flags().BoolVar(&paste, "paste", false, "print a paste line instead of bridging over SSH")
	return c
}

func init() { RootCmd.AddCommand(newBrowserShimCmd()) }
