# `azrl console` — Web Console Deep-Link Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `azrl console <name>` (and `azrl gh|aws|gcp console <name>`, promoted `ghrl console`) opens the provider's web console as the selected credential in the mapped browser — profile `*_BROWSER_CMD` override → global `BROWSER_CMD`, local direct / remote over SSH, any failure falling back to printing the URL (never an error state); the TUI gains `c` Open console.

**Architecture:** Plan 4 of the TUI UX redesign spec (`docs/superpowers/specs/2026-07-07-tui-ux-redesign-design.md`, section "Open console (`azrl console`)"). A new ~10-line `bridge.OpenURL(g, url)` reuses `LaunchLocal` (local mode) and the bare `ssh <host> '<cmd> <url>'` remote form — deliberately NOT `bridge.Bridge`, which is login-shaped (OAuth callback tunnel console doesn't have). `cmd/console.go` mirrors `cmd/shell.go`: a per-provider `consoleURL` conf switch, a `runConsole` orchestrator, one `newConsoleCmd` constructor registered on all four surfaces (`ghrl` inherits via `githubSubcommands()`). URLs from data already in the confs: azure `https://portal.azure.com/#@<AZ_TENANT>`, aws `AWS_SSO_START_URL`, gcp `https://console.cloud.google.com/?project=<GCP_PROJECT>&authuser=<account>` (account = disk-only `Status().Identity`), github `https://<GH_HOST>`.

**Tech Stack:** Go, cobra, net/url for query escaping, existing PATH-shim ssh / fake-BROWSER_CMD test patterns.

## Global Constraints

- Build/test/format gates before every commit: `go build ./...`, `go test ./...`, `gofmt -l .` (empty output = clean).
- Conventional commits with scope.
- **Never an error state** (spec): no mapped browser + no global command, missing/invalid global config, or launch failure ⇒ print the URL and return nil.
- **Mirror, never actor (PAT-002):** console reads confs and disk-only Status; it never mutates links, native defaults, or tokens. Launching a browser is the deliberate action.
- No `Provider` interface changes.
- URLs exactly: azure `https://portal.azure.com/#@<tenant>` (Tenant, else TenantID); github `https://<GH_HOST>`; aws the profile's `AWS_SSO_START_URL` verbatim; gcp `https://console.cloud.google.com/?project=<project>&authuser=<account>` with query-escaped values, `authuser` omitted when the identity is unknown.
- Browser resolution order: profile `BrowserCmd` → global `BROWSER_CMD` (the azure-login model: override the loaded Global's field; no env round-trip needed for a same-process launch).
- Local vs remote: `config.IsLocal()` decides; remote runs `ssh <BrowserHost> "<cmd> '<url>'"`.
- TUI: key `c`, label `Open console`, hint `web console as this credential`; the `?` help overlay gains `c` in the SAME task (P3 lesson — the overlay is part of "one keymap").
- UI language "link", never "pin".

## Recorded assumptions (surface to the user at the end)

1. **`bridge.OpenURL` is new** rather than reusing `Bridge` — `Bridge` drags in the OAuth callback tunnel; the spec says "reuses the existing browser plumbing verbatim", which the two shared primitives (LaunchLocal + the ssh remote form) satisfy.
2. **Azure tenant slug**: `Tenant` (domain) preferred, `TenantID` (GUID) fallback — the portal accepts both after `#@`; error only when both are empty.
3. **GCP `authuser`** uses the disk-only `Status().Identity` (the live `[core] account`), not `GCP_EXPECT_ACCOUNT` (a guardrail that may be unset); omitted when unknown.
4. **Success prints one line** (`azrl: opening <provider> console — <url>`) so a silent browser-less remote failure isn't mistaken for success; fallbacks print the URL with a one-line reason.
5. **AWS with an empty `AWS_SSO_START_URL`** errors ("nothing to open") — unlike launch failures this is a profile-data problem the user must fix; same for gcp missing `GCP_PROJECT` and github missing `GH_HOST` (required fields).
6. **Dashboard rows do not take `c` yet** — spec assigns dashboard verb dispatch to Plan 5 ("Dashboard verbs on rows").

---

### Task 1: `bridge.OpenURL`

**Files:**
- Modify: `internal/bridge/bridge.go` (append)
- Test: `internal/bridge/bridge_test.go` (append)

**Interfaces:**
- Produces: `bridge.OpenURL(g config.Global, url string) error` — Task 3 consumes it.
- Consumes: `LaunchLocal(browserCmd, url string) error` (bridge.go:41), `g.IsLocal()`, `g.BrowserCmd`, `g.BrowserHost` — all existing.

- [ ] **Step 1: Write the failing tests**

Append to `internal/bridge/bridge_test.go` (mirror the file's existing shim style — see `TestBridgePathB` at bridge_test.go:47 for the ssh shim):

```go
func TestOpenURLLocalRunsBrowserCmd(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "browser.log")
	script := "#!/usr/bin/env bash\necho \"$*\" >> \"" + log + "\"\n"
	if err := os.WriteFile(filepath.Join(bin, "mybrowser"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	g := config.Global{BrowserCmd: "mybrowser"} // BrowserHost empty ⇒ local
	if err := OpenURL(g, "https://portal.azure.com/#@acme.com"); err != nil {
		t.Fatal(err)
	}
	// LaunchLocal starts the command asynchronously; poll briefly for the log.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if b, _ := os.ReadFile(log); strings.Contains(string(b), "https://portal.azure.com/#@acme.com") {
			return
		}
		if time.Now().After(deadline) {
			b, _ := os.ReadFile(log)
			t.Fatalf("browser cmd never received the URL; log: %s", b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestOpenURLRemoteGoesOverSSH(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(bin, "ssh.log")
	script := "#!/usr/bin/env bash\necho \"$*\" >> \"" + log + "\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	g := config.Global{BrowserHost: "pc", BrowserCmd: "wslview"}
	if err := OpenURL(g, "https://acme.awsapps.com/start"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	got := string(b)
	if !strings.Contains(got, "pc") || !strings.Contains(got, "wslview 'https://acme.awsapps.com/start'") {
		t.Fatalf("remote launch not over ssh with the browser cmd; log: %s", got)
	}
}
```

Add `"time"` to the test imports if missing.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/bridge/ -run TestOpenURL -v`
Expected: FAIL — `undefined: OpenURL`

- [ ] **Step 3: Implement**

Append to `internal/bridge/bridge.go`:

```go
// OpenURL opens a plain URL in the configured browser — the login bridge's
// launch primitives without the OAuth callback tunnel (a console link has no
// port to forward). Local mode launches directly; remote runs the browser
// command on BrowserHost over SSH.
func OpenURL(g config.Global, url string) error {
	if g.IsLocal() {
		return LaunchLocal(g.BrowserCmd, url)
	}
	return exec.Command("ssh", g.BrowserHost, fmt.Sprintf("%s '%s'", g.BrowserCmd, url)).Run()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bridge/ -count=1`
Expected: PASS (new + all pre-existing bridge tests).

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add internal/bridge/
git commit -m "feat(bridge): OpenURL — tunnel-less browser launch for plain links"
```

---

### Task 2: `consoleURL` — the per-provider deep link

**Files:**
- Create: `cmd/console.go`
- Test: `cmd/console_test.go` (create)

**Interfaces:**
- Produces: `consoleURL(providerName, name string) (url, browserCmd string, err error)` — Task 3 consumes it. `browserCmd` is the profile's mapping ("" when none).
- Consumes: the same conf loaders `shellEnv` uses (`profile.LoadConf`, `github.LoadConf`, `aws.LoadConf`, `gcp.LoadConf` + their `BrowserCmd` fields); `gcp.NewProvider().Status(name, dir)` for the authuser identity; `net/url` for escaping.

- [ ] **Step 1: Write the failing tests**

Create `cmd/console_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedConsoleHome mirrors seedShellHome's provider confs for URL building.
func seedConsoleHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	write := func(rel, body string) {
		p := filepath.Join(home, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".azure-profiles/work.conf", "AZ_TENANT=acme.com\nAZ_BROWSER_CMD=chrome --profile-directory=Work\n")
	write(".azure-profiles/guest.conf", "AZ_TENANT=\nAZ_TENANT_ID=11111111-2222-3333-4444-555555555555\n")
	write(".github-profiles/oss.conf", "GH_HOST=github.com\n")
	write(".aws-profiles/prod.conf", "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	write(".aws-profiles/hollow.conf", "AWS_SSO_REGION=us-east-1\n")
	write(".gcp-profiles/lab.conf", "GCP_PROJECT=acme-lab\nGCP_CONFIG_NAME=labcfg\n")
	return home
}

func TestConsoleURLPerProvider(t *testing.T) {
	home := seedConsoleHome(t)

	u, browser, err := consoleURL("azure", "work")
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://portal.azure.com/#@acme.com" {
		t.Fatalf("azure url = %q", u)
	}
	if browser != "chrome --profile-directory=Work" {
		t.Fatalf("azure browser mapping = %q", browser)
	}

	u, _, err = consoleURL("azure", "guest")
	if err != nil || u != "https://portal.azure.com/#@11111111-2222-3333-4444-555555555555" {
		t.Fatalf("azure GUID fallback = %q, %v", u, err)
	}

	u, _, err = consoleURL("github", "oss")
	if err != nil || u != "https://github.com" {
		t.Fatalf("github url = %q, %v", u, err)
	}

	u, _, err = consoleURL("aws", "prod")
	if err != nil || u != "https://acme.awsapps.com/start" {
		t.Fatalf("aws url = %q, %v", u, err)
	}

	// gcp: no signed-in account on disk ⇒ authuser omitted.
	u, _, err = consoleURL("gcp", "lab")
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://console.cloud.google.com/?project=acme-lab" {
		t.Fatalf("gcp url without identity = %q", u)
	}

	// gcp with a live account in the named configuration ⇒ authuser added.
	gcDir := filepath.Join(home, ".config", "gcloud")
	os.MkdirAll(filepath.Join(gcDir, "configurations"), 0o755)
	os.WriteFile(filepath.Join(gcDir, "configurations", "config_labcfg"),
		[]byte("[core]\naccount = dev@example.com\nproject = acme-lab\n"), 0o644)
	u, _, err = consoleURL("gcp", "lab")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "project=acme-lab") || !strings.Contains(u, "authuser=dev%40example.com") {
		t.Fatalf("gcp url with identity = %q", u)
	}
}

func TestConsoleURLErrorsOnMissingData(t *testing.T) {
	seedConsoleHome(t)
	if _, _, err := consoleURL("aws", "hollow"); err == nil {
		t.Fatal("aws profile without a start URL must error")
	}
	if _, _, err := consoleURL("azure", "nope"); err == nil {
		t.Fatal("unknown profile must error")
	}
	if _, _, err := consoleURL("mars", "x"); err == nil {
		t.Fatal("unknown provider must error")
	}
}
```

**Implementer note:** the gcp identity comes from `gcp.NewProvider().Status(name, confdir).Identity` — check how gcp `Status` resolves the config dir (`CLOUDSDK_CONFIG` env else `~/.config/gcloud`) and make sure the test's seeding matches the non-isolate read path (the conf has no `GCP_ISOLATE`, so the ambient gcloud dir applies). If Status needs `t.Setenv("CLOUDSDK_CONFIG", "")` cleared, the shared test helpers already do this for other tests — mirror them.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run TestConsoleURL -v`
Expected: FAIL — `undefined: consoleURL`

- [ ] **Step 3: Implement**

Create `cmd/console.go`:

```go
package cmd

import (
	"fmt"
	"net/url"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/gcp"
	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/profile"
)

// consoleURL builds the provider's web-console deep link for a profile from
// data already in its conf (plus, for gcp, the disk-only signed-in account),
// and returns the profile's mapped browser command ("" when unmapped).
func consoleURL(providerName, name string) (string, string, error) {
	switch providerName {
	case "azure":
		c, err := profile.LoadConf(name, config.ProfilesDir())
		if err != nil {
			return "", "", fmt.Errorf("azrl: unknown azure profile %q: %w", name, err)
		}
		tenant := c.Tenant
		if tenant == "" {
			tenant = c.TenantID
		}
		if tenant == "" {
			return "", "", fmt.Errorf("azrl: profile %q has no AZ_TENANT or AZ_TENANT_ID — nothing to open", name)
		}
		return "https://portal.azure.com/#@" + tenant, c.BrowserCmd, nil
	case "github":
		c, err := github.LoadConf(name, config.GithubProfilesDir())
		if err != nil {
			return "", "", fmt.Errorf("azrl: unknown github profile %q: %w", name, err)
		}
		if c.Host == "" {
			return "", "", fmt.Errorf("azrl: profile %q has no GH_HOST — nothing to open", name)
		}
		return "https://" + c.Host, c.BrowserCmd, nil
	case "aws":
		c, err := aws.LoadConf(name, config.AwsProfilesDir())
		if err != nil {
			return "", "", fmt.Errorf("azrl: unknown aws profile %q: %w", name, err)
		}
		if c.SSOStartURL == "" {
			return "", "", fmt.Errorf("azrl: profile %q has no AWS_SSO_START_URL — nothing to open", name)
		}
		return c.SSOStartURL, c.BrowserCmd, nil
	case "gcp":
		dir := config.GcpProfilesDir()
		c, err := gcp.LoadConf(name, dir)
		if err != nil {
			return "", "", fmt.Errorf("azrl: unknown gcp profile %q: %w", name, err)
		}
		if c.Project == "" {
			return "", "", fmt.Errorf("azrl: profile %q has no GCP_PROJECT — nothing to open", name)
		}
		u := "https://console.cloud.google.com/?project=" + url.QueryEscape(c.Project)
		if st, err := gcp.NewProvider().Status(name, dir); err == nil && st.Identity != "" {
			u += "&authuser=" + url.QueryEscape(st.Identity)
		}
		return u, c.BrowserCmd, nil
	default:
		return "", "", fmt.Errorf("azrl: unknown provider %q", providerName)
	}
}
```

(If `gcp.NewProvider()` has a different constructor name, use the one `cmd/gcp.go` uses.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -run TestConsoleURL -v`
Expected: PASS (both).

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add cmd/console.go cmd/console_test.go
git commit -m "feat(console): per-provider web-console deep links"
```

---

### Task 3: `runConsole` + cobra wiring on all four surfaces

**Files:**
- Modify: `cmd/console.go` (append), `cmd/gh.go`/`cmd/aws.go`/`cmd/gcp.go` (subcommand slices)
- Test: `cmd/console_test.go` (append)

**Interfaces:**
- Produces: `runConsole(providerName, name string, out io.Writer) error`; seam `var consoleOpen = bridge.OpenURL`; verbs `azrl console`, `azrl gh|aws|gcp console` (ghrl inherits).
- Consumes: `consoleURL` (Task 2), `bridge.OpenURL` (Task 1), `config.LoadGlobal()`.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/console_test.go`:

```go
func TestRunConsoleOpensViaBridge(t *testing.T) {
	seedConsoleHome(t)
	writeGlobalConf(t, "BROWSER_CMD=wslview\n") // see implementer note below

	var opened []string
	orig := consoleOpen
	consoleOpen = func(g config.Global, u string) error {
		opened = append(opened, g.BrowserCmd+" | "+u)
		return nil
	}
	defer func() { consoleOpen = orig }()

	var out strings.Builder
	if err := runConsole("azure", "work", &out); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != "chrome --profile-directory=Work | https://portal.azure.com/#@acme.com" {
		t.Fatalf("profile browser mapping must override the global: %v", opened)
	}
	if !strings.Contains(out.String(), "opening azure console") {
		t.Fatalf("success line missing: %q", out.String())
	}
}

func TestRunConsoleFallsBackToPrintingURL(t *testing.T) {
	seedConsoleHome(t)

	// (a) no global config at all — print the URL, no error.
	var out strings.Builder
	if err := runConsole("github", "oss", &out); err != nil {
		t.Fatalf("missing global config must not error: %v", err)
	}
	if !strings.Contains(out.String(), "https://github.com") {
		t.Fatalf("fallback must print the URL: %q", out.String())
	}

	// (b) launch failure — print the URL, no error.
	writeGlobalConf(t, "BROWSER_CMD=wslview\n")
	orig := consoleOpen
	consoleOpen = func(config.Global, string) error { return fmt.Errorf("ssh unreachable") }
	defer func() { consoleOpen = orig }()
	out.Reset()
	if err := runConsole("github", "oss", &out); err != nil {
		t.Fatalf("launch failure must not error: %v", err)
	}
	if !strings.Contains(out.String(), "https://github.com") {
		t.Fatalf("fallback must print the URL: %q", out.String())
	}
}

func TestRunConsoleProfileDataErrorsSurface(t *testing.T) {
	seedConsoleHome(t)
	var out strings.Builder
	if err := runConsole("aws", "hollow", &out); err == nil {
		t.Fatal("missing start URL is a profile-data error and must surface")
	}
}

func TestConsoleVerbRegisteredOnAllSurfaces(t *testing.T) {
	find := func(cmds []*cobra.Command) bool {
		for _, c := range cmds {
			if strings.HasPrefix(c.Use, "console") {
				return true
			}
		}
		return false
	}
	if !find(RootCmd.Commands()) || !find(githubSubcommands()) || !find(awsSubcommands()) || !find(gcpSubcommands()) {
		t.Fatal("console verb missing from a surface")
	}
}
```

Add `"fmt"`, `"github.com/spf13/cobra"`, `"github.com/slamb2k/azrl/internal/config"` to the test imports as needed.

**Implementer note on `writeGlobalConf`:** the cmd test package likely already has a helper that writes `~/.azure-profiles/azrl.conf` (grep for `azrl.conf` in cmd/*_test.go); use the existing one. If none exists, add this small helper to console_test.go:

```go
func writeGlobalConf(t *testing.T, body string) {
	t.Helper()
	home := os.Getenv("HOME")
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	if err := os.WriteFile(filepath.Join(home, ".azure-profiles", "azrl.conf"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

Also check what `config.LoadGlobal` returns for a missing file and for a placeholder config — `runConsole` must treat every non-nil error as "fall back to printing", so the test seeding just needs a valid minimal conf for the happy path.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run 'TestRunConsole|TestConsoleVerb' -v`
Expected: FAIL — `undefined: runConsole`, `consoleOpen`.

- [ ] **Step 3: Implement**

Append to `cmd/console.go`:

```go
// consoleOpen is a test seam over the bridge launch.
var consoleOpen = bridge.OpenURL

// runConsole opens the provider's web console as the profile's credential.
// Failures to launch are never errors — the URL is the useful artifact, so
// every degraded path prints it and succeeds (spec: "never an error state").
// Only profile-data problems (no tenant/start URL/project) surface as errors.
func runConsole(providerName, name string, out io.Writer) error {
	u, profileBrowser, err := consoleURL(providerName, name)
	if err != nil {
		return err
	}
	g, err := config.LoadGlobal()
	if err != nil {
		fmt.Fprintf(out, "azrl: no browser configured — open it yourself:\n%s\n", u)
		return nil
	}
	if profileBrowser != "" {
		g.BrowserCmd = profileBrowser
	}
	if g.BrowserCmd == "" {
		fmt.Fprintf(out, "azrl: no browser configured — open it yourself:\n%s\n", u)
		return nil
	}
	if err := consoleOpen(g, u); err != nil {
		fmt.Fprintf(out, "azrl: browser launch failed (%v) — open it yourself:\n%s\n", err, u)
		return nil
	}
	fmt.Fprintf(out, "azrl: opening %s console — %s\n", providerName, u)
	return nil
}

func newConsoleCmd(providerName, short string) *cobra.Command {
	return &cobra.Command{
		Use:          "console <name>",
		Short:        short,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsole(providerName, args[0], cmd.OutOrStdout())
		},
	}
}

func init() {
	RootCmd.AddCommand(newConsoleCmd("azure", "Open the Azure portal as a profile's tenant"))
}
```

Add imports: `"io"`, `"github.com/slamb2k/azrl/internal/bridge"`, `"github.com/spf13/cobra"`.

Check `config.LoadGlobal`'s exact name/signature (it may be `config.LoadGlobal()` returning `(config.Global, error)` — mirror whatever `cmd/login.go`/`loadGlobalOrSetup` calls underneath; do NOT use `loadGlobalOrSetup` here, it launches the setup wizard on a TTY, which contradicts "never an error state" for a link-opener).

Register the group verbs — append to each slice:
- `githubSubcommands()`: `newConsoleCmd("github", "Open GitHub as a profile's account")`
- `awsSubcommands()`: `newConsoleCmd("aws", "Open the AWS access portal for a profile")`
- `gcpSubcommands()`: `newConsoleCmd("gcp", "Open the GCP console for a profile's project")`

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -count=1`
Expected: PASS (all).

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add cmd/console.go cmd/console_test.go cmd/gh.go cmd/aws.go cmd/gcp.go
git commit -m "feat(console): azrl console verb on all four provider surfaces"
```

---

### Task 4: TUI — `c` Open console (+ help overlay)

**Files:**
- Modify: `internal/ui/provider_view.go` (`providerActions`, new `consoleAction`)
- Modify: `internal/ui/tabs.go` (`helpOverlay` gains `c`)
- Test: `internal/ui/provider_view_shell_test.go` or a new sibling (append/create), `internal/ui/tabs_test.go` (extend the help-overlay test)

**Interfaces:**
- Consumes: `groupArgs`, `cliGroup`, `runHandoff` (NOT `runShellHandoff` — console returns immediately; the generic "console complete" message is right), the `providerAction` list.

- [ ] **Step 1: Write the failing tests**

Append (next to the shell-action tests, mirroring their scaffolding exactly — they already solved the constructor/driver idioms):

```go
func TestConsoleActionListedAndDispatches(t *testing.T) {
	v := /* same constructor the shell-action test uses */
	if !strings.Contains(v.View(), "Open console") {
		t.Fatalf("c Open console missing from actions:\n%s", v.View())
	}
	_, cmd := /* drive the 'c' key the same way the shell test drives 't' */
	if cmd == nil {
		t.Fatal("c on a selected profile should hand off to azrl console")
	}
}
```

Extend the help-overlay test in `internal/ui/tabs_test.go` (the one extended for `t` in the shell plan) to also assert the overlay lists `c`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestConsoleAction|Help' -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

**(a)** `providerActions` — add after the `b` Browser profile entry:

```go
		{key: "c", label: "Open console", hint: "web console as this credential", run: consoleAction},
```

**(b)** next to `shellAction`:

```go
func consoleAction(v *providerTabView) tea.Cmd {
	name := v.selected()
	if name == "" {
		return nil
	}
	args := append(groupArgs(cliGroup(v.prov.Name()), "console"), name)
	return runHandoff(args)
}
```

**(c)** `helpOverlay` in tabs.go: add `c` (`open console`) next to the `t` entry, matching the overlay's phrasing style.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -count=1`
Expected: PASS — including every pre-existing test that counts actions (`ACTIONS (6)` assertions become `(7)`; update them exactly as the shell plan's Task 4 did for 5→6, and say so in your report).

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add internal/ui/
git commit -m "feat(ui): c Open console on every tab"
```

---

### Task 5: Docs + manual-verify + final verification

**Files:**
- Modify: `README.md` (console subsection after the shell one), `CLAUDE.md` (cmd/ bullet verb lists + one console sentence; internal/ui keymap sentence gains `c`; internal/bridge bullet gains OpenURL)
- Modify: `specs/tui-ux-redesign.manual-verify.md` (append a Plan 4 section)

**Steps:**

- [ ] **Step 1: README** — after the "Shell as a profile" subsection:

```markdown
### Open the web console

`azrl console work` (also `azrl gh|aws|gcp console <name>`, `ghrl console <name>`)
opens the provider's web console as that profile — the Azure portal scoped to the
profile's tenant, the AWS access portal, the GCP console pinned to the profile's
project and signed-in account, or the GitHub host — in the profile's mapped
browser (falling back to the global `BROWSER_CMD`; remote setups launch it on
`BROWSER_HOST` over SSH). If no browser is configured or the launch fails, the
URL is printed instead.
```

- [ ] **Step 2: CLAUDE.md** — splice in the existing voice: `console` added to the four verb lists; one sentence on `cmd/console.go` (deep links from conf data — azure `#@tenant`, aws start URL, gcp project+authuser from disk-only Status, github host; profile browser override → global; degraded paths print the URL, never error); `internal/bridge/` bullet mentions `OpenURL` (tunnel-less launch); `internal/ui/` keymap sentence gains `c` Open console.

- [ ] **Step 3: manual-verify** — append to `specs/tui-ux-redesign.manual-verify.md`:

```markdown
## azrl console (Plan 4)

- [ ] `azrl console <azure-profile>` on a remote VM opens the tenant-scoped
      portal in the mapped browser profile on the laptop.
- [ ] `azrl gcp console <name>` lands on the right project AND the right
      Google account (authuser honored with multiple signed-in accounts).
- [ ] No-browser config and unreachable-host paths print the URL cleanly.
- [ ] TUI `c` opens the console and returns without disturbing the TUI.
```

- [ ] **Step 4: Whole-branch verification**

```bash
go build ./... && go test ./... && gofmt -l .
git diff main --stat
```

- [ ] **Step 5: Commit**

```bash
git add README.md CLAUDE.md specs/tui-ux-redesign.manual-verify.md
git commit -m "docs: azrl console — usage, deep links, manual-verify additions"
```

---

## Post-plan checklist (for the executor)

- Surface the six **Recorded assumptions** with the final report.
- Final whole-branch review before shipping; ship via /ship.
