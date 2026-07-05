// Package browserpick discovers the local machine's Chromium browser profiles
// (Edge, Chrome) over ssh so an azrl profile can be mapped to one. Read-only
// and best-effort: every failure degrades to manual command entry.
package browserpick

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"github.com/slamb2k/azrl/internal/config"
)

// safeDir matches Chromium profile directory names we're willing to splice
// into a shell command line (defense-in-depth: Dir comes from a remote
// machine's Local State and lands inside a login command).
var safeDir = regexp.MustCompile(`^[A-Za-z0-9 ._-]+$`)

// Profile is one browser profile discovered on the local machine.
type Profile struct {
	Browser string // "edge" or "chrome"
	OS      string // "linux", "macos", "wsl" or "windows"
	Dir     string // Chromium profile directory name, e.g. "Profile 2"
	Name    string // display name from the browser's profile switcher
	Email   string // signed-in account email; "" when not signed in
}

// Label is the human-facing name used in pickers and *_BROWSER_LABEL keys.
func (p Profile) Label() string {
	b := "Edge"
	if p.Browser == "chrome" {
		b = "Chrome"
	}
	return b + " — " + p.Name
}

// Command renders the local launch command; the bridge appends the sign-in
// URL exactly as it does for LOCAL_BROWSER_CMD.
func (p Profile) Command() string {
	pd := fmt.Sprintf("--profile-directory=%q", p.Dir)
	switch p.OS {
	case "macos":
		app := "Microsoft Edge"
		if p.Browser == "chrome" {
			app = "Google Chrome"
		}
		return fmt.Sprintf("open -na %q --args %s", app, pd)
	case "wsl":
		exe := "/mnt/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"
		if p.Browser == "chrome" {
			exe = "/mnt/c/Program Files/Google/Chrome/Application/chrome.exe"
		}
		return fmt.Sprintf("%q %s", exe, pd)
	case "windows":
		exe := `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`
		if p.Browser == "chrome" {
			exe = `C:\Program Files\Google\Chrome\Application\chrome.exe`
		}
		return fmt.Sprintf(`"%s" %s`, exe, pd)
	default: // linux
		bin := "microsoft-edge"
		if p.Browser == "chrome" {
			bin = "google-chrome"
		}
		return bin + " " + pd
	}
}

// Keys returns the per-profile conf key names for a provider's mapping.
func Keys(provider string) (cmdKey, labelKey string) {
	switch provider {
	case "azure":
		return "AZ_BROWSER_CMD", "AZ_BROWSER_LABEL"
	case "github":
		return "GH_BROWSER_CMD", "GH_BROWSER_LABEL"
	case "aws":
		return "AWS_BROWSER_CMD", "AWS_BROWSER_LABEL"
	case "gcp":
		return "GCP_BROWSER_CMD", "GCP_BROWSER_LABEL"
	}
	return "", ""
}

// localState mirrors the fragment of Chromium's Local State we read.
type localState struct {
	Profile struct {
		InfoCache map[string]struct {
			Name     string `json:"name"`
			UserName string `json:"user_name"`
		} `json:"info_cache"`
	} `json:"profile"`
}

// parseLocalState decodes one Local State document; nil on malformed JSON.
func parseLocalState(browser, osName string, data []byte) []Profile {
	var ls localState
	if err := json.Unmarshal(data, &ls); err != nil {
		return nil
	}
	var out []Profile
	for dir, info := range ls.Profile.InfoCache {
		if !safeDir.MatchString(dir) {
			continue
		}
		name := info.Name
		if name == "" {
			name = dir
		}
		out = append(out, Profile{Browser: browser, OS: osName, Dir: dir, Name: name, Email: info.UserName})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Browser != out[j].Browser {
			return out[i].Browser < out[j].Browser
		}
		return out[i].Dir < out[j].Dir
	})
	return out
}

// classify derives (browser, os) from the path a Local State was found at.
func classify(path string) (string, string) {
	browser := "chrome"
	if strings.Contains(strings.ToLower(path), "edge") {
		browser = "edge"
	}
	switch {
	case strings.HasPrefix(path, "/mnt/"):
		return browser, "wsl"
	case strings.Contains(path, "/Library/"):
		return browser, "macos"
	default:
		return browser, "linux"
	}
}

const marker = "===AZRL "

// posixProbe cats every candidate Local State (Linux, macOS, WSL) with a
// marker line before each hit, in one ssh round-trip. `true` keeps the exit
// status 0 when nothing matches.
const posixProbe = `for f in "$HOME/.config/microsoft-edge/Local State" "$HOME/.config/google-chrome/Local State" "$HOME/Library/Application Support/Microsoft Edge/Local State" "$HOME/Library/Application Support/Google/Chrome/Local State" /mnt/c/Users/*/AppData/Local/Microsoft/Edge/"User Data"/"Local State" /mnt/c/Users/*/AppData/Local/Google/Chrome/"User Data"/"Local State"; do [ -f "$f" ] && { echo "===AZRL $f"; cat "$f"; echo; }; done; true`

// winProbes cover native-Windows OpenSSH, whose default shell is cmd.exe.
var winProbes = []struct{ browser, path string }{
	{"edge", `%LOCALAPPDATA%\Microsoft\Edge\User Data\Local State`},
	{"chrome", `%LOCALAPPDATA%\Google\Chrome\User Data\Local State`},
}

func sshRun(host, command string) ([]byte, error) {
	return exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", host, command).Output()
}

// Discover reads the local machine's Edge/Chrome profile lists over ssh.
// Best-effort and read-only: any failure yields an error and callers fall
// back to manual entry.
func Discover(g config.Global) ([]Profile, error) {
	out, posixErr := sshRun(g.LocalHost, posixProbe)
	if posixErr == nil {
		if ps := parseProbe(string(out)); len(ps) > 0 {
			return ps, nil
		}
	}
	var all []Profile
	winErrs := 0
	for _, w := range winProbes {
		b, werr := sshRun(g.LocalHost, `cmd /c type "`+w.path+`"`)
		if werr != nil {
			winErrs++
			continue
		}
		all = append(all, parseLocalState(w.browser, "windows", b)...)
	}
	if len(all) > 0 {
		return all, nil
	}
	if posixErr != nil && winErrs == len(winProbes) {
		return nil, fmt.Errorf("browserpick: cannot reach %s: %w", g.LocalHost, posixErr)
	}
	return nil, fmt.Errorf("browserpick: no browser profiles found on %s", g.LocalHost)
}

// parseProbe splits the POSIX probe output on marker lines.
func parseProbe(out string) []Profile {
	var all []Profile
	for _, c := range strings.Split(out, marker)[1:] {
		nl := strings.IndexByte(c, '\n')
		if nl < 0 {
			continue
		}
		browser, osName := classify(strings.TrimSpace(c[:nl]))
		all = append(all, parseLocalState(browser, osName, []byte(c[nl+1:]))...)
	}
	return all
}
