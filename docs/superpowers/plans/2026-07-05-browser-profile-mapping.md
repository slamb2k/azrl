# Browser-Profile Mapping (Discovery + Picker) — Implementation Plan (Part 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Discover the local machine's Edge/Chrome browser profiles over ssh and map an azrl profile to one — via a `browser` CLI verb on every provider group and a fuzzy overlay picker in the TUI.

**Architecture:** A new pure package `internal/browserpick` (probe over ssh → parse Chromium `Local State` → `Profile{Browser,OS,Dir,Name,Email}` + `Command()`/`Label()`), a generic `Scheme.SetKey`/`GetKey` conf writer, a shared `newBrowserMapCmd` cobra constructor registered on all four groups, and a `b` action on every provider tab that runs discovery in an async `tea.Cmd` and opens a centered overlay picker (options-popup style) with an inline manual-entry fallback.

**Tech Stack:** Go stdlib (`encoding/json`, `os/exec`) + existing Bubble Tea components. NO new dependencies. Spec: `docs/superpowers/specs/2026-07-05-browser-profile-mapping-design.md` (Part 2).

## Global Constraints

- **Depends on Part 1** (`feat: per-profile browser command override`) being merged: keys `AZ_BROWSER_CMD`/`GH_BROWSER_CMD`/`AWS_BROWSER_CMD`/`GCP_BROWSER_CMD` and the `AZRL_BROWSER_CMD` env hook must exist.
- New label keys: `AZ_BROWSER_LABEL`, `GH_BROWSER_LABEL`, `AWS_BROWSER_LABEL`, `GCP_BROWSER_LABEL` (display-only; a hand-set `*_BROWSER_CMD` without a label must work everywhere).
- Discovery is read-only and best-effort: ssh failure, missing files, malformed JSON, unknown OS ⇒ degrade to manual entry, never a blocking error, confs untouched on failure.
- ssh probes always use `-o BatchMode=yes -o ConnectTimeout=5` (never hang the TUI on a password prompt).
- Discovery in the TUI runs in a `tea.Cmd` closure returning a msg — NEVER `tea.ExecProcess`.
- `gofmt -l .` clean; conventional commits with scope.

## Setup (controller, before Task 1)

```bash
cd /home/slamb2k/work/azrl && git checkout main && git pull && git checkout -b feat/browser-profile-mapping
```

---

### Task 1: `internal/browserpick` — types, Local State parser, command/label rendering

**Files:**
- Create: `internal/browserpick/browserpick.go`
- Create: `internal/browserpick/browserpick_test.go`

**Interfaces:**
- Produces (used by Tasks 2, 4, 5):
  - `type Profile struct { Browser, OS, Dir, Name, Email string }`
  - `func (p Profile) Label() string` — e.g. `Edge — Work`
  - `func (p Profile) Command() string` — OS-appropriate launch command
  - `func Keys(provider string) (cmdKey, labelKey string)` — conf key names per provider
  - unexported `parseLocalState(browser, osName string, data []byte) []Profile` and `classify(path string) (browser, osName string)` (tested via exported behaviour where possible; direct calls are fine in-package)

- [ ] **Step 1: Write the failing tests** — `internal/browserpick/browserpick_test.go`:

```go
package browserpick

import "testing"

const edgeState = `{"profile":{"info_cache":{
  "Default":{"name":"Personal","user_name":"me@gmail.com"},
  "Profile 2":{"name":"Work","user_name":"simon@acme.com"},
  "Profile 3":{"name":"Unsigned"}}}}`

func TestParseLocalState(t *testing.T) {
	ps := parseLocalState("edge", "linux", []byte(edgeState))
	if len(ps) != 3 {
		t.Fatalf("want 3 profiles, got %+v", ps)
	}
	// sorted by Dir: Default, Profile 2, Profile 3
	if ps[1].Dir != "Profile 2" || ps[1].Name != "Work" || ps[1].Email != "simon@acme.com" {
		t.Fatalf("got %+v", ps[1])
	}
	if ps[2].Email != "" {
		t.Fatalf("unsigned profile should have empty email: %+v", ps[2])
	}
	if parseLocalState("edge", "linux", []byte("not json")) != nil {
		t.Fatal("malformed JSON must yield nil")
	}
}

func TestClassify(t *testing.T) {
	for _, tc := range []struct{ path, browser, os string }{
		{"/home/u/.config/microsoft-edge/Local State", "edge", "linux"},
		{"/home/u/.config/google-chrome/Local State", "chrome", "linux"},
		{"/Users/u/Library/Application Support/Microsoft Edge/Local State", "edge", "macos"},
		{"/mnt/c/Users/u/AppData/Local/Google/Chrome/User Data/Local State", "chrome", "wsl"},
	} {
		b, o := classify(tc.path)
		if b != tc.browser || o != tc.os {
			t.Fatalf("%s: got (%s,%s) want (%s,%s)", tc.path, b, o, tc.browser, tc.os)
		}
	}
}

func TestCommandPerOS(t *testing.T) {
	for _, tc := range []struct {
		p    Profile
		want string
	}{
		{Profile{Browser: "edge", OS: "linux", Dir: "Profile 2"}, `microsoft-edge --profile-directory="Profile 2"`},
		{Profile{Browser: "chrome", OS: "linux", Dir: "Default"}, `google-chrome --profile-directory="Default"`},
		{Profile{Browser: "edge", OS: "macos", Dir: "Profile 2"}, `open -na "Microsoft Edge" --args --profile-directory="Profile 2"`},
		{Profile{Browser: "edge", OS: "wsl", Dir: "Profile 2"}, `"/mnt/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" --profile-directory="Profile 2"`},
		{Profile{Browser: "chrome", OS: "windows", Dir: "Profile 2"}, `"C:\Program Files\Google\Chrome\Application\chrome.exe" --profile-directory="Profile 2"`},
	} {
		if got := tc.p.Command(); got != tc.want {
			t.Fatalf("Command() = %q, want %q", got, tc.want)
		}
	}
}

func TestLabelAndKeys(t *testing.T) {
	if l := (Profile{Browser: "edge", Name: "Work"}).Label(); l != "Edge — Work" {
		t.Fatalf("label %q", l)
	}
	if c, l := Keys("gcp"); c != "GCP_BROWSER_CMD" || l != "GCP_BROWSER_LABEL" {
		t.Fatalf("gcp keys %q %q", c, l)
	}
	if c, _ := Keys("nope"); c != "" {
		t.Fatal("unknown provider must yield empty keys")
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/browserpick/ -v` — expect FAIL (package doesn't exist).

- [ ] **Step 3: Implement** — `internal/browserpick/browserpick.go`:

```go
// Package browserpick discovers the local machine's Chromium browser profiles
// (Edge, Chrome) over ssh so an azrl profile can be mapped to one. Read-only
// and best-effort: every failure degrades to manual command entry.
package browserpick

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

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
		return fmt.Sprintf("%q %s", exe, pd)
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
```

- [ ] **Step 4: Run** `go test ./internal/browserpick/ -v` — expect PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/browserpick/
git commit -m "feat(browserpick): Chromium Local State parser and launch-command rendering"
```

---

### Task 2: `browserpick.Discover` — ssh probe

**Files:**
- Modify: `internal/browserpick/browserpick.go`
- Test: `internal/browserpick/discover_test.go` (create)

**Interfaces:**
- Consumes: Task 1's `parseLocalState`/`classify`; `config.Global` (`internal/config`).
- Produces: `func Discover(g config.Global) ([]Profile, error)` — used by Tasks 4 and 5.

- [ ] **Step 1: Write the failing tests** — `internal/browserpick/discover_test.go` (fake-ssh-on-PATH pattern from `internal/bridge/bridge_test.go`):

```go
package browserpick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slamb2k/azrl/internal/config"
)

// fakeSSH installs an ssh shim that logs args and prints the given stdout.
func fakeSSH(t *testing.T, script string) (logPath string) {
	t.Helper()
	bin := t.TempDir()
	logPath = filepath.Join(bin, "ssh.log")
	body := "#!/usr/bin/env bash\necho \"$*\" >> \"" + logPath + "\"\n" + script
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestDiscoverParsesPosixProbe(t *testing.T) {
	log := fakeSSH(t, `cat <<'EOF'
===AZRL /home/u/.config/microsoft-edge/Local State
{"profile":{"info_cache":{"Profile 2":{"name":"Work","user_name":"simon@acme.com"}}}}
===AZRL /home/u/.config/google-chrome/Local State
{"profile":{"info_cache":{"Default":{"name":"Personal","user_name":"me@gmail.com"}}}}
EOF
`)
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm"}
	ps, err := Discover(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("want 2 profiles, got %+v", ps)
	}
	if ps[0].Browser == ps[1].Browser {
		t.Fatalf("want one edge + one chrome, got %+v", ps)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "BatchMode=yes") || !strings.Contains(string(b), "pc") {
		t.Fatalf("probe must use BatchMode and target LocalHost:\n%s", b)
	}
}

func TestDiscoverUnreachable(t *testing.T) {
	fakeSSH(t, "exit 1\n")
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm"}
	if _, err := Discover(g); err == nil {
		t.Fatal("unreachable host must return an error")
	}
}

func TestDiscoverEmptyOutput(t *testing.T) {
	fakeSSH(t, "exit 0\n")
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm"}
	if _, err := Discover(g); err == nil {
		t.Fatal("no profiles found must return an error")
	}
}
```

(The empty-output case exercises the native-Windows fallback path too: the `cmd /c type` probes also return nothing, so Discover errors.)

- [ ] **Step 2: Run** `go test ./internal/browserpick/ -run TestDiscover -v` — expect FAIL (`undefined: Discover`).

- [ ] **Step 3: Implement** — append to `internal/browserpick/browserpick.go` (add `"os/exec"` and the config import):

```go
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
	out, err := sshRun(g.LocalHost, posixProbe)
	if err != nil {
		return nil, fmt.Errorf("browserpick: cannot reach %s: %w", g.LocalHost, err)
	}
	if ps := parseProbe(string(out)); len(ps) > 0 {
		return ps, nil
	}
	var all []Profile
	for _, w := range winProbes {
		b, werr := sshRun(g.LocalHost, `cmd /c type "`+w.path+`"`)
		if werr != nil {
			continue
		}
		all = append(all, parseLocalState(w.browser, "windows", b)...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("browserpick: no browser profiles found on %s", g.LocalHost)
	}
	return all, nil
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
```

- [ ] **Step 4: Run** `go test ./internal/browserpick/ -v` — expect PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/browserpick/
git commit -m "feat(browserpick): discover Edge/Chrome profiles over ssh"
```

---

### Task 3: `Scheme.SetKey`/`GetKey` + `BrowserLabel` conf round-trip

**Files:**
- Modify: `internal/profile/scheme.go:174-189` (SetLabel becomes a SetKey call; SetKey/GetKey added beside it)
- Modify: `internal/profile/conf.go`, `internal/aws/conf.go`, `internal/gcp/conf.go`, `internal/github/conf.go` (add `BrowserLabel` — same three-line pattern as Part 1's `BrowserCmd`; keys `AZ_BROWSER_LABEL` etc., appended last in each `Write` format string)
- Test: `internal/profile/scheme_test.go`, plus the four conf round-trip tests

**Interfaces:**
- Produces: `func (s Scheme) SetKey(name, confdir, key, value string) error` (order-preserving, appends missing keys), `func (s Scheme) GetKey(name, confdir, key string) string` ("" on missing/unreadable), and `Conf.BrowserLabel` on all four providers. Used by Tasks 4 and 5.

- [ ] **Step 1: Write the failing SetKey/GetKey tests** — append to `internal/profile/scheme_test.go` (use the package's existing scheme fixture; `AzureScheme()` works):

```go
func TestSetKeyPreservesOrderAndAppends(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "work.conf"),
		[]byte("AZ_TENANT=contoso.com\nAZ_LABEL=Work\n"), 0o644)
	s := AzureScheme()
	if err := s.SetKey("work", dir, "AZ_BROWSER_CMD", "chrome-work"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKey("work", dir, "AZ_TENANT", "fabrikam.com"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "work.conf"))
	want := "AZ_TENANT=fabrikam.com\nAZ_LABEL=Work\nAZ_BROWSER_CMD=chrome-work\n"
	if string(b) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b, want)
	}
}

func TestGetKey(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "work.conf"),
		[]byte("AZ_TENANT=contoso.com\nAZ_BROWSER_LABEL=Edge — Work\n"), 0o644)
	s := AzureScheme()
	if v := s.GetKey("work", dir, "AZ_BROWSER_LABEL"); v != "Edge — Work" {
		t.Fatalf("got %q", v)
	}
	if v := s.GetKey("work", dir, "MISSING"); v != "" {
		t.Fatalf("missing key must be empty, got %q", v)
	}
	if v := s.GetKey("absent", dir, "AZ_TENANT"); v != "" {
		t.Fatalf("absent conf must be empty, got %q", v)
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/profile/ -run 'TestSetKey|TestGetKey' -v` — expect FAIL (`undefined`).

- [ ] **Step 3: Implement** — in `internal/profile/scheme.go`, replace `SetLabel`'s body and add the two methods:

```go
// SetLabel updates only the label key of profile name, preserving its other
// fields. An empty label reverts the display name to the slug.
func (s Scheme) SetLabel(name, confdir, label string) error {
	return s.SetKey(name, confdir, s.LabelKey, label)
}

// SetKey updates a single key of profile name's conf, preserving the other
// keys and their order (the key is appended when absent).
func (s Scheme) SetKey(name, confdir, key, value string) error {
	path := filepath.Join(confdir, name+".conf")
	m, order, err := readOrderedKV(path)
	if err != nil {
		return err
	}
	if _, ok := m[key]; !ok {
		order = append(order, key)
	}
	m[key] = value
	var b strings.Builder
	for _, k := range order {
		fmt.Fprintf(&b, "%s=%s\n", k, m[k])
	}
	return writeAtomic(path, b.String())
}

// GetKey returns one key's value from the conf; "" when the key or the conf
// is missing (best-effort, display-only callers).
func (s Scheme) GetKey(name, confdir, key string) string {
	m, _, err := readOrderedKV(filepath.Join(confdir, name+".conf"))
	if err != nil {
		return ""
	}
	return m[key]
}
```

- [ ] **Step 4: Run** `go test ./internal/profile/ -v` — expect PASS (including the existing SetLabel tests, now routed through SetKey).

- [ ] **Step 5: Add `BrowserLabel` to the four Confs (TDD per package)** — for each of `internal/profile/conf.go` (`AZ_BROWSER_LABEL`), `internal/aws/conf.go` (`AWS_BROWSER_LABEL`), `internal/gcp/conf.go` (`GCP_BROWSER_LABEL`), `internal/github/conf.go` (`GH_BROWSER_LABEL`): first extend that package's round-trip test exactly as Part 1 did for `BrowserCmd` (aws/gcp: add `BrowserLabel: "Edge — Work",` to the fixture; profile/github: set the field and assert it back), run to see the compile FAIL, then add the struct field `BrowserLabel string // human label for BrowserCmd, e.g. "Edge — Work" (display-only)`, the `LoadConf` map read, and the `\n<KEY>=%s` appended last in `Write`. Run `go test ./internal/profile/ ./internal/aws/ ./internal/gcp/ ./internal/github/ -v` — expect PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/profile/ internal/aws/conf.go internal/aws/conf_test.go internal/gcp/conf.go internal/gcp/conf_test.go internal/github/conf.go internal/github/conf_test.go
git commit -m "feat(profile): generic Scheme.SetKey/GetKey and BrowserLabel conf keys"
```

---

### Task 4: `browser` CLI verb on all four groups

**Files:**
- Create: `cmd/browsermap.go`
- Modify: `cmd/aws.go:281-284` (`awsSubcommands`), `cmd/gh.go:221-226` (`githubSubcommands`), the gcp equivalent subcommand builder in `cmd/gcp.go`
- Test: `cmd/browsermap_test.go` (create)

**Interfaces:**
- Consumes: `browserpick.Discover`/`Keys`/`Profile.Command`/`Label`; `Scheme.SetKey`; `provider.Provider`.
- Produces: `azrl browser <name>`, `azrl gh browser <name>`, `azrl aws browser <name>`, `azrl gcp browser <name>` (and `ghrl browser <name>` via the promoted subcommand list).

- [ ] **Step 1: Write the failing tests** — `cmd/browsermap_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedBrowserMapEnv wires azrl.conf, one gcp profile conf, and an ssh shim
// that prints a POSIX-probe hit for an Edge Local State.
func seedBrowserMapEnv(t *testing.T, sshScript string) (confPath string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "azrl.conf"),
		[]byte("LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm\n"), 0o644)
	gp := filepath.Join(home, ".gcp-profiles")
	os.MkdirAll(gp, 0o755)
	confPath = filepath.Join(gp, "work.conf")
	os.WriteFile(confPath,
		[]byte("GCP_PROJECT=acme-prod\nGCP_EXPECT_ACCOUNT=simon@acme.com\n"), 0o644)

	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/usr/bin/env bash\n"+sshScript), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return confPath
}

const twoProfileProbe = `cat <<'EOF'
===AZRL /home/u/.config/microsoft-edge/Local State
{"profile":{"info_cache":{"Default":{"name":"Personal","user_name":"me@gmail.com"},"Profile 2":{"name":"Work","user_name":"simon@acme.com"}}}}
EOF
`

func TestBrowserMapPicksDiscoveredProfile(t *testing.T) {
	confPath := seedBrowserMapEnv(t, twoProfileProbe)
	RootCmd.SetIn(strings.NewReader("1\n"))
	out, err := execRoot(t, "gcp", "browser", "work")
	if err != nil {
		t.Fatalf("browser map: %v (out=%q)", err, out)
	}
	// GCP_EXPECT_ACCOUNT matches "Profile 2" (Work), so identity sorting puts
	// it first — option 1.
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), `GCP_BROWSER_CMD=microsoft-edge --profile-directory="Profile 2"`) {
		t.Fatalf("conf missing mapped command:\n%s", b)
	}
	if !strings.Contains(string(b), "GCP_BROWSER_LABEL=Edge — Work") {
		t.Fatalf("conf missing label:\n%s", b)
	}
	if !strings.Contains(string(b), "GCP_PROJECT=acme-prod") {
		t.Fatalf("existing keys must be preserved:\n%s", b)
	}
}

func TestBrowserMapManualFallbackWhenUnreachable(t *testing.T) {
	confPath := seedBrowserMapEnv(t, "exit 1\n")
	RootCmd.SetIn(strings.NewReader("m\nmy-browser --foo\n"))
	if _, err := execRoot(t, "gcp", "browser", "work"); err != nil {
		t.Fatalf("manual fallback: %v", err)
	}
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), "GCP_BROWSER_CMD=my-browser --foo") {
		t.Fatalf("manual command not written:\n%s", b)
	}
}

func TestBrowserMapClear(t *testing.T) {
	confPath := seedBrowserMapEnv(t, "exit 1\n")
	os.WriteFile(confPath,
		[]byte("GCP_PROJECT=acme-prod\nGCP_BROWSER_CMD=old\nGCP_BROWSER_LABEL=Old\n"), 0o644)
	RootCmd.SetIn(strings.NewReader("0\n"))
	if _, err := execRoot(t, "gcp", "browser", "work"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), "GCP_BROWSER_CMD=\n") || !strings.Contains(string(b), "GCP_BROWSER_LABEL=\n") {
		t.Fatalf("mapping not cleared:\n%s", b)
	}
}

func TestBrowserMapUnknownProfileErrors(t *testing.T) {
	seedBrowserMapEnv(t, "exit 1\n")
	if _, err := execRoot(t, "gcp", "browser", "nope"); err == nil {
		t.Fatal("unknown profile must error")
	}
}
```

(If `execRoot` lives in another cmd test file with a different reset behaviour for stdin, reset it after use: `t.Cleanup(func() { RootCmd.SetIn(nil) })` inside `seedBrowserMapEnv`.)

- [ ] **Step 2: Run** `go test ./cmd/ -run TestBrowserMap -v` — expect FAIL (`unknown command "browser"`).

- [ ] **Step 3: Implement** — create `cmd/browsermap.go`:

```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/browserpick"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/gcp"
	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// newBrowserMapCmd builds the `browser <name>` verb for one provider group:
// discover the laptop's Edge/Chrome profiles, offer a numbered pick (identity
// matches first) plus manual entry and clear, and write the mapping keys.
func newBrowserMapCmd(tool string, provFn func() provider.Provider, expectIdent func(name, dir string) string, validName func(string) error) *cobra.Command {
	return &cobra.Command{
		Use:   "browser <name>",
		Short: "Map the profile to a local browser profile (Edge/Chrome)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validName(name); err != nil {
				return err
			}
			prov := provFn()
			dir := prov.ProfilesDir()
			if _, err := os.Stat(filepath.Join(dir, name+".conf")); err != nil {
				return fmt.Errorf("%s: no profile %q", tool, name)
			}
			cmdKey, labelKey := browserpick.Keys(prov.Name())
			out := cmd.OutOrStdout()
			var found []browserpick.Profile
			if g, err := config.LoadGlobal(config.ProfilesDir()); err == nil {
				if ps, derr := browserpick.Discover(g); derr == nil {
					found = ps
				} else {
					fmt.Fprintf(out, "%s: discovery failed (%v) — manual entry only\n", tool, derr)
				}
			}
			if ident := expectIdent(name, dir); ident != "" {
				sort.SliceStable(found, func(i, j int) bool {
					return found[i].Email == ident && found[j].Email != ident
				})
			}
			for i, p := range found {
				email := p.Email
				if email == "" {
					email = "(not signed in)"
				}
				fmt.Fprintf(out, "%2d) %-28s %s\n", i+1, p.Label(), email)
			}
			fmt.Fprintln(out, " m) enter command manually")
			fmt.Fprintln(out, " 0) clear mapping")
			fmt.Fprint(out, "select: ")
			in := bufio.NewScanner(cmd.InOrStdin())
			if !in.Scan() {
				return fmt.Errorf("%s: no selection", tool)
			}
			s := prov.Scheme()
			set := func(cmdVal, labelVal string) error {
				if err := s.SetKey(name, dir, cmdKey, cmdVal); err != nil {
					return err
				}
				return s.SetKey(name, dir, labelKey, labelVal)
			}
			switch ans := strings.TrimSpace(in.Text()); {
			case ans == "0":
				if err := set("", ""); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s: cleared browser mapping for %q\n", tool, name)
			case ans == "m":
				fmt.Fprint(out, "command: ")
				if !in.Scan() || strings.TrimSpace(in.Text()) == "" {
					return fmt.Errorf("%s: no command entered", tool)
				}
				c := strings.TrimSpace(in.Text())
				if err := set(c, ""); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s: %q now opens with: %s\n", tool, name, c)
			default:
				n, err := strconv.Atoi(ans)
				if err != nil || n < 1 || n > len(found) {
					return fmt.Errorf("%s: invalid selection %q", tool, ans)
				}
				p := found[n-1]
				if err := set(p.Command(), p.Label()); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s: %q now opens with %s\n", tool, name, p.Label())
			}
			return nil
		},
	}
}

func azureExpectIdent(name, dir string) string {
	c, err := profile.LoadConf(name, dir)
	if err != nil {
		return ""
	}
	return c.ExpectUser
}

func gcpExpectIdent(name, dir string) string {
	c, err := gcp.LoadConf(name, dir)
	if err != nil {
		return ""
	}
	return c.ExpectAccount
}

func ghExpectIdent(name, dir string) string {
	c, err := github.LoadConf(name, dir)
	if err != nil {
		return ""
	}
	return c.User // a login, not an email — matches only when they coincide
}

func noExpectIdent(string, string) string { return "" }

func newAzureBrowserCmd() *cobra.Command {
	return newBrowserMapCmd("azrl", func() provider.Provider { return azure.NewProvider() }, azureExpectIdent, validAzureName)
}

func newGhBrowserCmd() *cobra.Command {
	return newBrowserMapCmd("ghrl", func() provider.Provider { return github.NewProvider() }, ghExpectIdent, validGhName)
}

func newAwsBrowserCmd() *cobra.Command {
	return newBrowserMapCmd("azrl aws", func() provider.Provider { return aws.NewProvider() }, noExpectIdent, validAwsName)
}

func newGcpBrowserCmd() *cobra.Command {
	return newBrowserMapCmd("azrl gcp", func() provider.Provider { return gcp.NewProvider() }, gcpExpectIdent, validGcpName)
}

func init() { RootCmd.AddCommand(newAzureBrowserCmd()) }
```

Notes for the implementer:
- Import `internal/azure` for `azure.NewProvider()` alongside the imports shown.
- Register the group verbs by appending `newGhBrowserCmd()`, `newAwsBrowserCmd()`, `newGcpBrowserCmd()` to `githubSubcommands()` (cmd/gh.go:221), `awsSubcommands()` (cmd/aws.go:281), and the gcp subcommand builder in cmd/gcp.go respectively — the ghrl promoted top level reuses `githubSubcommands()` so it gets `browser` for free.
- If `validGhName`/`validGcpName` have different names, use the group's existing name validator.

- [ ] **Step 4: Run** `go test ./cmd/ -run TestBrowserMap -v` — expect PASS. Then `go test ./cmd/` — full package PASS (no regressions in help output tests).

- [ ] **Step 5: Commit**

```bash
git add cmd/browsermap.go cmd/browsermap_test.go cmd/aws.go cmd/gh.go cmd/gcp.go
git commit -m "feat(cmd): browser verb — map a profile to a local browser profile"
```

---

### Task 5: TUI — `b` action, async discovery, overlay picker, manual fallback, DETAILS row

**Files:**
- Create: `internal/ui/browserpicker.go`
- Modify: `internal/ui/provider_view.go` (struct fields, `capturesInput`, update routing, `View` overlay, new action func; add the action to the azure tab's slice if it lives here)
- Modify: `internal/ui/aws_view.go:16-20`, `internal/ui/github_view.go:14-19`, the gcp view's action slice, and the azure view's action slice — each gains `{key: "b", label: "Browser profile", hint: "map to a local browser profile", run: browserAction}` before `Remove`
- Modify: `internal/ui/panes.go:82-100` (`profileInfoBlock` gains a `browser` row)
- Test: `internal/ui/browserpicker_test.go` (create), extend `internal/ui/aws_view_test.go`

**Interfaces:**
- Consumes: `browserpick.Discover`/`Keys`/`Profile`; `Scheme.SetKey`/`GetKey` (Task 3); existing `fuzzyScore`, `overlayCenter`, `paneTitle`, `keyHelp`, `selBlockActive`, `mutedStyle`/`successStyle`/`failureStyle`, `truncateLine`, `padTo`.
- Produces: UI-only; no new exported API.

- [ ] **Step 1: Write the failing tests** — `internal/ui/browserpicker_test.go`:

```go
package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/browserpick"
)

func seedAwsHome(t *testing.T, confBody string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	confPath := filepath.Join(ap, "work.conf")
	os.WriteFile(confPath, []byte(confBody), 0o644)
	return confPath
}

func TestBrowserActionListedOnProviderTabs(t *testing.T) {
	seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if out := nm.(awsView).View(); !strings.Contains(out, "Browser profile") {
		t.Fatalf("missing Browser profile action:\n%s", out)
	}
}

func TestBrowserProfilesMsgOpensPickerAndEnterWritesKeys(t *testing.T) {
	confPath := seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	av := nm.(awsView)
	av.providerTabView.browserFor = "work"
	msg := browserProfilesMsg{forProfile: "work", profiles: []browserpick.Profile{
		{Browser: "edge", OS: "linux", Dir: "Profile 2", Name: "Work", Email: "simon@acme.com"},
	}}
	nm2, _ := av.Update(msg)
	out := nm2.(awsView).View()
	if !strings.Contains(out, "BROWSER PROFILE") || !strings.Contains(out, "Edge — Work") {
		t.Fatalf("picker not rendered:\n%s", out)
	}
	nm3, _ := nm2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), `AWS_BROWSER_CMD=microsoft-edge --profile-directory="Profile 2"`) ||
		!strings.Contains(string(b), "AWS_BROWSER_LABEL=Edge — Work") {
		t.Fatalf("keys not written:\n%s", b)
	}
	if strings.Contains(nm3.(awsView).View(), "BROWSER PROFILE") {
		t.Fatal("picker should close after enter")
	}
}

func TestBrowserDiscoveryFailureFallsBackToManualInput(t *testing.T) {
	confPath := seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	av := nm.(awsView)
	av.providerTabView.browserFor = "work"
	nm2, _ := av.Update(browserProfilesMsg{forProfile: "work", err: os.ErrDeadlineExceeded})
	out := nm2.(awsView).View()
	if !strings.Contains(out, "Browser command") {
		t.Fatalf("manual fallback prompt not shown:\n%s", out)
	}
	av2 := nm2.(awsView)
	av2.providerTabView.browserInput.SetValue("my-browser --foo")
	nm3, _ := av2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = nm3
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), "AWS_BROWSER_CMD=my-browser --foo") {
		t.Fatalf("manual command not written:\n%s", b)
	}
}

func TestDetailsShowsBrowserLabel(t *testing.T) {
	seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_BROWSER_LABEL=Edge — Work\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if out := nm.(awsView).View(); !strings.Contains(out, "Edge — Work") {
		t.Fatalf("DETAILS missing browser label:\n%s", out)
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'TestBrowser|TestDetailsShowsBrowser' -v` — expect compile FAIL (`undefined: browserProfilesMsg` etc.).

- [ ] **Step 3: Implement the picker component** — create `internal/ui/browserpicker.go`:

```go
package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/browserpick"
	"github.com/slamb2k/azrl/internal/config"
)

// browserProfilesMsg carries the async discovery result for one azrl profile.
type browserProfilesMsg struct {
	forProfile string
	profiles   []browserpick.Profile
	err        error
}

// browserPicker is the fuzzy overlay listing discovered browser profiles.
type browserPicker struct {
	input   textinput.Model
	all     []browserpick.Profile
	matches []browserpick.Profile
	cursor  int
}

func newBrowserPicker(profiles []browserpick.Profile, ident string) browserPicker {
	if ident != "" {
		sort.SliceStable(profiles, func(i, j int) bool {
			return profiles[i].Email == ident && profiles[j].Email != ident
		})
	}
	ti := textinput.New()
	ti.Placeholder = "filter"
	ti.Focus()
	p := browserPicker{input: ti, all: profiles}
	p.refilter()
	return p
}

func (p *browserPicker) refilter() {
	pattern := p.input.Value()
	type scored struct {
		bp    browserpick.Profile
		score int
	}
	var hits []scored
	for _, b := range p.all {
		if sc := fuzzyScore(pattern, b.Label()+" "+b.Email); sc >= 0 {
			hits = append(hits, scored{b, sc})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	p.matches = p.matches[:0]
	for _, h := range hits {
		p.matches = append(p.matches, h.bp)
	}
	if p.cursor >= len(p.matches) {
		p.cursor = 0
	}
}

// update returns (picker, picked, closed): picked is non-nil only on enter.
func (p browserPicker) update(msg tea.KeyMsg) (browserPicker, *browserpick.Profile, bool) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return p, nil, true
	case "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return p, nil, false
	case "down":
		if p.cursor < len(p.matches)-1 {
			p.cursor++
		}
		return p, nil, false
	case "enter":
		if p.cursor < len(p.matches) {
			bp := p.matches[p.cursor]
			return p, &bp, true
		}
		return p, nil, false
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	_ = cmd
	p.refilter()
	return p, nil, false
}

func (p browserPicker) view() string {
	innerW := 48
	var b strings.Builder
	b.WriteString(paneTitle("BROWSER PROFILE", true) + "\n\n")
	b.WriteString(p.input.View() + "\n\n")
	for i, m := range p.matches {
		email := m.Email
		if email == "" {
			email = "(not signed in)"
		}
		line := truncateLine(m.Label()+"  "+mutedStyle.Render(email), innerW-4)
		if i == p.cursor {
			b.WriteString("  " + selBlockActive.Render(line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	if len(p.matches) == 0 {
		b.WriteString(mutedStyle.Render("  (no matches)") + "\n")
	}
	b.WriteString("\n" + lipgloss.PlaceHorizontal(innerW, lipgloss.Center,
		keyHelp("↑↓", "select", "↵", "map", "esc", "cancel")))
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = padTo(truncateLine(l, innerW), innerW)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(azureBlue).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))
}

// browserAction starts async discovery for the selected profile.
func browserAction(v *providerTabView) tea.Cmd {
	name := v.selected()
	if name == "" {
		v.status = mutedStyle.Render("no profile selected")
		return nil
	}
	v.browserFor = name
	v.status = mutedStyle.Render("looking for browser profiles on the local machine…")
	return func() tea.Msg {
		g, err := config.LoadGlobal(config.ProfilesDir())
		if err != nil {
			return browserProfilesMsg{forProfile: name, err: err}
		}
		ps, derr := browserpick.Discover(g)
		return browserProfilesMsg{forProfile: name, profiles: ps, err: derr}
	}
}
```

(`azureBlue`, `selBlockActive`, `paneTitle`, `keyHelp`, `truncateLine`, `padTo`, `mutedStyle` already exist in the ui package — reuse, don't redefine. If the border color constant has a different name, use the one `options.go:94-97` uses.)

- [ ] **Step 4: Wire the provider tab** — in `internal/ui/provider_view.go`:

`providerTabView` struct gains three fields (after `nameInput`):

```go
	browserFor    string // profile a browser mapping is being chosen for
	browserPick   *browserPicker
	browserManual bool
	browserInput  textinput.Model
```

`capturesInput()` becomes:

```go
func (v providerTabView) capturesInput() bool {
	return v.naming || v.browserManual || v.browserPick != nil
}
```

In `update`, add a `browserProfilesMsg` case (alongside the existing msg cases):

```go
	case browserProfilesMsg:
		if msg.forProfile != v.browserFor || v.browserFor == "" {
			return v, nil
		}
		if msg.err != nil || len(msg.profiles) == 0 {
			ti := textinput.New()
			ti.Placeholder = "e.g. microsoft-edge --profile-directory=\"Profile 2\""
			ti.Focus()
			v.browserInput = ti
			v.browserManual = true
			v.status = mutedStyle.Render("discovery unavailable — enter a command")
			return v, nil
		}
		ident := v.statuses[v.browserFor].Identity
		pk := newBrowserPicker(msg.profiles, ident)
		v.browserPick = &pk
		return v, nil
```

In the `tea.KeyMsg` branch, BEFORE the `if v.naming` block, add picker/manual routing:

```go
		if v.browserPick != nil {
			np, picked, closed := v.browserPick.update(msg)
			v.browserPick = &np
			if closed {
				v.browserPick = nil
				if picked != nil {
					v.applyBrowserMapping(picked.Command(), picked.Label())
				}
			}
			return v, nil
		}
		if v.browserManual {
			switch msg.String() {
			case "esc":
				v.browserManual = false
			case "enter":
				if c := strings.TrimSpace(v.browserInput.Value()); c != "" {
					v.browserManual = false
					v.applyBrowserMapping(c, "")
				}
			default:
				var cmd tea.Cmd
				v.browserInput, cmd = v.browserInput.Update(msg)
				_ = cmd
			}
			return v, nil
		}
```

Add the write helper (near `useAction` and friends):

```go
// applyBrowserMapping writes the browser cmd/label keys for browserFor.
func (v *providerTabView) applyBrowserMapping(cmdVal, labelVal string) {
	cmdKey, labelKey := browserpick.Keys(v.prov.Name())
	s := v.prov.Scheme()
	dir := v.prov.ProfilesDir()
	if err := s.SetKey(v.browserFor, dir, cmdKey, cmdVal); err != nil {
		v.status = failureStyle.Render(err.Error())
		return
	}
	if err := s.SetKey(v.browserFor, dir, labelKey, labelVal); err != nil {
		v.status = failureStyle.Render(err.Error())
		return
	}
	disp := labelVal
	if disp == "" {
		disp = cmdVal
	}
	v.status = successStyle.Render(fmt.Sprintf("%q opens with %s", v.browserFor, disp))
}
```

In `View()`, render the manual prompt in the actions body (same place as the naming prompt, provider_view.go:414-418):

```go
	if v.browserManual {
		actionsBody = mutedStyle.Render("Browser command (runs on the local machine):") + "\n\n" +
			v.browserInput.View() + "\n\n" +
			keyHelp("↵", "save", "esc", "cancel")
	}
```

and wrap the final returned string with the overlay:

```go
	if v.browserPick != nil {
		return overlayCenter(view, v.browserPick.view(), v.width)
	}
	return view
```

(where `view` is whatever the function currently returns — adapt the variable name.)

- [ ] **Step 5: Add the action to all four tab slices** — in `aws_view.go`, `github_view.go`, the gcp view, and the azure view's action slice, insert before `Remove`:

```go
		{key: "b", label: "Browser profile", hint: "map to a local browser profile", run: browserAction},
```

- [ ] **Step 6: DETAILS row** — in `internal/ui/panes.go`, change `profileInfoBlock` to take a `browser` string and render it after `Detail`:

```go
func profileInfoBlock(pr profile.Listed, st provider.Status, browser, driftNote string, w int) string {
```

with `row("Browser", browser)` inserted after `row("Detail", pr.Detail)`. Update every call site (grep `profileInfoBlock(`): provider tabs pass

```go
	cmdKey, labelKey := browserpick.Keys(v.prov.Name())
	browser := v.prov.Scheme().GetKey(name, v.prov.ProfilesDir(), labelKey)
	if browser == "" {
		browser = v.prov.Scheme().GetKey(name, v.prov.ProfilesDir(), cmdKey)
	}
```

any non-provider call site (dashboard, if one exists) passes `""`.

- [ ] **Step 7: Run** `go test ./internal/ui/ -v` — expect PASS (new tests plus all existing view tests; existing `profileInfoBlock` callers updated).

- [ ] **Step 8: Commit**

```bash
git add internal/ui/
git commit -m "feat(ui): browser-profile picker action on every provider tab"
```

---

### Task 6: Docs + full verification

**Files:**
- Modify: `CLAUDE.md`, `README.md`

- [ ] **Step 1: CLAUDE.md** — in the cmd/ architecture bullet, add `browser` to each provider's verb list (`login/list/use/rm/capture/status/browser`); in the internal/ui bullet, mention the `b` Browser profile action + overlay picker; add `internal/browserpick/` to the package list (one line: ssh discovery of Edge/Chrome profiles → launch-command mapping); add `*_BROWSER_LABEL` next to the `*_BROWSER_CMD` keys in the configuration model.

- [ ] **Step 2: README.md** — document `azrl [gh|aws|gcp] browser <name>` and the TUI `b` action; note the identity-match sorting, the manual fallback, and the Windows-native quoting caveat (generated full-path commands assume default install locations — edit via manual entry if yours differ).

- [ ] **Step 3: Full verification**

```bash
go build ./... && gofmt -l . && go vet ./... && go test ./...
```

Expected: build clean, gofmt empty, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: document the browser verb and TUI browser-profile mapping"
```

---

## Verification (whole plan)

```bash
go build ./... && gofmt -l . && go vet ./... && go test ./...
```

Manual (real machine): `azrl gcp browser work` from a shell → numbered list of your Edge/Chrome profiles with the acme account sorted first; pick it; `azrl gcp login work` opens that browser profile. In the TUI: select a profile → `b` → overlay picker → enter → DETAILS shows the label.

## Ship

`/ship` from `feat/browser-profile-mapping` (PR title `feat: map azrl profiles to local browser profiles (discovery + picker)`). Ships only after Part 1's PR is merged.
