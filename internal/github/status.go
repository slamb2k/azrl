package github

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/slamb2k/azrl/internal/provider"
)

// Status returns a disk-only snapshot of profile name from its isolated
// GH_CONFIG_DIR (<confdir>/<name>) and conf file. gh tokens don't expire, so
// Expiry is always nil. It never spawns gh.
func (Provider) Status(name, confdir string) (provider.Status, error) {
	isolated := filepath.Join(confdir, name)
	last, dir := scheme.LastTouch(name, confdir)
	return provider.Status{
		ProfileName: name,
		Identity:    githubIdentity(isolated),
		Directory:   dir,
		Expiry:      nil,
		Drifted:     provider.Drifted(scheme, "GH_CONFIG_DIR", name, isolated),
		LastUsed:    last,
	}, nil
}

// githubIdentity reads the signed-in user@host from hosts.yml; blank on any error.
func githubIdentity(confdir string) string {
	b, err := os.ReadFile(filepath.Join(confdir, "hosts.yml"))
	if err != nil {
		return ""
	}
	var hosts map[string]struct {
		User string `yaml:"user"`
	}
	if yaml.Unmarshal(b, &hosts) != nil {
		return ""
	}
	for host, h := range hosts {
		if h.User != "" {
			return h.User + "@" + host
		}
		return host
	}
	return ""
}
