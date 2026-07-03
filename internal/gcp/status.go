package gcp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/provider"
)

// Status returns a disk-only snapshot of profile name from its conf and the
// gcloud configuration files. It never spawns gcloud or makes a network call.
// Expiry comes from the per-account token_expiry row in access_tokens.db,
// read via the pure-Go sqlittle reader (see gcpExpiry); nil when absent.
func (Provider) Status(name, confdir string) (provider.Status, error) {
	c, _ := LoadConf(name, confdir)
	last, dir := scheme.LastTouch(name, confdir)
	configName := c.ResolvedConfigName(name)
	gcDir := gcloudConfigDir(name, confdir, c.Isolate)
	last = provider.LatestMtime(last,
		filepath.Join(gcDir, "configurations", "config_"+configName),
		filepath.Join(gcDir, "active_config"),
		filepath.Join(gcDir, "credentials.db"),
		filepath.Join(gcDir, "access_tokens.db"))
	drifted := driftedDefault(name, confdir, configName)
	if c.Isolate {
		drifted = driftedIsolate(name, confdir)
	}
	identity := gcpIdentity(name, confdir, configName, c.Isolate)
	return provider.Status{
		ProfileName: name,
		Identity:    identity,
		Directory:   dir,
		Expiry:      gcpExpiry(gcDir, identity),
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
