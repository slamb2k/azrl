// Package config reads azrl's KEY=value config files and resolves paths.
package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ParseKV parses simple KEY=value lines, trimming surrounding whitespace and
// skipping blank lines and lines beginning with '#'.
func ParseKV(r io.Reader) (map[string]string, error) {
	m := make(map[string]string)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		m[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return m, sc.Err()
}

// ProfilesDir returns ~/.azure-profiles.
func ProfilesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".azure-profiles")
}

// GithubProfilesDir returns ~/.github-profiles, the root for GitHub profile
// confs and their per-profile GH_CONFIG_DIRs.
func GithubProfilesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".github-profiles")
}

// AwsProfilesDir returns ~/.aws-profiles, the root for AWS profile confs and
// their per-profile isolated config/credentials dirs.
func AwsProfilesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws-profiles")
}

// GcpProfilesDir returns ~/.gcp-profiles, the root for GCP profile confs and
// their per-profile isolated CLOUDSDK_CONFIG dirs.
func GcpProfilesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gcp-profiles")
}

// DashboardPollSecs reads DASHBOARD_POLL_SECS from <dir>/azrl.conf, returning 3
// on any missing/parse failure or a non-positive value. It does not require the
// other azrl.conf keys, so the dashboard works standalone.
func DashboardPollSecs(dir string) int {
	f, err := os.Open(filepath.Join(dir, "azrl.conf"))
	if err != nil {
		return 3
	}
	defer f.Close()
	m, err := ParseKV(f)
	if err != nil {
		return 3
	}
	n, err := strconv.Atoi(m["DASHBOARD_POLL_SECS"])
	if err != nil || n <= 0 {
		return 3
	}
	return n
}

// Global holds the values from azrl.conf.
type Global struct {
	LocalHost       string
	LocalBrowserCmd string
	VMHost          string
}

// LoadGlobal reads <dir>/azrl.conf and validates all three fields are present.
// The AZRL_BROWSER_CMD env var, when set, overrides LocalBrowserCmd.
func LoadGlobal(dir string) (Global, error) {
	var g Global
	path := filepath.Join(dir, "azrl.conf")
	f, err := os.Open(path)
	if err != nil {
		return g, fmt.Errorf("azrl: missing %s (run install.sh): %w", path, err)
	}
	defer f.Close()
	m, err := ParseKV(f)
	if err != nil {
		return g, err
	}
	g = Global{LocalHost: m["LOCAL_HOST"], LocalBrowserCmd: m["LOCAL_BROWSER_CMD"], VMHost: m["VM_HOST"]}
	if g.LocalHost == "" || g.LocalBrowserCmd == "" || g.VMHost == "" {
		return g, fmt.Errorf("azrl: LOCAL_HOST, LOCAL_BROWSER_CMD and VM_HOST must all be set in %s", path)
	}
	// AZRL_BROWSER_CMD overrides the browser command for this process only:
	// set per-profile by the cmd layer, or exported by the user as an escape
	// hatch. Applied after validation so azrl.conf must still be complete.
	if v := os.Getenv("AZRL_BROWSER_CMD"); v != "" {
		g.LocalBrowserCmd = v
	}
	return g, nil
}

// defaultProviders is the tab set shown when azrl.conf carries no PROVIDERS
// key: the flagship provider plus GitHub.
var defaultProviders = []string{"azure", "github"}

// EnabledProviders reads the comma-separated PROVIDERS key from
// <dir>/azrl.conf — the provider tabs the TUI shows. A missing file or key
// (or an empty value) yields the default: azure, github.
func EnabledProviders(dir string) []string {
	def := append([]string(nil), defaultProviders...)
	f, err := os.Open(filepath.Join(dir, "azrl.conf"))
	if err != nil {
		return def
	}
	defer f.Close()
	kv, err := ParseKV(f)
	if err != nil {
		return def
	}
	var out []string
	for _, p := range strings.Split(kv["PROVIDERS"], ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

// SetEnabledProviders persists the PROVIDERS key into <dir>/azrl.conf,
// replacing an existing assignment and preserving every other line (comments
// included). The directory and file are created when missing; the rewrite is
// atomic.
func SetEnabledProviders(dir string, names []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "azrl.conf")
	entry := "PROVIDERS=" + strings.Join(names, ",")
	var lines []string
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimRight(string(b), "\n"); s != "" {
			lines = strings.Split(s, "\n")
		}
	}
	replaced := false
	for i, l := range lines {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "PROVIDERS=") {
			lines[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, entry)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
