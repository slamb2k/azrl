package profile

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ErrInvalidName marks a profile name that cannot be used as a path component
// under a profiles dir. Match it with errors.Is.
var ErrInvalidName = errors.New("invalid profile name")

// ValidateName is the single rule every profile name must pass before it is
// joined onto a profiles dir — typed on the command line, read from a pointer
// file in a (possibly untrusted) repo, or loaded from disk. A name becomes
// <confdir>/<name>.conf and <confdir>/<name>/, so it must stay one plain child
// of confdir: non-empty, no path separators, not "." or "..", no leading dot
// (hidden files, and any ".."-prefixed trick), and no control characters
// (newlines would also corrupt pointer files and the mappings TSV).
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty", ErrInvalidName)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("%w %q: must not start with '.'", ErrInvalidName, name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w %q: must not contain a path separator", ErrInvalidName, name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w %q: must not contain control characters", ErrInvalidName, name)
		}
	}
	return nil
}
