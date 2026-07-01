package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/slamb2k/azrl/internal/provider"
	"github.com/spf13/cobra"
)

var statusJSON bool

// statusRow is one profile's cross-provider snapshot for `azrl status`.
type statusRow struct {
	Provider    string     `json:"provider"`
	ProfileName string     `json:"profileName"`
	Identity    string     `json:"identity"`
	Directory   string     `json:"directory"`
	Expiry      *time.Time `json:"expiry"`
	Drifted     bool       `json:"drifted"`
	LastUsed    time.Time  `json:"lastUsed"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show a cross-provider status dashboard (who am I, everywhere)",
	RunE: func(cmd *cobra.Command, args []string) error {
		var rows []statusRow
		for _, ps := range provider.Collect(provider.All()) {
			for _, st := range ps.Statuses {
				rows = append(rows, statusRow{
					Provider: ps.Title, ProfileName: st.ProfileName, Identity: st.Identity,
					Directory: st.Directory, Expiry: st.Expiry, Drifted: st.Drifted, LastUsed: st.LastUsed,
				})
			}
		}
		if statusJSON {
			b, err := json.MarshalIndent(rows, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-14s %-28s %-8s %s\n", "PROVIDER", "PROFILE", "IDENTITY", "DRIFT", "DIR")
		for _, r := range rows {
			drift := ""
			if r.Drifted {
				drift = "drift"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-14s %-28s %-8s %s\n", r.Provider, r.ProfileName, dash(r.Identity), drift, r.Directory)
		}
		return nil
	},
}

// dash renders a blank field as an em dash for the plain table.
func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output the snapshot as a JSON array")
	RootCmd.AddCommand(statusCmd)
}
