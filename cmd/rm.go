package cmd

import (
	"fmt"
	"strings"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

// refuseIfLinked is the read-only half of a rm command's mapping-awareness:
// with no mapped dirs it's a no-op; with mappings and neither flag it
// refuses, listing each dir and both flags; with --unmap-all or --replace it
// approves without mutating anything — the actual unmap/repoint happens in
// unlinkOrReplace, called only after any confirmation prompt.
func refuseIfLinked(scheme profile.Scheme, confdir, name string, unlinkAll bool, replace string) error {
	if unlinkAll || replace != "" {
		return nil
	}
	dirs := scheme.LinkedDirs(confdir, name)
	if len(dirs) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: profile %q is mapped from:\n", scheme.Prefix, name)
	for _, d := range dirs {
		fmt.Fprintf(&b, "  %s\n", d)
	}
	b.WriteString("use --unmap-all to remove the mappings, or --replace <profile> to repoint them")
	return fmt.Errorf("%s", b.String())
}

// unlinkOrReplace is the mutating half: --unmap-all removes every mapping;
// --replace repoints every mapping at the given profile (erroring if that
// profile doesn't exist, or is the profile being removed). No-op otherwise.
func unlinkOrReplace(cmd *cobra.Command, scheme profile.Scheme, confdir, name string, unlinkAll bool, replace string) error {
	switch {
	case unlinkAll:
		unlinked, err := scheme.UnlinkAll(confdir, name)
		for _, d := range unlinked {
			fmt.Fprintf(cmd.ErrOrStderr(), "unmapped %s\n", d)
		}
		return err
	case replace != "":
		repointed, err := scheme.ReplaceLinks(confdir, name, replace)
		for _, d := range repointed {
			fmt.Fprintf(cmd.ErrOrStderr(), "repointed %s -> %s\n", d, replace)
		}
		return err
	default:
		return nil
	}
}
