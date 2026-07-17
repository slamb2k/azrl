package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// isInteractive reports whether stdin is a terminal. It is a package var so
// tests can stub the picker's TTY branch deterministically.
var isInteractive = func() bool { return term.IsTerminal(os.Stdin.Fd()) }

// confirmCreateProfile reports whether to create a missing profile. With
// assumeYes it returns true without prompting. On a non-interactive stream
// without assumeYes it returns false (the caller should error with guidance).
// Otherwise it prompts "[y/N]" (default No) reading from cmd.InOrStdin().
func confirmCreateProfile(cmd *cobra.Command, label, name, detail string, assumeYes bool) bool {
	if assumeYes {
		return true
	}
	if !isInteractive() {
		return false
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s: profile %q doesn't exist. Create it (%s)? [y/N]: ", label, name, detail)
	sc := bufio.NewScanner(cmd.InOrStdin())
	if !sc.Scan() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(sc.Text())) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// resolveLoginTarget returns the profile to log into: the explicit arg, else the
// directory-pinned profile, else an interactive pick among existing profiles.
// label is the provider's CLI label ("ghrl", "azrl", "azrl aws", "azrl gcp"),
// used in prompts and errors. validate guards a name entered at the first-login
// prompt. newProfile is true only when the caller should create the returned
// name (the interactive zero-profile first-login path). Errors here are runtime
// errors (no usage dump).
func resolveLoginTarget(cmd *cobra.Command, prov provider.Provider, args []string, label string, validate func(string) error) (name string, newProfile bool, err error) {
	name, _, newProfile, err = resolveLoginTargetWithProfiles(cmd, prov, args, label, validate)
	return name, newProfile, err
}

// resolveLoginTargetWithProfiles is resolveLoginTarget plus the profile slice it
// listed while deciding. profs is nil when it short-circuited on an explicit arg
// or a directory pin (no listing needed) or when the directory read failed; it
// is a non-nil (possibly empty) slice whenever the listing succeeded. Azure login
// uses that distinction to trigger its tenant-less fallback on zero saved
// profiles without a second directory read, while still propagating read errors.
//
// With zero saved profiles on a TTY it runs the first-login prompt: it asks for a
// name (defaulting to the current directory basename), validates it, and returns
// newProfile=true so the caller creates and signs in without a second [y/N]
// confirm. Non-interactively it preserves the original "no profiles yet" error.
func resolveLoginTargetWithProfiles(cmd *cobra.Command, prov provider.Provider, args []string, label string, validate func(string) error) (name string, profs []profile.Listed, newProfile bool, err error) {
	if len(args) == 1 {
		return args[0], nil, false, nil
	}
	pwd, _ := os.Getwd()
	if pin, _ := prov.Resolve("", pwd); pin != "" {
		return pin, nil, false, nil
	}
	out := cmd.OutOrStdout()
	profs, err = prov.ListProfiles(prov.ProfilesDir())
	if err != nil {
		return "", nil, false, err
	}
	if profs == nil {
		profs = []profile.Listed{}
	}
	switch len(profs) {
	case 0:
		if !isInteractive() {
			return "", profs, false, fmt.Errorf(`%s: no profiles yet — run "%s login <name>" to create one`, label, label)
		}
		def := profile.DefaultName("", pwd)
		sc := bufio.NewScanner(cmd.InOrStdin())
		for attempt := 0; attempt < 3; attempt++ {
			fmt.Fprintf(out, "No %s profiles yet. Name for this account [%s]: ", label, def)
			chosen := def
			if sc.Scan() {
				if t := strings.TrimSpace(sc.Text()); t != "" {
					chosen = t
				}
			}
			if verr := validate(chosen); verr != nil {
				fmt.Fprintf(out, "%s: %v\n", label, verr)
				continue
			}
			return chosen, profs, true, nil
		}
		return "", profs, false, fmt.Errorf("%s: no valid profile name provided", label)
	case 1:
		fmt.Fprintf(out, "%s: using the only profile %q\n", label, profs[0].Name)
		return profs[0].Name, profs, false, nil
	}
	names := make([]string, len(profs))
	for i, p := range profs {
		names[i] = p.Name
	}
	if !isInteractive() {
		return "", profs, false, fmt.Errorf("%s: multiple profiles — specify one of: %s", label, strings.Join(names, ", "))
	}
	for i, p := range profs {
		fmt.Fprintf(out, "  %d) %-24s %s\n", i+1, p.Display(), p.Detail)
	}
	sc := bufio.NewScanner(cmd.InOrStdin())
	for attempt := 0; attempt < 3; attempt++ {
		fmt.Fprintf(out, "Select a profile [1-%d]: ", len(profs))
		if !sc.Scan() {
			break
		}
		text := strings.TrimSpace(sc.Text())
		if n, aerr := strconv.Atoi(text); aerr == nil && n >= 1 && n <= len(profs) {
			return profs[n-1].Name, profs, false, nil
		}
		fmt.Fprintf(out, "%s: not a choice in [1-%d]: %q\n", label, len(profs), text)
	}
	return "", profs, false, fmt.Errorf("%s: no valid profile selected", label)
}
