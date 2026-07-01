package aws

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/slamb2k/azrl/internal/provider"
)

// Status returns a disk-only snapshot of profile name from its conf and the
// shared SSO token cache. It never spawns aws or makes a network call.
func (Provider) Status(name, confdir string) (provider.Status, error) {
	c, _ := LoadConf(name, confdir)
	last, dir := scheme.LastTouch(name, confdir)
	last = provider.LatestMtime(last, awsCacheFiles(c.SSOStartURL)...)
	drifted := provider.Drifted(scheme, "AWS_PROFILE", name, name)
	if c.Isolate {
		drifted = driftedIsolate(name, confdir)
	}
	return provider.Status{
		ProfileName: name,
		Identity:    awsIdentity(c),
		Directory:   dir,
		Expiry:      awsExpiry(c.SSOStartURL),
		Drifted:     drifted,
		LastUsed:    last,
	}, nil
}

// awsIdentity renders the account/role this profile targets from its conf; blank
// when neither is set.
func awsIdentity(c Conf) string {
	if c.AccountID == "" && c.RoleName == "" {
		return ""
	}
	return c.AccountID + "/" + c.RoleName
}

// awsCacheFiles returns the ~/.aws/sso/cache/*.json paths whose cached token
// matches startURL, so their mtime can fold into LastUsed. Best-effort: an
// empty URL or unreadable cache yields no paths.
func awsCacheFiles(startURL string) []string {
	if startURL == "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	cacheDir := filepath.Join(home, ".aws", "sso", "cache")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(cacheDir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var t struct {
			StartURL string `json:"startUrl"`
		}
		if json.Unmarshal(b, &t) != nil || t.StartURL != startURL {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

// awsExpiry returns the latest token expiry from ~/.aws/sso/cache for the given
// SSO start URL; nil on any error or when no matching cached token is present.
func awsExpiry(startURL string) *time.Time {
	if startURL == "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(home, ".aws", "sso", "cache"))
	if err != nil {
		return nil
	}
	var latest *time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(home, ".aws", "sso", "cache", e.Name()))
		if err != nil {
			continue
		}
		var t struct {
			StartURL  string `json:"startUrl"`
			ExpiresAt string `json:"expiresAt"`
		}
		if json.Unmarshal(b, &t) != nil || t.StartURL != startURL {
			continue
		}
		exp, err := time.Parse(time.RFC3339, t.ExpiresAt)
		if err != nil {
			continue
		}
		if latest == nil || exp.After(*latest) {
			e := exp
			latest = &e
		}
	}
	return latest
}
