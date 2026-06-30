# azrl Go CLI/TUI Rewrite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `azrl` Bash CLI with a single self-contained Go binary that exposes Cobra subcommands and launches a Bubble Tea / Lip Gloss TUI when run bare.

**Architecture:** Pure logic in `internal/profile` and `internal/config`; `az`/`ssh` orchestration in `internal/azure` (shelling out via `os/exec`, JSON via `encoding/json`); Cobra commands in `cmd/`; the binary is its own `$BROWSER` capture shim via a hidden `__browser-capture` subcommand; the TUI lives in `internal/ui`.

**Tech Stack:** Go 1.24, `github.com/spf13/cobra`, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles`, `github.com/charmbracelet/lipgloss`, `github.com/common-nighthawk/go-figure`.

## Global Constraints

- Module path: `github.com/slamb2k/azrl`. Go version floor: `go 1.24`.
- Runtime deps: `az` and `ssh` on PATH. `jq` is NOT used (parse JSON natively).
- Config formats and layout are UNCHANGED: global `~/.azure-profiles/azrl.conf` with `LOCAL_HOST`, `LOCAL_BROWSER_CMD`, `VM_HOST`; per-profile `~/.azure-profiles/<name>.conf` with `AZ_TENANT` (required), `AZ_TENANT_ID`, `AZ_DEFAULT_SUB`, `AZ_EXPECT_USER`; isolated `AZURE_CONFIG_DIR=~/.azure-profiles/<name>/`; repo `.azprofile` is one line naming the profile.
- `az login` is always invoked with `--allow-no-subscription --only-show-errors`.
- The `$BROWSER` capture command defaults to `<self> __browser-capture` but is overridable via env `AZRL_CAPTURE` (used by tests). The capture file path is passed via env `AZRL_CAPFILE`.
- Login watchdog timeout: env `AZRL_LOGIN_TIMEOUT` seconds, default `180`.
- NO legacy flag aliases (`--init/--capture/--use/--rm/--list/--paste/--save`). Subcommands only; bare `azrl` opens the TUI.
- Conventional commits with `azrl` scope. TDD: write the failing test first, watch it fail, implement, watch it pass, commit.
- Every task ends green: `go build ./...` and `go test ./...` pass; `gofmt -l .` reports nothing.

---

### Task 1: Project scaffold and Cobra root

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `cmd/root.go`
- Create: `cmd/root_test.go`

**Interfaces:**
- Produces: `cmd.Execute() error`; `cmd.RootCmd *cobra.Command`; `cmd.Version string`.

- [ ] **Step 1: Initialize the module and add Cobra**

Run:
```bash
cd /home/slamb2k/work/azrl
go mod init github.com/slamb2k/azrl
go get github.com/spf13/cobra@latest
```
Expected: `go.mod` created with `module github.com/slamb2k/azrl` and `go 1.24`, plus a `require` for cobra.

- [ ] **Step 2: Write the failing test**

Create `cmd/root_test.go`:
```go
package cmd

import (
	"bytes"
	"testing"
)

func TestRootVersionFlag(t *testing.T) {
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"--version"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("azrl")) {
		t.Fatalf("version output missing 'azrl': %q", buf.String())
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL (package does not compile — `RootCmd` undefined).

- [ ] **Step 4: Implement the root command**

Create `cmd/root.go`:
```go
package cmd

import (
	"github.com/spf13/cobra"
)

// Version is the azrl version string.
const Version = "0.2.0"

// RootCmd is the base command. With no subcommand it launches the TUI
// (wired in a later task); for now it prints help.
var RootCmd = &cobra.Command{
	Use:     "azrl",
	Short:   "Azure Remote Login — interactive az login from a headless VM",
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// Execute runs the root command.
func Execute() error {
	return RootCmd.Execute()
}
```

Create `main.go`:
```go
package main

import (
	"os"

	"github.com/slamb2k/azrl/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Run tests and build**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: build ok, test PASS, gofmt prints nothing.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum main.go cmd/
git commit -m "feat(azrl): scaffold Go module and Cobra root command"
```

---

### Task 2: config package (paths + global conf)

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `config.ParseKV(r io.Reader) (map[string]string, error)` — parses `KEY=value` lines, trims spaces, skips blanks and `#` comments.
  - `config.ProfilesDir() string` — `~/.azure-profiles` (honors `$HOME`).
  - `config.Global struct { LocalHost, LocalBrowserCmd, VMHost string }`.
  - `config.LoadGlobal(dir string) (Global, error)` — reads `<dir>/azrl.conf`; errors if missing or any field empty.

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseKV(t *testing.T) {
	in := "# comment\nAZ_TENANT=acme.com\n\n  AZ_DEFAULT_SUB = sub-1 \n"
	m, err := ParseKV(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if m["AZ_TENANT"] != "acme.com" || m["AZ_DEFAULT_SUB"] != "sub-1" {
		t.Fatalf("got %v", m)
	}
}

func TestLoadGlobal(t *testing.T) {
	dir := t.TempDir()
	conf := "LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm\n"
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGlobal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.LocalHost != "pc" || g.LocalBrowserCmd != "wslview" || g.VMHost != "vm" {
		t.Fatalf("got %+v", g)
	}
}

func TestLoadGlobalMissing(t *testing.T) {
	if _, err := LoadGlobal(t.TempDir()); err == nil {
		t.Fatal("expected error for missing azrl.conf")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL (undefined `ParseKV`, `LoadGlobal`).

- [ ] **Step 3: Implement config**

Create `internal/config/config.go`:
```go
// Package config reads azrl's KEY=value config files and resolves paths.
package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(azrl): config package for azrl.conf and KEY=value parsing"
```

---

### Task 3: profile package — string helpers

**Files:**
- Create: `internal/profile/profile.go`
- Create: `internal/profile/profile_test.go`

**Interfaces:**
- Produces:
  - `profile.ExtractPort(url string) string` — the `localhost:<port>` callback port (handles `%3A`/`%2F` encoding); "" if none.
  - `profile.SanitizeName(raw string) string` — lowercase; non `[a-z0-9._-]` runs → `-`; trim leading/trailing `-`.
  - `profile.DefaultName(arg, dir string) string` — arg verbatim if non-empty, else SanitizeName(basename(dir)).

- [ ] **Step 1: Write the failing test**

Create `internal/profile/profile_test.go`:
```go
package profile

import "testing"

func TestExtractPort(t *testing.T) {
	cases := map[string]string{
		"https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A38149%2F&s=y": "38149",
		"https://login/x?redirect_uri=http://localhost:55322/&s=y":            "55322",
		"https://login/no-port":                                              "",
	}
	for in, want := range cases {
		if got := ExtractPort(in); got != want {
			t.Errorf("ExtractPort(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"Contoso Migration": "contoso-migration",
		"  --Foo__Bar!!  ":  "foo__bar",
	}
	for in, want := range cases {
		if got := SanitizeName(in); got != want {
			t.Errorf("SanitizeName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDefaultName(t *testing.T) {
	if got := DefaultName("My Profile", "/home/x/whatever"); got != "My Profile" {
		t.Errorf("explicit arg: got %q", got)
	}
	if got := DefaultName("", "/home/x/Contoso Migration"); got != "contoso-migration" {
		t.Errorf("fallback: got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/profile/...`
Expected: FAIL (undefined functions).

- [ ] **Step 3: Implement helpers**

Create `internal/profile/profile.go`:
```go
// Package profile holds azrl's pure profile logic: resolution, conf I/O, and
// name handling. No process execution lives here.
package profile

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	portRe = regexp.MustCompile(`localhost:(\d+)`)
	junkRe = regexp.MustCompile(`[^a-z0-9._-]+`)
	edgeRe = regexp.MustCompile(`^-+|-+$`)
)

// ExtractPort returns the callback port from an OAuth redirect URL, decoding the
// common %3A/%2F encodings first. Returns "" when no localhost:<port> is found.
func ExtractPort(url string) string {
	d := strings.ReplaceAll(url, "%3A", ":")
	d = strings.ReplaceAll(d, "%2F", "/")
	m := portRe.FindStringSubmatch(d)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// SanitizeName lowercases, collapses non [a-z0-9._-] runs to '-', and trims
// leading/trailing '-'.
func SanitizeName(raw string) string {
	s := strings.ToLower(raw)
	s = junkRe.ReplaceAllString(s, "-")
	s = edgeRe.ReplaceAllString(s, "")
	return s
}

// DefaultName returns arg verbatim when non-empty, else the sanitized basename
// of dir.
func DefaultName(arg, dir string) string {
	if arg != "" {
		return arg
	}
	return SanitizeName(filepath.Base(dir))
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/profile/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/
git commit -m "feat(azrl): profile name and callback-port helpers"
```

---

### Task 4: profile package — resolution, conf I/O, list

**Files:**
- Modify: `internal/profile/profile.go`
- Create: `internal/profile/conf.go`
- Create: `internal/profile/conf_test.go`

**Interfaces:**
- Consumes: `config.ParseKV`.
- Produces:
  - `profile.Resolve(arg, dir string) (string, error)` — arg wins; else nearest `.azprofile` walking up from dir; error if none.
  - `profile.Conf struct { Tenant, TenantID, DefaultSub, ExpectUser string }`.
  - `profile.LoadConf(name, confdir string) (Conf, error)` — reads `<confdir>/<name>.conf`; errors if missing or `AZ_TENANT` empty.
  - `profile.BuildConf(acct AccountJSON, domains DomainsJSON) Conf` and `profile.Conf.Write(path string) error` (atomic).
  - `profile.List(confdir string) ([]Listed, error)` where `Listed struct { Name, Tenant string }`, excluding `azrl`.
  - JSON shapes `profile.AccountJSON` and `profile.DomainsJSON` (defined here, reused by azure).

- [ ] **Step 1: Write the failing test**

Create `internal/profile/conf_test.go`:
```go
package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExplicitAndWalkUp(t *testing.T) {
	if got, _ := Resolve("fiig", "/tmp"); got != "fiig" {
		t.Fatalf("explicit: %q", got)
	}
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".azprofile"), []byte("digital-it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve("", deep)
	if err != nil || got != "digital-it" {
		t.Fatalf("walk-up: got %q err %v", got, err)
	}
	if _, err := Resolve("", t.TempDir()); err == nil {
		t.Fatal("expected error when no .azprofile")
	}
}

func TestLoadConf(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "fiig.conf"), []byte("AZ_TENANT=fiig.com.au\nAZ_DEFAULT_SUB=sub-123\n"), 0o644)
	c, err := LoadConf("fiig", dir)
	if err != nil || c.Tenant != "fiig.com.au" || c.DefaultSub != "sub-123" {
		t.Fatalf("got %+v err %v", c, err)
	}
	os.WriteFile(filepath.Join(dir, "bad.conf"), []byte("AZ_DEFAULT_SUB=x\n"), 0o644)
	if _, err := LoadConf("bad", dir); err == nil {
		t.Fatal("expected error for missing AZ_TENANT")
	}
}

func TestBuildAndWriteConf(t *testing.T) {
	acct := AccountJSON{TenantID: "guid-1", ID: "sub-9", User: struct {
		Name string `json:"name"`
	}{Name: "u@onenrg.onmicrosoft.com"}}
	doms := DomainsJSON{Value: []Domain{{ID: "onenrg.mail.onmicrosoft.com"}, {ID: "onenrg.onmicrosoft.com", IsDefault: true}}}
	c := BuildConf(acct, doms)
	if c.Tenant != "onenrg.onmicrosoft.com" || c.TenantID != "guid-1" || c.DefaultSub != "sub-9" {
		t.Fatalf("got %+v", c)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "nrg.conf")
	if err := c.Write(path); err != nil {
		t.Fatal(err)
	}
	rd, _ := LoadConf("nrg", dir)
	if rd.Tenant != "onenrg.onmicrosoft.com" || rd.ExpectUser != "u@onenrg.onmicrosoft.com" {
		t.Fatalf("roundtrip got %+v", rd)
	}
}

func TestBuildConfFallsBackToTenantID(t *testing.T) {
	c := BuildConf(AccountJSON{TenantID: "guid-2"}, DomainsJSON{})
	if c.Tenant != "guid-2" || c.TenantID != "guid-2" {
		t.Fatalf("got %+v", c)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "fiig.conf"), []byte("AZ_TENANT=fiig.com.au\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "nrg.conf"), []byte("AZ_TENANT=onenrg.onmicrosoft.com\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte("LOCAL_HOST=x\n"), 0o644)
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 profiles, got %d: %+v", len(got), got)
	}
	for _, p := range got {
		if p.Name == "azrl" {
			t.Fatal("azrl.conf must be excluded")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/profile/...`
Expected: FAIL (undefined `Resolve`, `LoadConf`, `BuildConf`, types).

- [ ] **Step 3: Implement conf I/O**

Create `internal/profile/conf.go`:
```go
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/slamb2k/azrl/internal/config"
)

// AccountJSON mirrors the fields azrl reads from `az account show -o json`.
type AccountJSON struct {
	TenantID            string `json:"tenantId"`
	TenantDefaultDomain string `json:"tenantDefaultDomain"`
	ID                  string `json:"id"`
	Name                string `json:"name"`
	User                struct {
		Name string `json:"name"`
	} `json:"user"`
}

// Domain is one entry from the Graph /domains response.
type Domain struct {
	ID        string `json:"id"`
	IsDefault bool   `json:"isDefault"`
}

// DomainsJSON is the Graph /v1.0/domains response.
type DomainsJSON struct {
	Value []Domain `json:"value"`
}

// Conf holds a per-profile configuration.
type Conf struct {
	Tenant     string
	TenantID   string
	DefaultSub string
	ExpectUser string
}

// Resolve returns arg when non-empty, otherwise the trimmed contents of the
// nearest .azprofile found walking up from dir.
func Resolve(arg, dir string) (string, error) {
	if arg != "" {
		return arg, nil
	}
	d := dir
	for d != "" && d != string(filepath.Separator) {
		b, err := os.ReadFile(filepath.Join(d, ".azprofile"))
		if err == nil {
			return strings.TrimSpace(string(b)), nil
		}
		d = filepath.Dir(d)
	}
	return "", fmt.Errorf("azrl: no profile arg and no .azprofile found from %s", dir)
}

// LoadConf reads <confdir>/<name>.conf and requires AZ_TENANT.
func LoadConf(name, confdir string) (Conf, error) {
	var c Conf
	path := filepath.Join(confdir, name+".conf")
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("azrl: missing config %s: %w", path, err)
	}
	defer f.Close()
	m, err := config.ParseKV(f)
	if err != nil {
		return c, err
	}
	c = Conf{Tenant: m["AZ_TENANT"], TenantID: m["AZ_TENANT_ID"], DefaultSub: m["AZ_DEFAULT_SUB"], ExpectUser: m["AZ_EXPECT_USER"]}
	if c.Tenant == "" {
		return c, fmt.Errorf("azrl: AZ_TENANT not set in %s", path)
	}
	return c, nil
}

// BuildConf derives a Conf from an account and the domains list. Tenant prefers
// the verified default domain, falling back to the tenant GUID (guest/B2B).
func BuildConf(acct AccountJSON, doms DomainsJSON) Conf {
	tenant := acct.TenantID
	for _, d := range doms.Value {
		if d.IsDefault {
			tenant = d.ID
			break
		}
	}
	return Conf{Tenant: tenant, TenantID: acct.TenantID, DefaultSub: acct.ID, ExpectUser: acct.User.Name}
}

// Write atomically writes the conf in the canonical KEY=value format.
func (c Conf) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("AZ_TENANT=%s\nAZ_TENANT_ID=%s\nAZ_DEFAULT_SUB=%s\nAZ_EXPECT_USER=%s\n",
		c.Tenant, c.TenantID, c.DefaultSub, c.ExpectUser)
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

// Listed is a profile name with its tenant.
type Listed struct {
	Name   string
	Tenant string
}

// List returns every <name>.conf in confdir (except azrl.conf) with its tenant,
// sorted by name.
func List(confdir string) ([]Listed, error) {
	entries, err := os.ReadDir(confdir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Listed
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".conf") {
			continue
		}
		name := strings.TrimSuffix(n, ".conf")
		if name == "azrl" {
			continue
		}
		tenant := "?"
		if c, err := LoadConf(name, confdir); err == nil {
			tenant = c.Tenant
		}
		out = append(out, Listed{Name: name, Tenant: tenant})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/profile/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/
git commit -m "feat(azrl): profile resolution, conf read/write, and listing"
```

---

### Task 5: profile package — Use and Remove

**Files:**
- Create: `internal/profile/manage.go`
- Create: `internal/profile/manage_test.go`

**Interfaces:**
- Produces:
  - `profile.Use(name, confdir, pwd string) error` — writes `<pwd>/.azprofile` after verifying `<confdir>/<name>.conf` exists; error (writing nothing) if missing.
  - `profile.RemoveTargets(name, confdir, pwd string) []string` — the existing paths a remove would delete (conf, dir, and `.azprofile` only if it names `name`).
  - `profile.Remove(name, confdir, pwd string) ([]string, error)` — deletes those targets; returns the removed list.

- [ ] **Step 1: Write the failing test**

Create `internal/profile/manage_test.go`:
```go
package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUse(t *testing.T) {
	confdir := t.TempDir()
	work := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	if err := Use("acme", confdir, work); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(work, ".azprofile"))
	if string(b) != "acme\n" {
		t.Fatalf("azprofile=%q", string(b))
	}
	if err := Use("ghost", confdir, t.TempDir()); err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestRemoveTargetsAndRemove(t *testing.T) {
	confdir := t.TempDir()
	work := t.TempDir()
	os.MkdirAll(filepath.Join(confdir, "acme"), 0o755)
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	os.WriteFile(filepath.Join(work, ".azprofile"), []byte("acme\n"), 0o644)
	if got := RemoveTargets("acme", confdir, work); len(got) != 3 {
		t.Fatalf("want 3 targets, got %v", got)
	}
	if _, err := Remove("acme", confdir, work); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(confdir, "acme.conf")); !os.IsNotExist(err) {
		t.Fatal("conf not removed")
	}
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); !os.IsNotExist(err) {
		t.Fatal("azprofile not removed")
	}
}

func TestRemoveLeavesNonMatchingAzprofile(t *testing.T) {
	confdir := t.TempDir()
	work := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	os.WriteFile(filepath.Join(work, ".azprofile"), []byte("other\n"), 0o644)
	got := RemoveTargets("acme", confdir, work)
	for _, p := range got {
		if filepath.Base(p) == ".azprofile" {
			t.Fatal("non-matching .azprofile must not be a target")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/profile/...`
Expected: FAIL (undefined `Use`, `RemoveTargets`, `Remove`).

- [ ] **Step 3: Implement manage.go**

Create `internal/profile/manage.go`:
```go
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Use links pwd to an existing profile by writing pwd/.azprofile, after
// verifying <confdir>/<name>.conf exists.
func Use(name, confdir, pwd string) error {
	conf := filepath.Join(confdir, name+".conf")
	if _, err := os.Stat(conf); err != nil {
		return fmt.Errorf("azrl: no such profile %q (missing %s)", name, conf)
	}
	return os.WriteFile(filepath.Join(pwd, ".azprofile"), []byte(name+"\n"), 0o644)
}

// RemoveTargets returns the existing paths that Remove would delete: the conf,
// the AZURE_CONFIG_DIR, and pwd/.azprofile only when it names this profile.
func RemoveTargets(name, confdir, pwd string) []string {
	var targets []string
	conf := filepath.Join(confdir, name+".conf")
	if _, err := os.Stat(conf); err == nil {
		targets = append(targets, conf)
	}
	dir := filepath.Join(confdir, name)
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		targets = append(targets, dir)
	}
	az := filepath.Join(pwd, ".azprofile")
	if b, err := os.ReadFile(az); err == nil && strings.TrimSpace(string(b)) == name {
		targets = append(targets, az)
	}
	return targets
}

// Remove deletes the RemoveTargets and returns the list it removed.
func Remove(name, confdir, pwd string) ([]string, error) {
	targets := RemoveTargets(name, confdir, pwd)
	for _, t := range targets {
		if err := os.RemoveAll(t); err != nil {
			return targets, err
		}
	}
	return targets, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/profile/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/
git commit -m "feat(azrl): profile use (link) and remove"
```

---

### Task 6: azure package — AssertAccount

**Files:**
- Create: `internal/azure/azure.go`
- Create: `internal/azure/assert_test.go`

**Interfaces:**
- Consumes: `profile.AccountJSON`.
- Produces:
  - `azure.AssertAccount(acctJSON []byte, expTenant, expUser string) error` — tenant matches `tenantId` OR `tenantDefaultDomain`; user matches when expUser non-empty.

- [ ] **Step 1: Write the failing test**

Create `internal/azure/assert_test.go`:
```go
package azure

import "testing"

func TestAssertAccount(t *testing.T) {
	ok := `{"tenantId":"g","tenantDefaultDomain":"fiig.com.au","user":{"name":"simon@fiig.com.au"}}`
	if err := AssertAccount([]byte(ok), "fiig.com.au", "simon@fiig.com.au"); err != nil {
		t.Fatalf("domain+user: %v", err)
	}
	if err := AssertAccount([]byte(ok), "g", ""); err != nil {
		t.Fatalf("by guid: %v", err)
	}
	guest := `{"tenantId":"96e360c3","tenantDefaultDomain":null,"user":{"name":"S@velrada.com"}}`
	if err := AssertAccount([]byte(guest), "96e360c3", "S@velrada.com"); err != nil {
		t.Fatalf("guest guid: %v", err)
	}
	if err := AssertAccount([]byte(ok), "other.com", ""); err == nil {
		t.Fatal("tenant mismatch should error")
	}
	if err := AssertAccount([]byte(ok), "fiig.com.au", "wrong@x"); err == nil {
		t.Fatal("user mismatch should error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/azure/...`
Expected: FAIL (undefined `AssertAccount`).

- [ ] **Step 3: Implement assert**

Create `internal/azure/azure.go`:
```go
// Package azure drives the az/ssh login lifecycle for azrl.
package azure

import (
	"encoding/json"
	"fmt"

	"github.com/slamb2k/azrl/internal/profile"
)

// AssertAccount verifies the signed-in account matches the expected tenant
// (by GUID or default domain) and, when expUser is non-empty, the user.
func AssertAccount(acctJSON []byte, expTenant, expUser string) error {
	var a profile.AccountJSON
	if err := json.Unmarshal(acctJSON, &a); err != nil {
		return fmt.Errorf("azrl: could not parse account json: %w", err)
	}
	if expTenant != a.TenantID && expTenant != a.TenantDefaultDomain {
		return fmt.Errorf("azrl: TENANT MISMATCH — expected %q, got tenantId=%q domain=%q",
			expTenant, a.TenantID, a.TenantDefaultDomain)
	}
	if expUser != "" && expUser != a.User.Name {
		return fmt.Errorf("azrl: USER MISMATCH — expected %q, got %q", expUser, a.User.Name)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/azure/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/azure/
git commit -m "feat(azrl): identity assertion against expected tenant/user"
```

---

### Task 7: azure package — CleanSlate, AccountShow, SetSubscription, Domains

**Files:**
- Create: `internal/azure/account.go`
- Create: `internal/azure/account_test.go`

**Interfaces:**
- Produces:
  - `azure.CleanSlate(cfgDir string) error` — `az logout`, `az account clear` (errors ignored), remove `msal_token_cache.json` + `service_principal_entries.json` in cfgDir.
  - `azure.AccountShow() ([]byte, error)` — `az account show -o json`.
  - `azure.SetSubscription(sub string) error` — `az account set --subscription <sub>`.
  - `azure.Domains() []byte` — Graph `/v1.0/domains` JSON, or `{}` on error.
  - Internal helper `runAz(args ...string) ([]byte, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/azure/account_test.go`:
```go
package azure

import (
	"os"
	"path/filepath"
	"testing"
)

// shimAz writes a fake `az` onto PATH that logs its args and echoes a fixed
// JSON for `account show`.
func shimAz(t *testing.T, logPath string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/usr/bin/env bash\necho \"$*\" >> \"" + logPath + "\"\n" +
		"case \"$*\" in *\"account show\"*) echo '{\"tenantId\":\"g\",\"name\":\"s\",\"user\":{\"name\":\"u@x\"}}';; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "az"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCleanSlate(t *testing.T) {
	log := filepath.Join(t.TempDir(), "az.log")
	shimAz(t, log)
	cfg := t.TempDir()
	os.WriteFile(filepath.Join(cfg, "msal_token_cache.json"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(cfg, "service_principal_entries.json"), []byte("x"), 0o644)
	if err := CleanSlate(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "msal_token_cache.json")); !os.IsNotExist(err) {
		t.Fatal("token cache not removed")
	}
	b, _ := os.ReadFile(log)
	if !contains(string(b), "logout") || !contains(string(b), "account clear") {
		t.Fatalf("missing logout/clear: %s", b)
	}
}

func TestAccountShow(t *testing.T) {
	shimAz(t, filepath.Join(t.TempDir(), "az.log"))
	out, err := AccountShow()
	if err != nil || !contains(string(out), "tenantId") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/azure/...`
Expected: FAIL (undefined `CleanSlate`, `AccountShow`).

- [ ] **Step 3: Implement account.go**

Create `internal/azure/account.go`:
```go
package azure

import (
	"os"
	"os/exec"
	"path/filepath"
)

func runAz(args ...string) ([]byte, error) {
	return exec.Command("az", args...).Output()
}

// CleanSlate logs out, clears accounts, and removes the scoped MSAL caches in
// cfgDir. The az errors are intentionally ignored (a fresh box has nothing to
// clear); only filesystem errors surface — and those are ignored too since the
// files may legitimately be absent.
func CleanSlate(cfgDir string) error {
	_ = exec.Command("az", "logout").Run()
	_ = exec.Command("az", "account", "clear").Run()
	os.Remove(filepath.Join(cfgDir, "msal_token_cache.json"))
	os.Remove(filepath.Join(cfgDir, "service_principal_entries.json"))
	return nil
}

// AccountShow returns `az account show -o json`.
func AccountShow() ([]byte, error) {
	return runAz("account", "show", "-o", "json")
}

// SetSubscription selects the given subscription.
func SetSubscription(sub string) error {
	return exec.Command("az", "account", "set", "--subscription", sub).Run()
}

// Domains returns the Graph /v1.0/domains JSON, or {} on error.
func Domains() []byte {
	out, err := runAz("rest", "--url", "https://graph.microsoft.com/v1.0/domains", "-o", "json")
	if err != nil {
		return []byte("{}")
	}
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/azure/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/azure/
git commit -m "feat(azrl): az account helpers (clean slate, show, set, domains)"
```

---

### Task 8: azure package — LoginCapture

**Files:**
- Create: `internal/azure/login.go`
- Create: `internal/azure/login_test.go`

**Interfaces:**
- Consumes: `profile.ExtractPort`.
- Produces:
  - `azure.Login struct { Cmd *exec.Cmd; URL, Port, Capfile string }`.
  - `azure.LoginCapture(tenant string) (*Login, error)` — starts `az login [--tenant t] --allow-no-subscription --only-show-errors` in the background with `BROWSER` set to the capture command (env `AZRL_CAPTURE` or `<self> __browser-capture`) and `AZRL_CAPFILE` set; polls the capfile; fills URL/Port. Caller must `Cmd.Wait()`.
  - `azure.captureCommand() string` — the BROWSER value.

- [ ] **Step 1: Write the failing test**

Create `internal/azure/login_test.go`:
```go
package azure

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoginCapture(t *testing.T) {
	bin := t.TempDir()
	// Fake az: emulate Python webbrowser by running $BROWSER with a URL.
	azScript := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"" + filepath.Join(bin, "az.log") + "\"\n" +
		"url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F&s=z'\n" +
		"cmd=\"${BROWSER/\\%s/$url}\"\n" +
		"eval \"$cmd\"\n" +
		"sleep 2\n"
	os.WriteFile(filepath.Join(bin, "az"), []byte(azScript), 0o755)
	// Capture shim: write the URL to $AZRL_CAPFILE.
	capShim := "#!/usr/bin/env bash\nprintf '%s' \"$1\" > \"$AZRL_CAPFILE\"\n"
	capPath := filepath.Join(bin, "cap")
	os.WriteFile(capPath, []byte(capShim), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZRL_CAPTURE", capPath)

	lg, err := LoginCapture("fiig.com.au")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lg.Cmd.Process.Kill() }()
	if lg.Port != "40404" {
		t.Fatalf("port=%q url=%q", lg.Port, lg.URL)
	}
	b, _ := os.ReadFile(filepath.Join(bin, "az.log"))
	if !contains(string(b), "--tenant") || !contains(string(b), "--allow-no-subscription") {
		t.Fatalf("az args missing flags: %s", b)
	}
}

func TestLoginCaptureNoTenantOmitsFlag(t *testing.T) {
	bin := t.TempDir()
	azScript := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"" + filepath.Join(bin, "az.log") + "\"\n" +
		"url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F'\n" +
		"cmd=\"${BROWSER/\\%s/$url}\"\neval \"$cmd\"\nsleep 2\n"
	os.WriteFile(filepath.Join(bin, "az"), []byte(azScript), 0o755)
	capPath := filepath.Join(bin, "cap")
	os.WriteFile(capPath, []byte("#!/usr/bin/env bash\nprintf '%s' \"$1\" > \"$AZRL_CAPFILE\"\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZRL_CAPTURE", capPath)

	lg, err := LoginCapture("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lg.Cmd.Process.Kill() }()
	b, _ := os.ReadFile(filepath.Join(bin, "az.log"))
	if contains(string(b), "--tenant") {
		t.Fatalf("--tenant should be omitted: %s", b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/azure/...`
Expected: FAIL (undefined `LoginCapture`).

- [ ] **Step 3: Implement login.go**

Create `internal/azure/login.go`:
```go
package azure

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/slamb2k/azrl/internal/profile"
)

// Login holds the running az login process and the captured callback details.
type Login struct {
	Cmd     *exec.Cmd
	URL     string
	Port    string
	Capfile string
}

// captureCommand returns the BROWSER value: env AZRL_CAPTURE override, else the
// running binary invoked as its hidden __browser-capture subcommand.
func captureCommand() string {
	if c := os.Getenv("AZRL_CAPTURE"); c != "" {
		return c
	}
	self, err := os.Executable()
	if err != nil {
		self = "azrl"
	}
	return self + " __browser-capture"
}

// LoginCapture starts az login in the background with the BROWSER shim and polls
// for the captured callback URL. The caller owns lg.Cmd and must Wait or Kill it.
func LoginCapture(tenant string) (*Login, error) {
	cap, err := os.CreateTemp("", "azrl-cap-*")
	if err != nil {
		return nil, err
	}
	cap.Close()
	capfile := cap.Name()

	args := []string{"login"}
	if tenant != "" {
		args = append(args, "--tenant", tenant)
	}
	args = append(args, "--allow-no-subscription", "--only-show-errors")

	cmd := exec.Command("az", args...)
	cmd.Env = append(os.Environ(),
		"AZRL_CAPFILE="+capfile,
		"BROWSER="+captureCommand()+" %s",
	)
	if err := cmd.Start(); err != nil {
		os.Remove(capfile)
		return nil, err
	}

	lg := &Login{Cmd: cmd, Capfile: capfile}
	pollMax := 200 // 200 × 0.1s = 20s
	for i := 0; i < pollMax; i++ {
		if b, err := os.ReadFile(capfile); err == nil && len(b) > 0 {
			lg.URL = string(b)
			break
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lg.URL == "" {
		return lg, fmt.Errorf("azrl: timed out waiting for auth URL")
	}
	lg.Port = profile.ExtractPort(lg.URL)
	if lg.Port == "" {
		return lg, fmt.Errorf("azrl: could not parse callback port from %q", lg.URL)
	}
	return lg, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/azure/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/azure/
git commit -m "feat(azrl): az login capture via self-shim BROWSER"
```

---

### Task 9: azure package — Bridge, WaitForLogin, PasteLine

**Files:**
- Create: `internal/azure/bridge.go`
- Create: `internal/azure/bridge_test.go`

**Interfaces:**
- Consumes: `config.Global`.
- Produces:
  - `azure.PasteLine(port, vmHost, browserCmd, url string) string` — `ssh -fNL <p>:localhost:<p> <vm> && <browser> "<url>"`.
  - `azure.Bridge(port, url string, g config.Global, forcePaste bool) (tunnel *exec.Cmd, pasteFallback string, err error)` — path B starts a reverse tunnel and opens the remote browser (returns the tunnel cmd to be killed later, empty pasteFallback); path A returns the paste line in pasteFallback and a nil tunnel.
  - `azure.WaitForLogin(cmd *exec.Cmd, timeout time.Duration) error` — waits with a deadline, kills on timeout, returns the process error (or a timeout error).

- [ ] **Step 1: Write the failing test**

Create `internal/azure/bridge_test.go`:
```go
package azure

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/slamb2k/azrl/internal/config"
)

func TestPasteLine(t *testing.T) {
	got := PasteLine("38149", "vm-always", "wslview", "https://login/x?y=z")
	want := `ssh -fNL 38149:localhost:38149 vm-always && wslview "https://login/x?y=z"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBridgePathA(t *testing.T) {
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm-always"}
	tun, paste, err := Bridge("40404", "https://login/x", g, true)
	if err != nil || tun != nil {
		t.Fatalf("forced paste should not tunnel: tun=%v err=%v", tun, err)
	}
	if paste == "" || !contains(paste, "ssh -fNL 40404:localhost:40404 vm-always") {
		t.Fatalf("paste=%q", paste)
	}
}

func TestBridgePathB(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "ssh.log")
	// ssh shim: reachability + browser cmd succeed; -R reverse tunnel stays up.
	sshScript := "#!/usr/bin/env bash\necho \"$*\" >> \"" + log + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && { sleep 2; exit 0; }; done\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(sshScript), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm-always"}
	tun, paste, err := Bridge("40404", "https://login/x", g, false)
	if err != nil || tun == nil || paste != "" {
		t.Fatalf("path B: tun=%v paste=%q err=%v", tun, paste, err)
	}
	defer func() { _ = tun.Process.Kill() }()
	b, _ := os.ReadFile(log)
	if !contains(string(b), "-R 40404:localhost:40404 pc") || !contains(string(b), "wslview") {
		t.Fatalf("ssh log missing tunnel/browser: %s", b)
	}
}

func TestWaitForLoginSuccessAndTimeout(t *testing.T) {
	ok := exec.Command("true")
	ok.Start()
	if err := WaitForLogin(ok, 5*time.Second); err != nil {
		t.Fatalf("success: %v", err)
	}
	slow := exec.Command("sleep", "10")
	slow.Start()
	if err := WaitForLogin(slow, 200*time.Millisecond); err == nil {
		t.Fatal("expected timeout error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/azure/...`
Expected: FAIL (undefined `PasteLine`, `Bridge`, `WaitForLogin`).

- [ ] **Step 3: Implement bridge.go**

Create `internal/azure/bridge.go`:
```go
package azure

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/slamb2k/azrl/internal/config"
)

// PasteLine is the one-line command the user runs on their LOCAL machine.
func PasteLine(port, vmHost, browserCmd, url string) string {
	return fmt.Sprintf("ssh -fNL %s:localhost:%s %s && %s %q", port, port, vmHost, browserCmd, url)
}

// Bridge connects the local browser to the VM's callback port. Path B (default):
// if LocalHost is SSH-reachable, open a reverse tunnel and launch the browser
// there, returning the tunnel command (kill it during teardown). Path A
// (forcePaste or unreachable): return the paste line for the user.
func Bridge(port, url string, g config.Global, forcePaste bool) (*exec.Cmd, string, error) {
	reachable := false
	if !forcePaste {
		probe := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", g.LocalHost, "true")
		reachable = probe.Run() == nil
	}
	if !reachable {
		return nil, PasteLine(port, g.VMHost, g.LocalBrowserCmd, url), nil
	}
	tunnel := exec.Command("ssh", "-N", "-R", fmt.Sprintf("%s:localhost:%s", port, port), g.LocalHost)
	if err := tunnel.Start(); err != nil {
		return nil, PasteLine(port, g.VMHost, g.LocalBrowserCmd, url), nil
	}
	time.Sleep(500 * time.Millisecond)
	if tunnel.ProcessState != nil && tunnel.ProcessState.Exited() {
		return nil, PasteLine(port, g.VMHost, g.LocalBrowserCmd, url), nil
	}
	_ = exec.Command("ssh", g.LocalHost, fmt.Sprintf("%s '%s'", g.LocalBrowserCmd, url)).Run()
	return tunnel, "", nil
}

// WaitForLogin waits for cmd with a deadline; on timeout it kills the process
// and returns an error.
func WaitForLogin(cmd *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("azrl: sign-in did not complete within %s", timeout)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/azure/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add internal/azure/
git commit -m "feat(azrl): browser bridge (reverse tunnel + paste fallback) and login watchdog"
```

---

### Task 10: Self-shim `__browser-capture` subcommand

**Files:**
- Create: `cmd/browsercapture.go`
- Create: `cmd/browsercapture_test.go`

**Interfaces:**
- Consumes: `cmd.RootCmd`.
- Produces: hidden command `azrl __browser-capture <url>` that writes `<url>` to `$AZRL_CAPFILE` and exits 0.

- [ ] **Step 1: Write the failing test**

Create `cmd/browsercapture_test.go`:
```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserCaptureWritesURL(t *testing.T) {
	cap := filepath.Join(t.TempDir(), "capfile")
	t.Setenv("AZRL_CAPFILE", cap)
	RootCmd.SetArgs([]string{"__browser-capture", "https://login/x?foo=bar"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(cap)
	if err != nil || string(b) != "https://login/x?foo=bar" {
		t.Fatalf("capfile=%q err=%v", string(b), err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL (unknown command `__browser-capture`).

- [ ] **Step 3: Implement the hidden command**

Create `cmd/browsercapture.go`:
```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var browserCaptureCmd = &cobra.Command{
	Use:    "__browser-capture [url]",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		capfile := os.Getenv("AZRL_CAPFILE")
		if capfile == "" {
			return fmt.Errorf("AZRL_CAPFILE not set")
		}
		return os.WriteFile(capfile, []byte(args[0]), 0o600)
	},
}

func init() {
	RootCmd.AddCommand(browserCaptureCmd)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add cmd/
git commit -m "feat(azrl): hidden __browser-capture self-shim subcommand"
```

---

### Task 11: CLI commands — list, use, rm

**Files:**
- Create: `cmd/list.go`
- Create: `cmd/use.go`
- Create: `cmd/rm.go`
- Create: `cmd/manage_test.go`

**Interfaces:**
- Consumes: `profile.List`, `profile.Use`, `profile.Remove`, `profile.RemoveTargets`, `config.ProfilesDir`.
- Produces: `azrl list`, `azrl use <name>`, `azrl rm <name> [-y]`. The rm command guards empty/`/`/`azrl` names (exit code 2 via `cobra` returning a sentinel; tests assert via RunE error) and prompts `[y/N]` unless `-y`.

- [ ] **Step 1: Write the failing test**

Create `cmd/manage_test.go`:
```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestListCmd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "fiig.conf"), []byte("AZ_TENANT=fiig.com.au\n"), 0o644)
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetArgs([]string{"list"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("fiig")) {
		t.Fatalf("list output: %s", buf.String())
	}
}

func TestUseCmd(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	chdir(t, work)
	RootCmd.SetArgs([]string{"use", "acme"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(work, ".azprofile"))
	if string(b) != "acme\n" {
		t.Fatalf("azprofile=%q", string(b))
	}
}

func TestRmCmdReservedName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	RootCmd.SetArgs([]string{"rm", "azrl", "-y"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatal("rm azrl should error")
	}
}

func TestRmCmdYes(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles", "acme"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	chdir(t, work)
	RootCmd.SetArgs([]string{"rm", "acme", "-y"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); !os.IsNotExist(err) {
		t.Fatal("conf not removed")
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL (unknown commands `list`/`use`/`rm`).

- [ ] **Step 3: Implement the commands**

Create `cmd/list.go`:
```go
package cmd

import (
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured profiles and their tenants",
	RunE: func(cmd *cobra.Command, args []string) error {
		profs, err := profile.List(config.ProfilesDir())
		if err != nil {
			return err
		}
		for _, p := range profs {
			cmd.Printf("%-24s %s\n", p.Name, p.Tenant)
		}
		return nil
	},
}

func init() { RootCmd.AddCommand(listCmd) }
```

Create `cmd/use.go`:
```go
package cmd

import (
	"os"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var useCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Link the current directory to an existing profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := validProfileName(name); err != nil {
			return err
		}
		pwd, _ := os.Getwd()
		if err := profile.Use(name, config.ProfilesDir(), pwd); err != nil {
			return err
		}
		cmd.Printf("azrl: linked %s/.azprofile -> profile %q\n", pwd, name)
		return nil
	},
}

func init() { RootCmd.AddCommand(useCmd) }
```

Create `cmd/rm.go`:
```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var rmYes bool

var rmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove a profile: its conf, token dir, and matching .azprofile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := validProfileName(name); err != nil {
			return err
		}
		pwd, _ := os.Getwd()
		confdir := config.ProfilesDir()
		targets := profile.RemoveTargets(name, confdir, pwd)
		if len(targets) == 0 {
			cmd.Printf("azrl: nothing to remove for %q\n", name)
			return nil
		}
		cmd.Println("azrl: will remove:")
		for _, t := range targets {
			cmd.Printf("  %s\n", t)
		}
		if !rmYes {
			cmd.Print("Remove these? [y/N] ")
			sc := bufio.NewScanner(os.Stdin)
			sc.Scan()
			if ans := strings.TrimSpace(sc.Text()); !strings.HasPrefix(strings.ToLower(ans), "y") {
				return fmt.Errorf("azrl: aborted")
			}
		}
		if _, err := profile.Remove(name, confdir, pwd); err != nil {
			return err
		}
		cmd.Printf("azrl: removed profile %q\n", name)
		return nil
	},
}

// validProfileName rejects empty names, names containing '/', and the reserved
// global config name.
func validProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("azrl: a profile name is required")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("azrl: invalid profile name %q", name)
	}
	if name == "azrl" {
		return fmt.Errorf("azrl: refusing to use the global azrl config")
	}
	return nil
}

func init() {
	rmCmd.Flags().BoolVarP(&rmYes, "yes", "y", false, "Skip the confirmation prompt")
	RootCmd.AddCommand(rmCmd)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add cmd/
git commit -m "feat(azrl): list, use, and rm subcommands"
```

---

### Task 12: CLI commands — capture, init, login

**Files:**
- Create: `cmd/flow.go` (shared login lifecycle runner)
- Create: `cmd/capture.go`
- Create: `cmd/init.go`
- Create: `cmd/login.go`
- Create: `cmd/flow_test.go`

**Interfaces:**
- Consumes: all of `internal/azure`, `internal/profile`, `internal/config`.
- Produces:
  - `cmd.runLogin(tenant string, g config.Global, forcePaste bool, out io.Writer) error` — the shared clean-slate→capture→bridge→wait sequence (clean slate only when an isolated cfgDir is in use; caller sets `AZURE_CONFIG_DIR`).
  - `cmd.captureSession(name, pwd string, out io.Writer) error` — `az account show` + domains → BuildConf → Write (refuse clobber) → write `.azprofile`.
  - Commands `azrl capture [name]`, `azrl init [name]`, `azrl login [profile] [--paste]`.

- [ ] **Step 1: Write the failing test**

Create `cmd/flow_test.go`:
```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// shimAzCapture provides an `az` whose `account show`/domains return fixed JSON.
func shimAzCapture(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/usr/bin/env bash\ncase \"$*\" in\n" +
		"  *\"account show\"*) echo '{\"tenantId\":\"guid-1\",\"id\":\"sub-1\",\"name\":\"Sub\",\"user\":{\"name\":\"u@acme.onmicrosoft.com\"}}';;\n" +
		"  *\"rest\"*\"domains\"*) echo '{\"value\":[{\"id\":\"acme.onmicrosoft.com\",\"isDefault\":true}]}';;\n" +
		"  *) echo '{}';;\nesac\n"
	os.WriteFile(filepath.Join(bin, "az"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCaptureCmd(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	shimAzCapture(t)
	chdir(t, work)
	RootCmd.SetArgs([]string{"capture", "acme"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".azure-profiles", "acme.conf"))
	if !contains(string(b), "AZ_TENANT=acme.onmicrosoft.com") || !contains(string(b), "AZ_TENANT_ID=guid-1") {
		t.Fatalf("conf=%s", b)
	}
	az, _ := os.ReadFile(filepath.Join(work, ".azprofile"))
	if string(az) != "acme\n" {
		t.Fatalf("azprofile=%q", string(az))
	}
}

func TestCaptureRefusesClobber(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=keep.me\n"), 0o644)
	shimAzCapture(t)
	chdir(t, t.TempDir())
	RootCmd.SetArgs([]string{"capture", "acme"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatal("expected clobber refusal")
	}
	b, _ := os.ReadFile(filepath.Join(home, ".azure-profiles", "acme.conf"))
	if !contains(string(b), "keep.me") {
		t.Fatalf("conf overwritten: %s", b)
	}
}

// contains/indexOf are defined in cmd test files via the azure package style;
// redefine here for the cmd package tests.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return len(sub) == 0
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL (unknown command `capture`).

- [ ] **Step 3: Implement flow.go and the commands**

Create `cmd/flow.go`:
```go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
)

// loginTimeout returns AZRL_LOGIN_TIMEOUT seconds (default 180).
func loginTimeout() time.Duration {
	if v := os.Getenv("AZRL_LOGIN_TIMEOUT"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 180 * time.Second
}

// runLogin performs the capture→bridge→wait sequence. The caller is responsible
// for CleanSlate and for setting AZURE_CONFIG_DIR when isolation is wanted.
func runLogin(tenant string, g config.Global, forcePaste bool, out io.Writer) error {
	lg, err := azure.LoginCapture(tenant)
	if err != nil {
		if lg != nil && lg.Cmd != nil && lg.Cmd.Process != nil {
			_ = lg.Cmd.Process.Kill()
		}
		return err
	}
	defer os.Remove(lg.Capfile)
	fmt.Fprintf(out, "azrl: callback port %s\n", lg.Port)
	tunnel, paste, _ := azure.Bridge(lg.Port, lg.URL, g, forcePaste)
	if tunnel != nil {
		defer func() { _ = tunnel.Process.Kill() }()
		fmt.Fprintf(out, "azrl: browser opened on %s (zero-paste path B)\n", g.LocalHost)
	} else {
		fmt.Fprintf(out, "azrl: paste this on your LOCAL machine:\n\n%s\n\n", paste)
	}
	fmt.Fprintln(out, "azrl: waiting for sign-in to complete...")
	if err := azure.WaitForLogin(lg.Cmd, loginTimeout()); err != nil {
		fmt.Fprintf(out, "✗ %v\n  Recover with:\n  %s\n", err,
			azure.PasteLine(lg.Port, g.VMHost, g.LocalBrowserCmd, lg.URL))
		return err
	}
	return nil
}

// captureSession records the current az session as <name>.conf + .azprofile.
func captureSession(name, pwd string, out io.Writer) error {
	confPath := filepath.Join(config.ProfilesDir(), name+".conf")
	if _, err := os.Stat(confPath); err == nil {
		return fmt.Errorf("azrl: %s already exists — remove it first", confPath)
	}
	acctBytes, err := azure.AccountShow()
	if err != nil {
		return fmt.Errorf("azrl: not logged in for %q — run azrl init first", name)
	}
	var acct profile.AccountJSON
	if err := json.Unmarshal(acctBytes, &acct); err != nil {
		return err
	}
	var doms profile.DomainsJSON
	_ = json.Unmarshal(azure.Domains(), &doms)
	c := profile.BuildConf(acct, doms)
	if err := c.Write(confPath); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pwd, ".azprofile"), []byte(name+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "azrl: wrote %s and %s/.azprofile\n", confPath, pwd)
	return nil
}
```

Create `cmd/capture.go`:
```go
package cmd

import (
	"os"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var captureCmd = &cobra.Command{
	Use:   "capture [name]",
	Short: "Record the current az session as a profile (no login)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pwd, _ := os.Getwd()
		arg := ""
		if len(args) == 1 {
			arg = args[0]
		}
		name := profile.DefaultName(arg, pwd)
		os.Setenv("AZURE_CONFIG_DIR", config.ProfilesDir()+"/"+name)
		return captureSession(name, pwd, cmd.OutOrStdout())
	},
}

func init() { RootCmd.AddCommand(captureCmd) }
```

Create `cmd/init.go`:
```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Tenant-less login, then record the session as a profile",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		g, err := config.LoadGlobal(config.ProfilesDir())
		if err != nil {
			return err
		}
		pwd, _ := os.Getwd()
		arg := ""
		if len(args) == 1 {
			arg = args[0]
		}
		name := profile.DefaultName(arg, pwd)
		confPath := filepath.Join(config.ProfilesDir(), name+".conf")
		if _, err := os.Stat(confPath); err == nil {
			return fmt.Errorf("azrl: %s already exists — remove it first", confPath)
		}
		cfgDir := filepath.Join(config.ProfilesDir(), name)
		os.MkdirAll(cfgDir, 0o755)
		os.Setenv("AZURE_CONFIG_DIR", cfgDir)
		cmd.Printf("azrl: init profile=%s (tenant-less sign-in)\n", name)
		azure.CleanSlate(cfgDir)
		if err := runLogin("", g, false, cmd.OutOrStdout()); err != nil {
			return err
		}
		return captureSession(name, pwd, cmd.OutOrStdout())
	},
}

func init() { RootCmd.AddCommand(initCmd) }
```

Create `cmd/login.go`:
```go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/spf13/cobra"
)

var loginPaste bool

var loginCmd = &cobra.Command{
	Use:   "login [profile]",
	Short: "Sign in via the remote-browser bridge",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		g, err := config.LoadGlobal(config.ProfilesDir())
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		pwd, _ := os.Getwd()
		arg := ""
		if len(args) == 1 {
			arg = args[0]
		}
		name, rErr := profile.Resolve(arg, pwd)
		if rErr != nil {
			// No profile: tenant-less sign-in into default ~/.azure.
			fmt.Fprintln(out, "azrl: no profile resolved — tenant-less sign-in into default ~/.azure")
			fmt.Fprintln(out, "      tip: run 'azrl init <name>' to save this as a profile")
			if err := runLogin("", g, loginPaste, out); err != nil {
				return err
			}
			acct, _ := azure.AccountShow()
			printSignedIn(out, acct)
			return nil
		}
		conf, err := profile.LoadConf(name, config.ProfilesDir())
		if err != nil {
			return err
		}
		cfgDir := filepath.Join(config.ProfilesDir(), name)
		os.MkdirAll(cfgDir, 0o755)
		os.Setenv("AZURE_CONFIG_DIR", cfgDir)
		fmt.Fprintf(out, "azrl: profile=%s tenant=%s\n", name, conf.Tenant)
		azure.CleanSlate(cfgDir)
		if err := runLogin(conf.Tenant, g, loginPaste, out); err != nil {
			return err
		}
		if conf.DefaultSub != "" {
			_ = azure.SetSubscription(conf.DefaultSub)
		}
		acct, _ := azure.AccountShow()
		expTenant := conf.TenantID
		if expTenant == "" {
			expTenant = conf.Tenant
		}
		if err := azure.AssertAccount(acct, expTenant, conf.ExpectUser); err != nil {
			return err
		}
		printSignedIn(out, acct)
		return nil
	},
}

func printSignedIn(out interface{ Write([]byte) (int, error) }, acct []byte) {
	var a profile.AccountJSON
	_ = json.Unmarshal(acct, &a)
	tenant := a.TenantDefaultDomain
	if tenant == "" {
		tenant = a.TenantID
	}
	fmt.Fprintf(out, "✓ azrl: signed in as %s (tenant %s, sub %s)\n", a.User.Name, tenant, a.Name)
}

func init() {
	loginCmd.Flags().BoolVar(&loginPaste, "paste", false, "Force the manual paste-line path (A)")
	RootCmd.AddCommand(loginCmd)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 5: Commit**

```bash
git add cmd/
git commit -m "feat(azrl): capture, init, and login subcommands with shared login flow"
```

---

### Task 13: UI — styles, banner, angel art

**Files:**
- Create: `internal/ui/styles.go`
- Create: `internal/ui/angel.go`
- Create: `internal/ui/banner.go`
- Create: `internal/ui/banner_test.go`

**Interfaces:**
- Produces:
  - `ui.Palette` styles (Lip Gloss) — `Title`, `Accent`, `Success`, `Failure`, `Muted`, `Panel`.
  - `ui.AngelArt string` — the embedded multi-line angel.
  - `ui.Banner() string` — the rendered AZRL block banner + tagline + angel.

- [ ] **Step 1: Add the figlet dependency**

Run:
```bash
go get github.com/common-nighthawk/go-figure@latest
go get github.com/charmbracelet/lipgloss@latest
```

- [ ] **Step 2: Write the failing test**

Create `internal/ui/banner_test.go`:
```go
package ui

import (
	"strings"
	"testing"
)

func TestBannerContents(t *testing.T) {
	b := Banner()
	if !strings.Contains(b, "Azure Remote Login") {
		t.Fatalf("banner missing tagline:\n%s", b)
	}
	if strings.TrimSpace(AngelArt) == "" {
		t.Fatal("angel art is empty")
	}
	if strings.Count(AngelArt, "\n") < 6 {
		t.Fatalf("angel should be multi-line (>=7 rows), got:\n%s", AngelArt)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ui/...`
Expected: FAIL (undefined `Banner`, `AngelArt`).

- [ ] **Step 4: Implement styles, angel, banner**

Create `internal/ui/styles.go`:
```go
// Package ui implements azrl's Bubble Tea terminal interface.
package ui

import "github.com/charmbracelet/lipgloss"

// Azure-blue + gold palette.
var (
	azureBlue = lipgloss.Color("#2599f7")
	azureDeep = lipgloss.Color("#0a4d8c")
	gold      = lipgloss.Color("#f2c14e")
	white     = lipgloss.Color("#f5f7fa")
	green     = lipgloss.Color("#3fb950")
	red       = lipgloss.Color("#f85149")
	gray      = lipgloss.Color("#8b949e")
)

var (
	titleStyle   = lipgloss.NewStyle().Foreground(azureBlue).Bold(true)
	accentStyle  = lipgloss.NewStyle().Foreground(gold).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	failureStyle = lipgloss.NewStyle().Foreground(red).Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(gray)
	panelStyle   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(azureDeep).
			Padding(0, 1)
)
```

Create `internal/ui/angel.go`:
```go
package ui

// AngelArt is the embedded angel shown on the home screen (halo + wings).
const AngelArt = `        .-""""""-.
      .'  .-""-.  '.
     /   /  o o \   \
    |   |   \/   |   |
     \   \  __  /   /
      '-./'----'\.-'
   \\  (  \    /  )  //
    \\__\  '..'  /__//
        '-.____.-'`
```

Create `internal/ui/banner.go`:
```go
package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	figure "github.com/common-nighthawk/go-figure"
)

// Banner renders the AZRL block-letter banner with a blue gradient, the angel
// art, and the tagline.
func Banner() string {
	fig := figure.NewFigure("AZRL", "standard", true).String()
	blues := []lipgloss.Color{azureBlue, azureBlue, azureDeep, azureDeep}
	var lines []string
	for i, line := range strings.Split(strings.TrimRight(fig, "\n"), "\n") {
		c := blues[i%len(blues)]
		lines = append(lines, lipgloss.NewStyle().Foreground(c).Render(line))
	}
	logo := strings.Join(lines, "\n")
	angel := lipgloss.NewStyle().Foreground(gold).Render(AngelArt)
	top := lipgloss.JoinHorizontal(lipgloss.Top, angel, "  ", logo)
	tagline := accentStyle.Render("Azure Remote Login")
	return lipgloss.JoinVertical(lipgloss.Left, top, "", tagline)
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/ui/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/ go.mod go.sum
git commit -m "feat(azrl): TUI palette, angel art, and AZRL banner"
```

---

### Task 14: UI — profile list model with context panel

**Files:**
- Create: `internal/ui/model.go`
- Create: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `profile.List`, `profile.Resolve`, `config.ProfilesDir`, `ui.Banner`.
- Produces:
  - `ui.item struct { name, tenant string }` implementing `list.Item` (and `Title`/`Description`).
  - `ui.Model struct { ... }` with `ui.NewModel() Model`, `Init`, `Update`, `View`.
  - `ui.contextLine(pwd string) string` — "This dir → <name>", "Link this dir to '<name>'?", or "Create profile for this dir".

- [ ] **Step 1: Add bubbletea + bubbles**

Run:
```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
```

- [ ] **Step 2: Write the failing test**

Create `internal/ui/model_test.go`:
```go
package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)

	// dir linked via .azprofile
	linked := t.TempDir()
	os.WriteFile(filepath.Join(linked, ".azprofile"), []byte("acme\n"), 0o644)
	if got := contextLine(linked); !strings.Contains(got, "This dir") || !strings.Contains(got, "acme") {
		t.Fatalf("linked: %q", got)
	}

	// dir whose basename matches an existing conf -> offer link
	matchDir := filepath.Join(t.TempDir(), "acme")
	os.MkdirAll(matchDir, 0o755)
	if got := contextLine(matchDir); !strings.Contains(strings.ToLower(got), "link") {
		t.Fatalf("match: %q", got)
	}

	// unknown dir -> offer create
	if got := contextLine(filepath.Join(t.TempDir(), "brand-new")); !strings.Contains(strings.ToLower(got), "create") {
		t.Fatalf("unknown: %q", got)
	}
}

func TestModelViewRenders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	m := NewModel()
	m.width, m.height = 80, 24
	v := m.View()
	if !strings.Contains(v, "Azure Remote Login") {
		t.Fatalf("view missing banner:\n%s", v)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ui/...`
Expected: FAIL (undefined `contextLine`, `NewModel`).

- [ ] **Step 4: Implement model.go**

Create `internal/ui/model.go`:
```go
package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
)

type item struct{ name, tenant string }

func (i item) Title() string       { return i.name }
func (i item) Description() string { return i.tenant }
func (i item) FilterValue() string { return i.name }

// Model is the root TUI model.
type Model struct {
	list          list.Model
	pwd           string
	width, height int
	status        string
}

// NewModel builds the home model from the profiles on disk.
func NewModel() Model {
	pwd, _ := os.Getwd()
	var items []list.Item
	profs, _ := profile.List(config.ProfilesDir())
	for _, p := range profs {
		items = append(items, item{name: p.Name, tenant: p.Tenant})
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Profiles"
	l.SetShowHelp(false)
	return Model{list: l, pwd: pwd}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-14)
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m Model) View() string {
	ctx := panelStyle.Render(contextLine(m.pwd))
	help := mutedStyle.Render("enter use · l login · i init · c capture · u use · d delete · r refresh · q quit")
	return lipgloss.JoinVertical(lipgloss.Left, Banner(), "", ctx, "", m.list.View(), "", help)
}

// contextLine describes the current directory's relationship to profiles.
func contextLine(pwd string) string {
	if name, err := profile.Resolve("", pwd); err == nil {
		return fmt.Sprintf("This dir → %s", accentStyle.Render(name))
	}
	base := profile.SanitizeName(filepath.Base(pwd))
	conf := filepath.Join(config.ProfilesDir(), base+".conf")
	if _, err := os.Stat(conf); err == nil {
		return fmt.Sprintf("No .azprofile here. Link this dir to %s? (press u)", accentStyle.Render(base))
	}
	return fmt.Sprintf("No profile for this dir. Create one named %s? (press i)", accentStyle.Render(base))
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/ui/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/ go.mod go.sum
git commit -m "feat(azrl): TUI profile list model with directory context panel"
```

---

### Task 15: UI — actions (spinner runner, confirm, name input)

**Files:**
- Modify: `internal/ui/model.go`
- Create: `internal/ui/actions.go`
- Create: `internal/ui/actions_test.go`

**Interfaces:**
- Consumes: `profile.Use`, `profile.Remove`, `config.ProfilesDir`.
- Produces:
  - `ui.opDoneMsg struct { msg string; err error }`.
  - `ui.runUse(name string) tea.Cmd`, `ui.runDelete(name string) tea.Cmd` — return commands producing `opDoneMsg`.
  - Model gains a `spinner.Model`, a `mode` (list / confirm / input / running), a `textinput.Model` for new names, and key handling for `u`/`d`/`i`/`r`.

- [ ] **Step 1: Write the failing test**

Create `internal/ui/actions_test.go`:
```go
package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunUseProducesMsg(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	cmd := runUse("acme")
	msg := cmd()
	done, ok := msg.(opDoneMsg)
	if !ok || done.err != nil {
		t.Fatalf("msg=%v ok=%v", msg, ok)
	}
	if b, _ := os.ReadFile(filepath.Join(work, ".azprofile")); string(b) != "acme\n" {
		t.Fatalf("azprofile=%q", string(b))
	}
}

func TestRunDeleteProducesMsg(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles", "acme"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	old, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(old)

	msg := runDelete("acme")()
	if done, ok := msg.(opDoneMsg); !ok || done.err != nil {
		t.Fatalf("msg=%v ok=%v", msg, ok)
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); !os.IsNotExist(err) {
		t.Fatal("conf not removed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/...`
Expected: FAIL (undefined `runUse`, `runDelete`, `opDoneMsg`).

- [ ] **Step 3: Implement actions.go**

Create `internal/ui/actions.go`:
```go
package ui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
)

// opDoneMsg reports the result of a background action.
type opDoneMsg struct {
	msg string
	err error
}

// runUse links the current directory to name.
func runUse(name string) tea.Cmd {
	return func() tea.Msg {
		pwd, _ := os.Getwd()
		if err := profile.Use(name, config.ProfilesDir(), pwd); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{msg: fmt.Sprintf("linked this dir → %s", name)}
	}
}

// runDelete removes a profile.
func runDelete(name string) tea.Cmd {
	return func() tea.Msg {
		pwd, _ := os.Getwd()
		if _, err := profile.Remove(name, config.ProfilesDir(), pwd); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{msg: fmt.Sprintf("removed profile %s", name)}
	}
}
```

- [ ] **Step 4: Wire actions into the model**

Replace the `Update` method and the `Model` struct in `internal/ui/model.go` with the versions below (additions: spinner, status handling, and `u`/`d`/`r` keys). Add the imports `"github.com/charmbracelet/bubbles/spinner"`.

In `internal/ui/model.go`, change the `Model` struct to:
```go
// Model is the root TUI model.
type Model struct {
	list          list.Model
	spin          spinner.Model
	pwd           string
	width, height int
	status        string
	busy          bool
}
```

Change `NewModel`'s return to include the spinner:
```go
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{list: l, spin: sp, pwd: pwd}
```

Replace `Update` with:
```go
// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-15)
	case opDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.status = failureStyle.Render("✗ " + msg.err.Error())
		} else {
			m.status = successStyle.Render("✓ " + msg.msg)
		}
		// refresh the list after a mutating action
		nm := NewModel()
		nm.width, nm.height, nm.status = m.width, m.height, m.status
		nm.list.SetSize(m.width-4, m.height-15)
		return nm, nil
	case spinner.TickMsg:
		var c tea.Cmd
		m.spin, c = m.spin.Update(msg)
		return m, c
	case tea.KeyMsg:
		if m.busy {
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			nm := NewModel()
			nm.width, nm.height = m.width, m.height
			nm.list.SetSize(m.width-4, m.height-15)
			return nm, nil
		case "u":
			base := profile.SanitizeName(filepathBase(m.pwd))
			m.busy = true
			m.status = ""
			return m, tea.Batch(m.spin.Tick, runUse(base))
		case "d":
			if it, ok := m.list.SelectedItem().(item); ok {
				m.busy = true
				m.status = ""
				return m, tea.Batch(m.spin.Tick, runDelete(it.name))
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
```

Add this helper at the bottom of `internal/ui/model.go`:
```go
func filepathBase(p string) string { return filepath.Base(p) }
```

Replace `View` with a version that shows spinner/status:
```go
// View implements tea.Model.
func (m Model) View() string {
	ctx := panelStyle.Render(contextLine(m.pwd))
	statusLine := m.status
	if m.busy {
		statusLine = m.spin.View() + " working..."
	}
	help := mutedStyle.Render("enter use · l login · i init · c capture · u use · d delete · r refresh · q quit")
	return lipgloss.JoinVertical(lipgloss.Left, Banner(), "", ctx, "", m.list.View(), "", statusLine, help)
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/ui/... && gofmt -l .`
Expected: PASS, no gofmt output.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/ go.mod go.sum
git commit -m "feat(azrl): TUI use/delete actions with spinner and status"
```

---

### Task 16: Wire bare `azrl` to launch the TUI

**Files:**
- Create: `internal/ui/run.go`
- Modify: `cmd/root.go`
- Create: `cmd/root_tui_test.go`

**Interfaces:**
- Consumes: `ui.NewModel`.
- Produces: `ui.Run() error` — starts the Bubble Tea program with alt-screen; `RootCmd.RunE` calls it when no subcommand is given.

- [ ] **Step 1: Write the failing test**

Create `cmd/root_tui_test.go`:
```go
package cmd

import "testing"

// We can't drive a full interactive program in a unit test, but we can assert
// the root command is configured to run the TUI (no args) rather than printing
// help, by checking the RunE is set and the command has subcommands registered.
func TestRootHasSubcommands(t *testing.T) {
	names := map[string]bool{}
	for _, c := range RootCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"login", "init", "capture", "use", "rm", "list"} {
		if !names[want] {
			t.Fatalf("missing subcommand %q", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails or passes**

Run: `go test ./cmd/...`
Expected: PASS if all prior tasks landed (subcommands registered). If it fails, a subcommand task was skipped — fix that first.

- [ ] **Step 3: Implement ui.Run and wire the root**

Create `internal/ui/run.go`:
```go
package ui

import tea "github.com/charmbracelet/bubbletea"

// Run launches the azrl TUI.
func Run() error {
	p := tea.NewProgram(NewModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
```

In `cmd/root.go`, change the `RunE` of `RootCmd` to launch the TUI:
```go
	RunE: func(cmd *cobra.Command, args []string) error {
		return ui.Run()
	},
```
and add the import `"github.com/slamb2k/azrl/internal/ui"`.

- [ ] **Step 4: Run tests and build**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: build ok, all tests PASS, no gofmt output.

- [ ] **Step 5: Manual smoke check (optional)**

Run: `go run . list` and `go run . --help`.
Expected: `list` prints profiles (or nothing on a clean box); `--help` shows the subcommands.

- [ ] **Step 6: Commit**

```bash
git add cmd/ internal/ui/
git commit -m "feat(azrl): bare azrl launches the TUI"
```

---

### Task 17: install.sh, remove Bash, docs

**Files:**
- Modify: `install.sh`
- Delete: `azrl`, `azrl-lib.sh`, `azrl-capture`, `tests/azrl.bats`
- Modify: `README.md`
- Modify: `CLAUDE.md`

**Interfaces:** none (packaging/docs).

- [ ] **Step 1: Rewrite install.sh**

Replace `install.sh` with:
```bash
#!/usr/bin/env bash
set -euo pipefail

# Build and install the azrl binary, gitignore .azprofile, and bootstrap config.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
mkdir -p "$BIN_DIR"

echo "azrl: building..."
( cd "$ROOT" && go build -o "$BIN_DIR/azrl" . )
echo "azrl: installed $BIN_DIR/azrl"

# Globally gitignore .azprofile so it is never committed.
GI="${XDG_CONFIG_HOME:-$HOME/.config}/git/ignore"
mkdir -p "$(dirname "$GI")"
grep -qxF '.azprofile' "$GI" 2>/dev/null || echo '.azprofile' >> "$GI"

# Bootstrap the global config from the example if absent.
PROFILES="$HOME/.azure-profiles"
mkdir -p "$PROFILES"
if [[ ! -f "$PROFILES/azrl.conf" && -f "$ROOT/azrl.conf.example" ]]; then
  cp "$ROOT/azrl.conf.example" "$PROFILES/azrl.conf"
  echo "azrl: wrote $PROFILES/azrl.conf (edit LOCAL_HOST/LOCAL_BROWSER_CMD/VM_HOST)"
fi

echo "azrl: done. Ensure $BIN_DIR is on your PATH."
```

- [ ] **Step 2: Remove the Bash implementation**

Run:
```bash
git rm azrl azrl-lib.sh azrl-capture tests/azrl.bats
```
Expected: the four files are staged for deletion (preserved in git history).

- [ ] **Step 3: Update README.md**

Replace the "Usage", "Install", "Layout", and "Development" sections of `README.md` so they describe the Go binary. Use this Usage block:
```bash
azrl                       # launch the TUI (manage/select/login profiles)
azrl login [profile]       # sign in via the remote-browser bridge
azrl init [name]           # tenant-less login, then record conf + .azprofile
azrl capture [name]        # record the current session as conf + .azprofile
azrl use <name>            # link this dir to an existing profile
azrl rm <name> [-y]        # remove a profile (conf + token dir + matching .azprofile)
azrl list                  # list configured profiles and their tenants
azrl --help                # usage; azrl --version prints the version
```
and this Development block:
```bash
go build ./...
go test ./...
gofmt -l .
```
and this Layout block:
```
main.go            # entrypoint
cmd/               # Cobra subcommands (+ hidden __browser-capture self-shim)
internal/config/   # azrl.conf + KEY=value parsing
internal/profile/  # pure profile logic (resolve, conf I/O, use, rm) — unit-tested
internal/azure/    # az/ssh login lifecycle — unit + shimmed-integration tested
internal/ui/       # Bubble Tea TUI (banner, angel, list, actions)
install.sh         # go build + install + config bootstrap
```

- [ ] **Step 4: Update CLAUDE.md**

Replace the "Commands", "Architecture", and "Testing approach" sections of `CLAUDE.md` to describe the Go codebase: build/test commands (`go build ./...`, `go test ./...`, `gofmt -l .`); the package layout (cmd / internal/{config,profile,azure,ui}); pure logic lives in `internal/profile` and is unit-tested; `internal/azure` shells out to `az`/`ssh` and is tested by shimming them onto `PATH` via `t.Setenv("PATH", ...)`; bare `azrl` launches the `internal/ui` TUI; the single binary is its own `$BROWSER` shim via the hidden `__browser-capture` subcommand.

- [ ] **Step 5: Verify build, tests, and a clean tree**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: build ok, all tests PASS, no gofmt output.

- [ ] **Step 6: Commit**

```bash
git add install.sh README.md CLAUDE.md
git commit -m "build(azrl): install Go binary; remove Bash implementation; update docs"
```

---

## Self-Review

**Spec coverage (design doc → tasks):**
- Replace + single binary → Tasks 1, 17. ✅
- Cobra subcommands, bare→TUI, no legacy aliases → Tasks 1, 11, 12, 16. ✅
- Native orchestration + self-shim BROWSER → Tasks 7, 8, 9, 10. ✅
- JSON native, no jq → Tasks 6, 12 (encoding/json). ✅
- Config/layout unchanged → Tasks 2, 4 (same KEY=value format, ~/.azure-profiles). ✅
- Login lifecycle (CleanSlate/LoginCapture/Bridge/WaitForLogin/AssertAccount/sub) → Tasks 6–9, 12. ✅
- Commands login/init/capture/use/rm/list → Tasks 11, 12. ✅
- TUI: banner+angel, list, context/no-profile flow, actions+spinner, bare launch → Tasks 13–16. ✅
- Visuals Azure-blue+gold, detailed angel → Task 13. ✅
- Tests: unit + PATH-shim integration + TUI model → every task; integration in Tasks 7–9, 12. ✅
- Install/migration + docs → Task 17. ✅
- `--allow-no-subscription` always → Task 8. ✅
- AZRL_LOGIN_TIMEOUT default 180 → Task 12 (`loginTimeout`). ✅

**Placeholder scan:** No TBD/TODO; every code step contains full code. ✅

**Type consistency:** `profile.AccountJSON`/`DomainsJSON`/`Domain`/`Conf`/`Listed` defined in Task 4 and reused in Tasks 6, 12; `azure.Login`/`PasteLine`/`Bridge`/`WaitForLogin`/`AssertAccount`/`CleanSlate`/`AccountShow`/`SetSubscription`/`Domains`/`LoginCapture` defined in Tasks 6–9 and consumed in Task 12; `ui.Banner`/`AngelArt`/`NewModel`/`contextLine`/`opDoneMsg`/`runUse`/`runDelete`/`Run` consistent across Tasks 13–16; `config.ProfilesDir`/`LoadGlobal`/`Global`/`ParseKV` consistent across Tasks 2, 4, 11, 12. ✅

**Notes for the implementer:**
- The `contains`/`indexOf` test helpers appear in `internal/azure` (Task 7) and `cmd` (Task 12); they are package-local test helpers, defined once per package — do not duplicate within a package.
- `go.sum` is updated by the `go get` steps; include it in the relevant commits.
- TUI tests assert `View()`/action behavior (reliable), not golden snapshots, to avoid version-fragile output.
