package azure

import (
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// Provider is the Azure implementation of provider.Provider. The profile
// mechanics delegate to azrl's existing profile functions (which run on the
// Azure Scheme); the az/ssh sign-in lifecycle stays in this package's other
// files and is orchestrated by the CLI.
type Provider struct{}

// NewProvider returns the Azure provider.
func NewProvider() provider.Provider { return Provider{} }

func (Provider) Name() string  { return "azure" }
func (Provider) Title() string { return "Azure" }

func (Provider) ProfilesDir() string { return config.ProfilesDir() }

func (Provider) Scheme() profile.Scheme { return profile.AzureScheme() }

func (Provider) ListProfiles(confdir string) ([]profile.Listed, error) {
	return profile.List(confdir)
}

func (Provider) Resolve(arg, dir string) (string, error) {
	return profile.Resolve(arg, dir)
}

func (Provider) Use(name, confdir, pwd string) error {
	return profile.Use(name, confdir, pwd)
}

func (Provider) Remove(name, confdir, pwd string) ([]string, error) {
	return profile.Remove(name, confdir, pwd)
}

func (Provider) SetLabel(name, confdir, label string) error {
	return profile.SetLabel(name, confdir, label)
}
