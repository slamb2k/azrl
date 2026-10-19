package cmd

import (
	"encoding/json"
	"io"

	"github.com/spf13/cobra"
)

// writeJSON emits v as indented JSON followed by a newline.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// jsonStdout reserves stdout for a --json document: it returns the command's
// current stdout and routes everything else the command prints (progress,
// notes, cmd.Printf) to stderr until restore is called, so `$(azrl … --json)`
// captures only the document.
func jsonStdout(cmd *cobra.Command) (stdout io.Writer, restore func()) {
	stdout = cmd.OutOrStdout()
	cmd.SetOut(cmd.ErrOrStderr())
	return stdout, func() { cmd.SetOut(nil) }
}

// captureResult is the --json document every provider's capture prints.
type captureResult struct {
	Provider string `json:"provider"`
	Profile  string `json:"profile"`
	Identity string `json:"identity,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Created  bool   `json:"created"`
}

// addJSONFlag registers the shared --json flag.
func addJSONFlag(c *cobra.Command, target *bool, what string) {
	c.Flags().BoolVar(target, "json", false, "Print "+what+" as JSON on stdout (progress goes to stderr)")
}
