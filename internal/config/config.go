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
	return g, nil
}
