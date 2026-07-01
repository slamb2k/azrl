package gcp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/provider"
)

// Status returns a disk-only snapshot of profile name from its conf and the
// gcloud configuration files. It never spawns gcloud or makes a network call.
// Expiry is always nil in v1: gcloud caches token expiry in access_tokens.db
// (SQLite), which cannot be read disk-only without a new dependency.
func (Provider) Status(name, confdir string) (provider.Status, error) {
	c, _ := LoadConf(name, confdir)
	last, dir := scheme.LastTouch(name, confdir)
	configName := c.ResolvedConfigName(name)
	gcDir := gcloudConfigDir(name, confdir, c.Isolate)
	last = provider.LatestMtime(last,
		filepath.Join(gcDir, "configurations", "config_"+configName),
		filepath.Join(gcDir, "active_config"),
		filepath.Join(gcDir, "credentials.db"))
	drifted := driftedDefault(name, confdir, configName)
	if c.Isolate {
		drifted = driftedIsolate(name, confdir)
	}
	return provider.Status{
		ProfileName: name,
		Identity:    gcpIdentity(name, confdir, configName, c.Isolate),
		Directory:   dir,
		Expiry:      nil,
		Drifted:     drifted,
		LastUsed:    last,
	}, nil
}

// gcpIdentity reads the active account from the plain-text INI configuration
// file (configurations/config_<configName>, [core] account); blank on any error.
func gcpIdentity(name, confdir, configName string, isolate bool) string {
	path := filepath.Join(gcloudConfigDir(name, confdir, isolate), "configurations", "config_"+configName)
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return iniValue(string(b), "core", "account")
}

// iniValue returns the value of key under [section] in a gcloud-style INI body,
// or "" when absent.
func iniValue(body, section, key string) string {
	cur := ""
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			cur = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if cur != section {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
