package github

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
)

// Conf holds a per-profile GitHub configuration. Host is the account's server
// (github.com, a *.ghe.com tenant, or a GHES hostname); User is the expected
// login for post-auth assertion; Label is an optional display name; Protocol is
// https (GCM/bridge path) or ssh (informational).
type Conf struct {
	Host     string
	User     string
	Label    string
	Protocol string
}

// LoadConf reads <confdir>/<name>.conf and requires GH_HOST. Protocol defaults
// to https when unset.
func LoadConf(name, confdir string) (Conf, error) {
	var c Conf
	path := filepath.Join(confdir, name+".conf")
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("ghrl: missing config %s: %w", path, err)
	}
	defer f.Close()
	m, err := config.ParseKV(f)
	if err != nil {
		return c, err
	}
	c = Conf{Host: m["GH_HOST"], User: m["GH_USER"], Label: m["GH_LABEL"], Protocol: m["GH_PROTOCOL"]}
	if c.Host == "" {
		return c, fmt.Errorf("ghrl: GH_HOST not set in %s", path)
	}
	if c.Protocol == "" {
		c.Protocol = "https"
	}
	return c, nil
}

// Write atomically writes the conf in the canonical KEY=value format.
func (c Conf) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	protocol := c.Protocol
	if protocol == "" {
		protocol = "https"
	}
	body := fmt.Sprintf("GH_HOST=%s\nGH_USER=%s\nGH_LABEL=%s\nGH_PROTOCOL=%s\n",
		c.Host, c.User, c.Label, protocol)
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	return os.Rename(tmp.Name(), path)
}
