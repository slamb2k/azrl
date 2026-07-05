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
		v = strings.TrimSpace(v)
		// Strip a trailing inline comment (whitespace followed by '#'), so a value
		// like `wslview   # opens a URL` yields `wslview`. A '#' not preceded by
		// whitespace is kept as a literal.
		for i := 1; i < len(v); i++ {
			if v[i] == '#' && (v[i-1] == ' ' || v[i-1] == '\t') {
				v = strings.TrimSpace(v[:i])
				break
			}
		}
		m[strings.TrimSpace(k)] = v
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

// Global holds the values from azrl.conf. The role-based keys BROWSER_CMD /
// BROWSER_HOST / VM_SSH_HOST are canonical; the legacy LOCAL_BROWSER_CMD /
// LOCAL_HOST / VM_HOST are read as aliases for backward compatibility.
type Global struct {
	BrowserCmd  string // command that opens a URL on BrowserHost
	BrowserHost string // SSH name of the machine that runs the browser ("" / localhost = this host)
	VMSSHHost   string // this VM's SSH name, used only in the path-A paste line
}

// placeholderHosts are the shipped sentinel host values; a config still carrying
// one was never configured (see IsPlaceholder).
var placeholderHosts = map[string]bool{"my-laptop": true, "my-vm": true}

// IsLocal reports whether azrl runs on the same machine as the browser, so the
// SSH bridge is bypassed and the OAuth callback loops back over localhost. True
// when BrowserHost is unset or names this host (localhost / 127.0.0.1) AND no
// VM_SSH_HOST is set — a VM_SSH_HOST-only box is remote.
func (g Global) IsLocal() bool {
	if g.VMSSHHost != "" {
		return false
	}
	switch g.BrowserHost {
	case "", "localhost", "127.0.0.1":
		return true
	}
	return false
}

// IsPlaceholder reports whether azrl.conf still carries a shipped sentinel host
// value (never configured). Advisory — it triggers the runtime setup nudge, not
// a LoadGlobal error.
func IsPlaceholder(g Global) bool {
	return placeholderHosts[g.BrowserHost] || placeholderHosts[g.VMSSHHost]
}

// firstNonEmpty returns the first non-empty argument, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// LoadGlobal reads <dir>/azrl.conf. Only BROWSER_CMD is required; both host keys
// are optional (a config with just BROWSER_CMD is local mode). Each new key
// falls back to its legacy alias when absent. The AZRL_BROWSER_CMD env var, when
// set, overrides BrowserCmd for this process (applied after validation).
func LoadGlobal(dir string) (Global, error) {
	var g Global
	path := filepath.Join(dir, "azrl.conf")
	f, err := os.Open(path)
	if err != nil {
		return g, fmt.Errorf("azrl: missing %s (run `azrl setup`): %w", path, err)
	}
	defer f.Close()
	m, err := ParseKV(f)
	if err != nil {
		return g, err
	}
	g = Global{
		BrowserCmd:  firstNonEmpty(m["BROWSER_CMD"], m["LOCAL_BROWSER_CMD"]),
		BrowserHost: firstNonEmpty(m["BROWSER_HOST"], m["LOCAL_HOST"]),
		VMSSHHost:   firstNonEmpty(m["VM_SSH_HOST"], m["VM_HOST"]),
	}
	if g.BrowserCmd == "" {
		return g, fmt.Errorf("azrl: BROWSER_CMD must be set in %s (run `azrl setup`)", path)
	}
	if v := os.Getenv("AZRL_BROWSER_CMD"); v != "" {
		g.BrowserCmd = v
	}
	return g, nil
}

// managedGlobalKeys are the keys Write itself emits (canonical plus legacy
// aliases). Any other KEY=value line in an existing azrl.conf (e.g. PROVIDERS,
// DASHBOARD_POLL_SECS) is preserved verbatim across a rewrite.
var managedGlobalKeys = map[string]bool{
	"BROWSER_CMD": true, "BROWSER_HOST": true, "VM_SSH_HOST": true,
	"LOCAL_BROWSER_CMD": true, "LOCAL_HOST": true, "VM_HOST": true,
}

// preservedGlobalLines returns the verbatim KEY=value lines from an existing
// azrl.conf whose key Write does not manage, plus the set of those keys, so a
// setup re-run keeps them. Comment and blank lines are dropped (Write re-adds its
// own); a missing/unreadable file yields nil.
func preservedGlobalLines(path string) ([]string, map[string]bool) {
	keys := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return nil, keys
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		raw := sc.Text()
		t := strings.TrimSpace(raw)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		k, _, ok := strings.Cut(t, "=")
		if !ok {
			continue
		}
		if k = strings.TrimSpace(k); managedGlobalKeys[k] {
			continue
		}
		lines = append(lines, raw)
		keys[k] = true
	}
	return lines, keys
}

// Write emits the global config to path with the new role-based keys and brief
// guiding comments. It is the single writer used by the setup wizard and
// `azrl setup --yes`; the caller is responsible for backing up any existing file.
// Unmanaged keys already present (PROVIDERS, DASHBOARD_POLL_SECS) are preserved.
func (g Global) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	extra, extraKeys := preservedGlobalLines(path)
	var b strings.Builder
	if g.IsLocal() {
		b.WriteString("# azrl config — local mode: the browser opens on this machine, no SSH bridge.\n")
		fmt.Fprintf(&b, "BROWSER_CMD=%s\n", g.BrowserCmd)
		fmt.Fprintf(&b, "BROWSER_HOST=%s\n", g.BrowserHost)
		b.WriteString("# VM_SSH_HOST=            # only needed for the remote path-A paste fallback\n")
	} else {
		b.WriteString("# azrl config — remote mode: browser opens on your dev machine over SSH.\n")
		fmt.Fprintf(&b, "BROWSER_CMD=%s              # runs on your dev machine\n", g.BrowserCmd)
		if g.BrowserHost != "" {
			fmt.Fprintf(&b, "BROWSER_HOST=%s     # set for zero-paste (VM must reach your machine)\n", g.BrowserHost)
		} else {
			b.WriteString("# BROWSER_HOST=my-laptop     # set for zero-paste (VM must reach your machine)\n")
		}
		fmt.Fprintf(&b, "VM_SSH_HOST=%s     # derived from $SSH_CONNECTION; edit for NAT/jump hosts\n", g.VMSSHHost)
	}
	if !extraKeys["DASHBOARD_POLL_SECS"] {
		b.WriteString("# DASHBOARD_POLL_SECS=3\n")
	}
	for _, l := range extra {
		b.WriteString(l + "\n")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	return os.Rename(tmp.Name(), path)
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
