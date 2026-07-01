// Package provider defines the account-type abstraction (Azure, GitHub, …) that
// the CLI and tabbed TUI drive generically. Each provider lives in its own
// package (internal/azure, internal/github) and is validated by the shared
// contract suite in providertest.
package provider

import "github.com/slamb2k/azrl/internal/profile"

// Provider is one login target. The profile-mechanic methods delegate to the
// provider's Scheme so behaviour stays identical across providers; provider-
// specific sign-in orchestration lives on the concrete type, not here.
type Provider interface {
	// Name is the stable identifier ("azure", "github").
	Name() string
	// Title is the human-facing tab/label ("Azure", "GitHub").
	Title() string
	// ProfilesDir is the root holding <name>.conf files and per-profile config dirs.
	ProfilesDir() string
	// Scheme is the parameterized profile mechanics (pointer file, keys, reserved name).
	Scheme() profile.Scheme
	// ListProfiles returns the provider's profiles under confdir.
	ListProfiles(confdir string) ([]profile.Listed, error)
	// Resolve returns the active profile for dir (explicit arg or pointer walk-up).
	Resolve(arg, dir string) (string, error)
	// Use pins pwd to the named profile.
	Use(name, confdir, pwd string) error
	// Remove deletes the named profile, returning the removed paths.
	Remove(name, confdir, pwd string) ([]string, error)
	// SetLabel sets a profile's display label ("" reverts to the slug).
	SetLabel(name, confdir, label string) error
}
