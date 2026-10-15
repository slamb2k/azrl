package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/provider"
)

// Exit codes are the agent contract: a caller (human script or LLM agent)
// branches on these instead of parsing error prose. Anything not wrapped in
// an exitError exits 1.
const (
	ExitOK         = 0
	ExitError      = 1
	ExitUsage      = 2 // bad/missing input, or a prompt was needed with no TTY
	ExitNeedsHuman = 3 // an interactive browser sign-in is required
	ExitNotFound   = 4 // the named profile does not exist
)

// exitError tags an error with the process exit code it should produce. The
// message is the wrapped error's, so callers and tests see the same text.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func withCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &exitError{code: code, err: err}
}

func usageErr(format string, a ...any) error {
	return withCode(ExitUsage, fmt.Errorf(format, a...))
}

func needsHumanErr(format string, a ...any) error {
	return withCode(ExitNeedsHuman, fmt.Errorf(format, a...))
}

func notFoundErr(format string, a ...any) error {
	return withCode(ExitNotFound, fmt.Errorf(format, a...))
}

// ExitCode maps an Execute error to the process exit status.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return ExitError
}

// reachedRun is set once a command's RunE starts. An error returned before
// that point came from cobra's own argument, flag or flag-group validation
// (or an unknown command) — a usage error by definition.
var reachedRun bool

// tagUsageErrors wraps every runnable command in the tree so executeTagged
// can tell cobra's pre-run validation failures from the command's own.
func tagUsageErrors(root *cobra.Command) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if run := c.RunE; run != nil {
			c.RunE = func(cmd *cobra.Command, args []string) error {
				reachedRun = true
				return run(cmd, args)
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

// executeTagged runs root and maps any failure cobra raised before the
// command ran to ExitUsage, leaving errors that already carry a code alone.
func executeTagged(root *cobra.Command) error {
	reachedRun = false
	err := root.Execute()
	var ee *exitError
	if err != nil && !reachedRun && !errors.As(err, &ee) {
		return withCode(ExitUsage, err)
	}
	return err
}

// requireProfile returns ExitNotFound when the provider has no conf for name.
// tool is the CLI label used in the message ("azrl", "ghrl", "azrl aws", …).
func requireProfile(tool string, prov provider.Provider, name string) error {
	if _, err := os.Stat(filepath.Join(prov.ProfilesDir(), name+".conf")); err != nil {
		return notFoundErr("%s: no profile %q", tool, name)
	}
	return nil
}

// profileLoadErr formats a conf-load failure, tagging it ExitNotFound when
// the conf file simply does not exist.
func profileLoadErr(err error, format string, a ...any) error {
	wrapped := fmt.Errorf(format, a...)
	if errors.Is(err, fs.ErrNotExist) {
		return withCode(ExitNotFound, wrapped)
	}
	return wrapped
}

// confirmDestructive asks "[y/N]" on a TTY; with no TTY it refuses with a
// usage error naming the flag, so an unattended caller never deletes by
// accident and never hangs.
func confirmDestructive(cmd *cobra.Command, tool, question string, assumeYes bool) error {
	if assumeYes {
		return nil
	}
	if !isInteractive() {
		return usageErr("%s: confirmation required — pass --yes to proceed without a TTY", tool)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", question)
	if !readYes(cmd.InOrStdin()) {
		return fmt.Errorf("%s: aborted", tool)
	}
	return nil
}
