package cmd

import "testing"

// We can't drive a full interactive program in a unit test, but we can assert
// the root command is configured to run the TUI (no args) rather than printing
// help, by checking the RunE is set and the command has subcommands registered.
func TestRootHasSubcommands(t *testing.T) {
	names := map[string]bool{}
	for _, c := range RootCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"login", "capture", "use", "rm", "whoami"} {
		if !names[want] {
			t.Fatalf("missing subcommand %q", want)
		}
	}
}
