package github

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/slamb2k/azrl/internal/provider"
)

// Ambient returns the identity gh itself would use right now, read from
// ${GH_CONFIG_DIR:-~/.config/gh}/hosts.yml and rendered user@host. Disk-only
// and best-effort: it never spawns gh, and missing or unparseable state
// yields the zero value.
func (Provider) Ambient() (provider.Ambient, error) {
	dir := os.Getenv("GH_CONFIG_DIR")
	source := "file:$GH_CONFIG_DIR/hosts.yml"
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return provider.Ambient{}, nil
		}
		dir = filepath.Join(home, ".config", "gh")
		source = "file:~/.config/gh/hosts.yml"
	}
	id := ambientIdentity(filepath.Join(dir, "hosts.yml"))
	if id == "" {
		return provider.Ambient{}, nil
	}
	return provider.Ambient{Identity: id, Source: source}, nil
}

// ambientIdentity reads hosts.yml and renders the active user as user@host,
// preferring github.com over other hosts. It supports both the legacy
// single-user shape (a host-level user key) and the modern multi-account
// shape (a users map, with the host-level user naming the active account);
// an unknown shape yields "" rather than an error.
func ambientIdentity(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var hosts map[string]struct {
		User  string               `yaml:"user"`
		Users map[string]yaml.Node `yaml:"users"`
	}
	if yaml.Unmarshal(b, &hosts) != nil {
		return ""
	}
	pick := func(host string) string {
		h := hosts[host]
		if h.User != "" {
			return h.User + "@" + host
		}
		if len(h.Users) == 1 {
			for u := range h.Users {
				return u + "@" + host
			}
		}
		return ""
	}
	if id := pick("github.com"); id != "" {
		return id
	}
	names := make([]string, 0, len(hosts))
	for host := range hosts {
		names = append(names, host)
	}
	sort.Strings(names)
	for _, host := range names {
		if id := pick(host); id != "" {
			return id
		}
	}
	return ""
}
