// Package github drives the gh/GCM login lifecycle for the GitHub provider: it
// manages per-account isolated GH_CONFIG_DIRs, relays the device-code sign-in to
// the local browser, wires git-HTTPS credentials, and asserts the signed-in
// account. Profile mechanics reuse the shared profile.Scheme.
package github

import (
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// scheme carries the GitHub profile mechanics: .ghprofile repo pins, GH_HOST as
// the headline detail, GH_LABEL as the display label, ghrl as the reserved
// global-conf basename.
var scheme = profile.Scheme{
	Pointer:   ".ghprofile",
	Reserved:  "ghrl",
	DetailKey: "GH_HOST",
	LabelKey:  "GH_LABEL",
	Prefix:    "ghrl",
}

// Scheme returns the GitHub profile Scheme.
func Scheme() profile.Scheme { return scheme }

// ConfigDir returns the isolated GH_CONFIG_DIR for a profile: <profilesDir>/<name>.
func ConfigDir(profilesDir, name string) string {
	return filepath.Join(profilesDir, name)
}

// Provider is the GitHub implementation of provider.Provider.
type Provider struct{}

// NewProvider returns the GitHub provider.
func NewProvider() provider.Provider { return Provider{} }

func (Provider) Name() string  { return "github" }
func (Provider) Title() string { return "GitHub" }

func (Provider) ProfilesDir() string { return config.GithubProfilesDir() }

func (Provider) Scheme() profile.Scheme { return scheme }

func (Provider) ListProfiles(confdir string) ([]profile.Listed, error) {
	return scheme.List(confdir)
}

func (Provider) Resolve(arg, dir string) (string, error) {
	return scheme.Resolve(arg, dir)
}

func (Provider) Use(name, confdir, pwd string) error {
	return scheme.Use(name, confdir, pwd)
}

func (Provider) Remove(name, confdir, pwd string) ([]string, error) {
	return scheme.Remove(name, confdir, pwd)
}

func (Provider) SetLabel(name, confdir, label string) error {
	return scheme.SetLabel(name, confdir, label)
}
