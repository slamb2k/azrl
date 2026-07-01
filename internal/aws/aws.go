// Package aws drives the aws-SSO login lifecycle for the AWS provider: it
// manages per-account isolated config/credentials files, relays the PKCE
// loopback sign-in to the local browser over the shared SSH bridge, syncs the
// ~/.aws/config stanzas, and asserts the signed-in caller identity. Profile
// mechanics reuse the shared profile.Scheme.
package aws

import (
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// scheme carries the AWS profile mechanics: .awsprofile repo pins, the SSO start
// URL as the headline detail, AWS_LABEL as the display label, aws as the reserved
// global-conf basename.
var scheme = profile.Scheme{
	Pointer:   ".awsprofile",
	Reserved:  "aws",
	DetailKey: "AWS_SSO_START_URL",
	LabelKey:  "AWS_LABEL",
	Prefix:    "aws",
}

// Scheme returns the AWS profile Scheme.
func Scheme() profile.Scheme { return scheme }

// Provider is the AWS implementation of provider.Provider.
type Provider struct{}

// NewProvider returns the AWS provider.
func NewProvider() provider.Provider { return Provider{} }

func init() { provider.Register(NewProvider()) }

func (Provider) Name() string  { return "aws" }
func (Provider) Title() string { return "AWS" }

func (Provider) ProfilesDir() string { return config.AwsProfilesDir() }

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
