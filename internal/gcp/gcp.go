// Package gcp drives the Google Cloud (`gcloud auth login`) login lifecycle for
// the GCP provider: it manages native gcloud named configurations, relays the
// loopback sign-in to the local browser over the shared SSH bridge, syncs the
// configuration's project/region, and asserts the signed-in account. Profile
// mechanics reuse the shared profile.Scheme.
package gcp

import (
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// scheme carries the GCP profile mechanics: .gcpprofile repo pins, the bound
// project as the headline detail, GCP_LABEL as the display label, gcp as the
// reserved global-conf basename.
var scheme = profile.Scheme{
	Pointer:   ".gcpprofile",
	Reserved:  "gcp",
	DetailKey: "GCP_PROJECT",
	LabelKey:  "GCP_LABEL",
	Prefix:    "gcp",
}

// Scheme returns the GCP profile Scheme.
func Scheme() profile.Scheme { return scheme }

// Provider is the GCP implementation of provider.Provider.
type Provider struct{}

// NewProvider returns the GCP provider.
func NewProvider() provider.Provider { return Provider{} }

func init() { provider.Register(NewProvider()) }

func (Provider) Name() string  { return "gcp" }
func (Provider) Title() string { return "Google Cloud" }

func (Provider) ProfilesDir() string { return config.GcpProfilesDir() }

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
