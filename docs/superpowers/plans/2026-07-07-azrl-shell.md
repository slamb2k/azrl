# `azrl shell` — Ephemeral Profile Subshell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `azrl shell <name>` (and `azrl gh|aws|gcp shell <name>`, `ghrl shell <name>`) drops the user into `$SHELL` acting as the named profile — signing in first if the session is dead — without touching any directory link; the TUI gains `t` Shell as…, an honest `⌁ shell:` header chip, and `azrl status` reports the override.

**Architecture:** Plan 3 of the TUI UX redesign spec (`docs/superpowers/specs/2026-07-07-tui-ux-redesign-design.md`, section "Shell as… (`azrl shell`)"). One shared core in `cmd/shell.go`: `shellEnv` builds the per-provider env map (the same values the `.envrc` writers emit, plus `AZRL_PROFILE` and the profile's `AZRL_BROWSER_CMD`), `runShell` orchestrates liveness → login-first (by exec-ing the real `azrl … login <name>` as a child — CLI-first, zero login-flow refactoring) → banner → exec `$SHELL` with std-stream passthrough and exit-status passthrough. Liveness reuses the TUI's predicate, promoted to `provider.SessionLive`. Four cobra registrations share the one core; `ghrl` inherits via `githubSubcommands()`.

**Tech Stack:** Go, cobra, os/exec (std passthrough — repo convention, no syscall.Exec), Bubble Tea for the TUI hook, PATH-shim/fake-`$SHELL` test pattern.

## Global Constraints

- Build/test/format gates before every commit: `go build ./...`, `go test ./...`, `gofmt -l .` (empty output = clean).
- Conventional commits with scope: `feat(scope): message`.
- **Mirror, never actor (PAT-002):** shell never mutates native defaults or links; `Status`/`Ambient` stay disk-only. Signing in and exec-ing `$SHELL` are deliberate CLI actions and may spawn processes.
- **No `Provider` interface changes.** `SessionLive` is a package-level function on the existing `Status` struct.
- Env values must equal what the `.envrc` writers emit: azure `AZURE_CONFIG_DIR=<ProfilesDir>/<name>`; github `GH_CONFIG_DIR=github.ConfigDir(dir,name)`; aws isolate ? `AWS_CONFIG_FILE=<dir>/<name>/config` + `AWS_SHARED_CREDENTIALS_FILE=<dir>/<name>/credentials` : `AWS_PROFILE=<name>`; gcp isolate ? `CLOUDSDK_CONFIG=<dir>/<name>` + `CLOUDSDK_ACTIVE_CONFIG_NAME=<ResolvedConfigName>` : `CLOUDSDK_ACTIVE_CONFIG_NAME=<ResolvedConfigName>` (mirrors `selectorEnv`, internal/gcp/env.go:27).
- Marker variable exactly `AZRL_PROFILE=<provider>:<name>` (e.g. `azure:work`). Currently unused anywhere (verified).
- Banner copy exactly: `azrl: shell as <name> (<provider>) — 'exit' returns`.
- Header chip copy exactly: `⌁ shell: <name>`.
- Missing `$SHELL` → `/bin/sh`. Exit status passes through. Nested shells warn and proceed (innermost wins).
- Login failure aborts the shell (no half-authenticated subshell).
- UI language "link", never "pin".

## Recorded assumptions (surface to the user at the end)

1. **Login-first is a child self-exec** (`azrl [gh|aws|gcp] login <name>` with std passthrough), not an in-process call — the login orchestration is inline in each RunE today and the spec's CLI-first principle says verbs exec real subcommands. No re-check after a successful login (login already asserts the account).
2. **Exit-status passthrough uses `os.Exit(code)`** behind a test seam (`shellExit` var) when the subshell exits non-zero — a wrapper that re-encodes exit codes any other way lies to scripts.
3. **The TUI `t` handoff reports "shell exited" neutrally** regardless of the subshell's exit code — a user's last failing command is not an azrl error.
4. **Nested-shell warning text**: `azrl: already inside an azrl shell (<current>) — nesting; innermost wins`.
5. **`azrl status` shows the override as a single leading line** (`shell override: <provider>:<name>`) plus an `omitempty` JSON field `shell_override` — additive, no shape break.
6. **The header chip replaces the identity slot** on the matching provider's tab and suppresses the Azure drift notice (the spec: the chip appears "instead of misreporting drift"). Other providers' tabs are untouched by a foreign override.

---

### Task 1: `provider.SessionLive` (promote the liveness predicate)

**Files:**
- Modify: `internal/provider/provider.go` (append near the `Status` struct)
- Modify: `internal/ui/provider_view.go:225` (`sessionLive` delegates)
- Test: `internal/provider/provider_test.go` (append; create if there is no such file — check for an existing `*_test.go` in the package and append there instead)

**Interfaces:**
- Produces: `provider.SessionLive(st Status) bool` — Tasks 3 uses it from `cmd`.
- Consumes: nothing new.

- [ ] **Step 1: Write the failing test**

Append (to `internal/provider/provider_test.go`, or the package's existing test file):

```go
func TestSessionLive(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	cases := []struct {
		name string
		st   Status
		want bool
	}{
		{"no identity", Status{}, false},
		{"identity, nil expiry", Status{Identity: "u@x"}, true},
		{"identity, future expiry", Status{Identity: "u@x", Expiry: &future}, true},
		{"identity, past expiry", Status{Identity: "u@x", Expiry: &past}, false},
	}
	for _, c := range cases {
		if got := SessionLive(c.st); got != c.want {
			t.Fatalf("%s: SessionLive = %v, want %v", c.name, got, c.want)
		}
	}
}
```

Add `"time"` to the test file imports if missing.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/provider/ -run TestSessionLive -v`
Expected: FAIL — `undefined: SessionLive`

- [ ] **Step 3: Implement**

Append to `internal/provider/provider.go`:

```go
// SessionLive reports whether a profile's cached session is usable right now:
// an identity exists and any tracked expiry is still in the future. A nil
// expiry (github; providers whose CLIs refresh silently) counts as live.
func SessionLive(st Status) bool {
	return st.Identity != "" && (st.Expiry == nil || st.Expiry.After(time.Now()))
}
```

(Add `"time"` to provider.go imports if missing.)

In `internal/ui/provider_view.go`, replace the body of `sessionLive` with a delegation:

```go
func sessionLive(st provider.Status) bool {
	return provider.SessionLive(st)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/provider/ ./internal/ui/ -count=1`
Expected: PASS (all — the ui delegation must not change any behavior).

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add internal/provider/ internal/ui/provider_view.go
git commit -m "feat(provider): promote SessionLive liveness predicate"
```

---

### Task 2: `shellEnv` — the per-provider env map

**Files:**
- Create: `cmd/shell.go`
- Test: `cmd/shell_test.go` (create)

**Interfaces:**
- Produces: `shellEnv(providerName, name string) ([]string, error)` — returns the complete `KEY=VALUE` list including `AZRL_PROFILE` and (when mapped) `AZRL_BROWSER_CMD`; error when the profile conf doesn't exist. Task 3 consumes it.
- Consumes: `config.ProfilesDir()/GithubProfilesDir()/AwsProfilesDir()/GcpProfilesDir()`, `profile.LoadConf`, `github.LoadConf`+`github.ConfigDir`, `aws.LoadConf` (`Conf.Isolate`, `Conf.BrowserCmd`), `gcp.LoadConf` (`Conf.Isolate`, `Conf.BrowserCmd`, `Conf.ResolvedConfigName(name)`) — all existing. Verify the azure `profile.LoadConf` conf struct's browser-cmd field name (loaded from `AZ_BROWSER_CMD`, see internal/profile conf loading) and use it; if azure's loaded conf has no such field, read it via `profile.AzureScheme().GetKey(name, config.ProfilesDir(), "AZ_BROWSER_CMD")` instead.

- [ ] **Step 1: Write the failing tests**

Create `cmd/shell_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedShellHome creates a temp HOME with one conf per provider, exercising
// both plain and isolate variants. Returns the HOME path.
func seedShellHome(t *testing.T) string {
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
	write(".github-profiles/oss.conf", "GH_HOST=github.com\n")
	write(".aws-profiles/prod.conf", "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_SSO_REGION=us-east-1\n")
	write(".aws-profiles/sealed.conf", "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_ISOLATE=true\n")
	write(".gcp-profiles/lab.conf", "GCP_PROJECT=acme-lab\nGCP_CONFIG_NAME=labcfg\n")
	write(".gcp-profiles/vault.conf", "GCP_PROJECT=acme-vault\nGCP_ISOLATE=true\n")
	return home
}

func envHas(t *testing.T, env []string, want string) {
	t.Helper()
	for _, e := range env {
		if e == want {
			return
		}
	}
	t.Fatalf("env missing %q:\n%s", want, strings.Join(env, "\n"))
}

func envLacksKey(t *testing.T, env []string, key string) {
	t.Helper()
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			t.Fatalf("env must not carry %s: %q", key, e)
		}
	}
}

func TestShellEnvPerProvider(t *testing.T) {
	home := seedShellHome(t)

	env, err := shellEnv("azure", "work")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "AZURE_CONFIG_DIR="+filepath.Join(home, ".azure-profiles", "work"))
	envHas(t, env, "AZRL_PROFILE=azure:work")
	envHas(t, env, "AZRL_BROWSER_CMD=chrome --profile-directory=Work")

	env, err = shellEnv("github", "oss")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "GH_CONFIG_DIR="+filepath.Join(home, ".github-profiles", "oss"))
	envHas(t, env, "AZRL_PROFILE=github:oss")
	envLacksKey(t, env, "AZRL_BROWSER_CMD") // no mapping on this profile

	env, err = shellEnv("aws", "prod")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "AWS_PROFILE=prod")
	envHas(t, env, "AZRL_PROFILE=aws:prod")
	envLacksKey(t, env, "AWS_CONFIG_FILE")

	env, err = shellEnv("aws", "sealed")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "AWS_CONFIG_FILE="+filepath.Join(home, ".aws-profiles", "sealed", "config"))
	envHas(t, env, "AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(home, ".aws-profiles", "sealed", "credentials"))
	envLacksKey(t, env, "AWS_PROFILE")

	env, err = shellEnv("gcp", "lab")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "CLOUDSDK_ACTIVE_CONFIG_NAME=labcfg")
	envLacksKey(t, env, "CLOUDSDK_CONFIG")

	env, err = shellEnv("gcp", "vault")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "CLOUDSDK_CONFIG="+filepath.Join(home, ".gcp-profiles", "vault"))
	envHas(t, env, "CLOUDSDK_ACTIVE_CONFIG_NAME=vault") // config name defaults to profile name
}

func TestShellEnvUnknownProfileErrors(t *testing.T) {
	seedShellHome(t)
	if _, err := shellEnv("azure", "nope"); err == nil {
		t.Fatal("unknown profile should error")
	}
	if _, err := shellEnv("aws", "nope"); err == nil {
		t.Fatal("unknown aws profile should error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run TestShellEnv -v`
Expected: FAIL — `undefined: shellEnv`

- [ ] **Step 3: Implement**

Create `cmd/shell.go` (env-builder half; `runShell` and the cobra commands come in Task 3):

```go
package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/gcp"
	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/profile"
)

// shellEnv builds the env pairs a subshell needs to act as the profile — the
// same values the .envrc writers emit — plus the AZRL_PROFILE marker and the
// profile's mapped browser command (so git push / az login inside the
// subshell route to the right browser profile).
func shellEnv(providerName, name string) ([]string, error) {
	var env []string
	browser := ""
	switch providerName {
	case "azure":
		dir := config.ProfilesDir()
		c, err := profile.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown azure profile %q: %w", name, err)
		}
		env = append(env, "AZURE_CONFIG_DIR="+filepath.Join(dir, name))
		browser = c.BrowserCmd
	case "github":
		dir := config.GithubProfilesDir()
		c, err := github.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown github profile %q: %w", name, err)
		}
		env = append(env, "GH_CONFIG_DIR="+github.ConfigDir(dir, name))
		browser = c.BrowserCmd
	case "aws":
		dir := config.AwsProfilesDir()
		c, err := aws.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown aws profile %q: %w", name, err)
		}
		if c.Isolate {
			env = append(env,
				"AWS_CONFIG_FILE="+filepath.Join(dir, name, "config"),
				"AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(dir, name, "credentials"))
		} else {
			env = append(env, "AWS_PROFILE="+name)
		}
		browser = c.BrowserCmd
	case "gcp":
		dir := config.GcpProfilesDir()
		c, err := gcp.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown gcp profile %q: %w", name, err)
		}
		if c.Isolate {
			env = append(env, "CLOUDSDK_CONFIG="+filepath.Join(dir, name))
		}
		env = append(env, "CLOUDSDK_ACTIVE_CONFIG_NAME="+c.ResolvedConfigName(name))
		browser = c.BrowserCmd
	default:
		return nil, fmt.Errorf("azrl: unknown provider %q", providerName)
	}
	if browser != "" {
		env = append(env, "AZRL_BROWSER_CMD="+browser)
	}
	env = append(env, "AZRL_PROFILE="+providerName+":"+name)
	return env, nil
}
```

Note: if the azure `profile.LoadConf` conf struct's browser field is not named `BrowserCmd`, use its actual name (it loads `AZ_BROWSER_CMD`); if no field exists, fall back to `profile.AzureScheme().GetKey(name, dir, "AZ_BROWSER_CMD")` and note it in your report.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -run TestShellEnv -v`
Expected: PASS (both).

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add cmd/shell.go cmd/shell_test.go
git commit -m "feat(shell): per-provider subshell env map"
```

---

### Task 3: `runShell` + cobra wiring on all four surfaces

**Files:**
- Modify: `cmd/shell.go` (append), `cmd/gh.go` (`githubSubcommands`), `cmd/aws.go` (`awsSubcommands`), `cmd/gcp.go` (`gcpSubcommands`)
- Test: `cmd/shell_test.go` (append)

**Interfaces:**
- Produces: `runShell(providerName, name string, out io.Writer) error`; seams `var shellLoginRunner = runShellLogin` and `var shellExit = os.Exit`; cobra verbs `azrl shell`, `azrl gh|aws|gcp shell` (ghrl inherits `shell` via `githubSubcommands()`).
- Consumes: `shellEnv` (Task 2), `provider.SessionLive` (Task 1), `provider.All()` registry.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/shell_test.go`:

```go
// fakeShell installs a fake $SHELL that logs selected env vars, and returns
// the log path. exitCode is the fake shell's exit status.
func fakeShell(t *testing.T, exitCode int) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "shell.log")
	script := "#!/bin/sh\n" +
		"{ echo \"AZRL_PROFILE=$AZRL_PROFILE\"; echo \"AZURE_CONFIG_DIR=$AZURE_CONFIG_DIR\"; " +
		"echo \"AWS_PROFILE=$AWS_PROFILE\"; echo \"AZRL_BROWSER_CMD=$AZRL_BROWSER_CMD\"; } >> \"" + log + "\"\n" +
		fmt.Sprintf("exit %d\n", exitCode)
	sh := filepath.Join(bin, "fakeshell")
	if err := os.WriteFile(sh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", sh)
	return log
}

// liveAzureSession marks the azure profile's isolated dir as signed in so
// runShell skips the login-first path (azure Status reads azureProfile.json).
func liveAzureSession(t *testing.T, home, name string) {
	t.Helper()
	p := filepath.Join(home, ".azure-profiles", name, "azureProfile.json")
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p,
		[]byte(`{"subscriptions":[{"user":{"name":"u@acme.com"},"isDefault":true,"tenantId":"g1"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunShellExecsShellWithEnvAndBanner(t *testing.T) {
	home := seedShellHome(t)
	liveAzureSession(t, home, "work")
	log := fakeShell(t, 0)
	t.Setenv("AZRL_PROFILE", "")

	var out strings.Builder
	if err := runShell("azure", "work", &out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	got := string(b)
	for _, want := range []string{
		"AZRL_PROFILE=azure:work",
		"AZURE_CONFIG_DIR=" + filepath.Join(home, ".azure-profiles", "work"),
		"AZRL_BROWSER_CMD=chrome --profile-directory=Work",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("subshell env missing %q; got:\n%s", want, got)
		}
	}
	if !strings.Contains(out.String(), "azrl: shell as work (azure) — 'exit' returns") {
		t.Fatalf("banner missing: %q", out.String())
	}
}

func TestRunShellSignsInFirstWhenSessionDead(t *testing.T) {
	seedShellHome(t) // no azureProfile.json → session dead
	fakeShell(t, 0)
	t.Setenv("AZRL_PROFILE", "")

	var calls [][]string
	orig := shellLoginRunner
	shellLoginRunner = func(providerName, name string) error {
		calls = append(calls, []string{providerName, name})
		return nil
	}
	defer func() { shellLoginRunner = orig }()

	var out strings.Builder
	if err := runShell("azure", "work", &out); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0][0] != "azure" || calls[0][1] != "work" {
		t.Fatalf("expected one login call for azure:work, got %v", calls)
	}
}

func TestRunShellAbortsWhenLoginFails(t *testing.T) {
	seedShellHome(t)
	log := fakeShell(t, 0)
	t.Setenv("AZRL_PROFILE", "")

	orig := shellLoginRunner
	shellLoginRunner = func(string, string) error { return fmt.Errorf("boom") }
	defer func() { shellLoginRunner = orig }()

	var out strings.Builder
	if err := runShell("azure", "work", &out); err == nil {
		t.Fatal("login failure must abort the shell")
	}
	if b, _ := os.ReadFile(log); len(b) != 0 {
		t.Fatalf("no subshell may start after a failed login; log: %s", b)
	}
}

func TestRunShellWarnsOnNesting(t *testing.T) {
	home := seedShellHome(t)
	liveAzureSession(t, home, "work")
	fakeShell(t, 0)
	t.Setenv("AZRL_PROFILE", "gcp:lab")

	var out strings.Builder
	if err := runShell("azure", "work", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already inside an azrl shell (gcp:lab)") {
		t.Fatalf("nesting warning missing: %q", out.String())
	}
}

func TestRunShellPassesExitStatusThrough(t *testing.T) {
	home := seedShellHome(t)
	liveAzureSession(t, home, "work")
	fakeShell(t, 3)
	t.Setenv("AZRL_PROFILE", "")

	code := -1
	orig := shellExit
	shellExit = func(c int) { code = c }
	defer func() { shellExit = orig }()

	var out strings.Builder
	if err := runShell("azure", "work", &out); err != nil {
		t.Fatal(err)
	}
	if code != 3 {
		t.Fatalf("exit status not passed through: got %d, want 3", code)
	}
}

func TestShellVerbRegisteredOnAllSurfaces(t *testing.T) {
	find := func(cmds []*cobra.Command) bool {
		for _, c := range cmds {
			if strings.HasPrefix(c.Use, "shell") {
				return true
			}
		}
		return false
	}
	if !find(RootCmd.Commands()) {
		t.Fatal("azrl shell not registered")
	}
	if !find(githubSubcommands()) {
		t.Fatal("gh shell not registered")
	}
	if !find(awsSubcommands()) {
		t.Fatal("aws shell not registered")
	}
	if !find(gcpSubcommands()) {
		t.Fatal("gcp shell not registered")
	}
}
```

Add `"fmt"` and `"github.com/spf13/cobra"` to the test file imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run 'TestRunShell|TestShellVerb' -v`
Expected: FAIL — `undefined: runShell`, `shellLoginRunner`, `shellExit`.

- [ ] **Step 3: Implement**

Append to `cmd/shell.go`:

```go
// shellLoginRunner and shellExit are test seams: login-first execs the real
// azrl login as a child, and exit-status passthrough must not kill the test
// binary.
var (
	shellLoginRunner = runShellLogin
	shellExit        = os.Exit
)

// runShellLogin signs the profile in by exec-ing the real login verb as a
// child (bridge, browser mapping, everything — CLI-first). The promoted ghrl
// binary has github verbs at top level, so the gh group prefix is dropped
// there, mirroring internal/ui's groupArgs.
func runShellLogin(providerName, name string) error {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "azrl"
	}
	var args []string
	switch providerName {
	case "azure":
		args = []string{"login", name}
	case "github":
		if strings.TrimSuffix(filepath.Base(self), ".exe") == "ghrl" {
			args = []string{"login", name}
		} else {
			args = []string{"gh", "login", name}
		}
	default:
		args = []string{providerName, "login", name}
	}
	c := exec.Command(self, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// shellNeedsLogin reports whether the profile's cached session is unusable —
// disk-only, via the provider registry's Status.
func shellNeedsLogin(providerName, name string) bool {
	for _, p := range provider.All() {
		if p.Name() != providerName {
			continue
		}
		st, err := p.Status(name, p.ProfilesDir())
		return err != nil || !provider.SessionLive(st)
	}
	return true
}

// runShell drops the user into $SHELL acting as the profile: sign in first if
// the session is dead, then exec the shell with the profile's env map. No
// directory link is read or written.
func runShell(providerName, name string, out io.Writer) error {
	env, err := shellEnv(providerName, name)
	if err != nil {
		return err
	}
	if shellNeedsLogin(providerName, name) {
		fmt.Fprintf(out, "azrl: no live session for %s:%s — signing in first\n", providerName, name)
		if err := shellLoginRunner(providerName, name); err != nil {
			return fmt.Errorf("azrl: sign-in failed — not starting a shell: %w", err)
		}
	}
	if cur := os.Getenv("AZRL_PROFILE"); cur != "" {
		fmt.Fprintf(out, "azrl: already inside an azrl shell (%s) — nesting; innermost wins\n", cur)
	}
	fmt.Fprintf(out, "azrl: shell as %s (%s) — 'exit' returns\n", name, providerName)
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	c := exec.Command(sh)
	c.Env = append(os.Environ(), env...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			shellExit(ee.ExitCode())
			return nil
		}
		return fmt.Errorf("azrl: shell failed to start: %w", err)
	}
	return nil
}

func newShellCmd(providerName, short string) *cobra.Command {
	return &cobra.Command{
		Use:          "shell <name>",
		Short:        short,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShell(providerName, args[0], cmd.OutOrStdout())
		},
	}
}

func init() {
	RootCmd.AddCommand(newShellCmd("azure", "Open a subshell acting as an Azure profile (no link)"))
}
```

Add the needed imports to cmd/shell.go: `"errors"`, `"io"`, `"os"`, `"os/exec"`, `"strings"`, `"github.com/spf13/cobra"`, `"github.com/slamb2k/azrl/internal/provider"`.

Register the group verbs — append one entry to each slice:
- `cmd/gh.go` `githubSubcommands()`: `newShellCmd("github", "Open a subshell acting as a GitHub profile (no link)")`
- `cmd/aws.go` `awsSubcommands()`: `newShellCmd("aws", "Open a subshell acting as an AWS profile (no link)")`
- `cmd/gcp.go` `gcpSubcommands()`: `newShellCmd("gcp", "Open a subshell acting as a GCP profile (no link)")`

(`ghrl shell` comes for free via `githubSubcommands()` promotion.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -count=1`
Expected: PASS (all new + pre-existing).

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add cmd/shell.go cmd/shell_test.go cmd/gh.go cmd/aws.go cmd/gcp.go
git commit -m "feat(shell): azrl shell verb on all four provider surfaces"
```

---

### Task 4: TUI — `t` Shell as…, header chip, drift suppression

**Files:**
- Modify: `internal/ui/provider_view.go` (`providerActions` list ~line 88, `reload` ~line 111, `identityStrip` ~line 569, new `shellAction`)
- Modify: `internal/ui/actions.go` (append `runShellHandoff`)
- Modify: `internal/ui/azure_view.go` (`syncHeader` ~line 91)
- Test: `internal/ui/provider_view_shell_test.go` (create), `internal/ui/azure_view_test.go` (append)

**Interfaces:**
- Consumes: `groupArgs`, `runHandoff` pattern, `providerAction`/`actionState`, `headerStrip`, `keycap`, `accentStyle`/`mutedStyle`.
- Produces: `providerTabView.shellName string` (set from `AZRL_PROFILE` when its provider prefix matches the tab), `runShellHandoff(args []string) tea.Cmd`, `shellAction`.

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/provider_view_shell_test.go` (mirror the construction idioms of the existing aws/gcp view tests — look at `aws_view_test.go` for how a `providerTabView` is built and driven; reuse its helpers if exported within the package):

```go
package ui

import (
	"strings"
	"testing"
	tea "github.com/charmbracelet/bubbletea"
)

func TestShellActionListedAndDispatches(t *testing.T) {
	v := newTestAwsView(t) // use the same constructor helper the aws view tests use; adapt the name to what exists
	if !strings.Contains(v.View(), "Shell as") {
		t.Fatalf("t Shell as… missing from actions:\n%s", v.View())
	}
	_, cmd := v.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if cmd == nil {
		t.Fatal("t on a selected profile should hand off to azrl shell")
	}
}

func TestShellOverrideChipInHeader(t *testing.T) {
	t.Setenv("AZRL_PROFILE", "aws:prod")
	v := newTestAwsView(t)
	v.reload()
	if !strings.Contains(v.View(), "⌁ shell: prod") {
		t.Fatalf("shell override chip missing:\n%s", v.View())
	}
}

func TestForeignShellOverrideIgnored(t *testing.T) {
	t.Setenv("AZRL_PROFILE", "gcp:lab")
	v := newTestAwsView(t)
	v.reload()
	if strings.Contains(v.View(), "⌁ shell:") {
		t.Fatalf("foreign provider override must not show a chip:\n%s", v.View())
	}
}
```

**Implementer note:** `newTestAwsView` is a placeholder name — read `internal/ui/aws_view_test.go` first and use the actual pattern that file uses to construct and drive a provider tab (constructor, `update` method name, `reload`). Keep the three behaviors asserted exactly; adapt only the scaffolding. Also check `update`'s dispatch path: accelerator keys are matched in the update default case (provider_view.go:377) via the action list, so once the action exists `t` dispatches generically.

Append to `internal/ui/azure_view_test.go`:

```go
func TestAzureDriftNoticeSuppressedUnderShellOverride(t *testing.T) {
	t.Setenv("AZRL_PROFILE", "azure:work")
	v := newTestAzureView(t) // same note as above: use this file's existing constructor helper
	v.drift = true           // or drive whatever field/message the existing drift tests use
	v.syncHeader()
	if strings.Contains(v.notice, "⚠ shell az") {
		t.Fatalf("drift notice must be suppressed under a shell override: %q", v.notice)
	}
	if !strings.Contains(v.View(), "⌁ shell: work") {
		t.Fatalf("override chip missing on azure tab:\n%s", v.View())
	}
}
```

(Adapt field/method names to the real ones in azure_view.go / its tests — the behaviors to pin: drift warning absent, chip present.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestShell|TestAzureDriftNoticeSuppressed' -v`
Expected: FAIL — no "Shell as" action, no chip rendering.

- [ ] **Step 3: Implement**

**(a)** `internal/ui/actions.go` — append:

```go
// runShellHandoff suspends the TUI into `azrl … shell <name>` and reports the
// return neutrally: the subshell's own exit status is the user's business
// (their last command failing is not an azrl error).
func runShellHandoff(args []string) tea.Cmd {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "azrl"
	}
	c := exec.Command(self, args...)
	return tea.ExecProcess(c, func(error) tea.Msg {
		return opDoneMsg{msg: "shell exited"}
	})
}
```

**(b)** `internal/ui/provider_view.go`:

Add to `providerActions` (after the `s` entry, so the radio order reads sign-in → shell):

```go
		{key: "t", label: "Shell as…", hint: "subshell as this profile — no link", run: shellAction},
```

Add next to `loginAction`:

```go
func shellAction(v *providerTabView) tea.Cmd {
	name := v.selected()
	if name == "" {
		return nil
	}
	args := append(groupArgs(cliGroup(v.prov.Name()), "shell"), name)
	return runShellHandoff(args)
}
```

(Verify `cliGroup` is the group-name helper `loginAction` uses; mirror exactly how `loginAction` builds its argv.)

Add a `shellName string` field to `providerTabView` and set it in `reload()`:

```go
	v.shellName = ""
	if ov := os.Getenv("AZRL_PROFILE"); ov != "" {
		if prov, prof, ok := strings.Cut(ov, ":"); ok && prov == v.prov.Name() {
			v.shellName = prof
		}
	}
```

In `identityStrip()`, when `v.shellName != ""`, replace the identity slot with the chip:

```go
	ident := effectiveIdentity(v.dirProfile, dirIdentity, v.ambIdent)
	if v.identityOverride != "" {
		ident = v.identityOverride
	}
	if v.shellName != "" {
		ident = accentStyle.Render("⌁ shell: " + v.shellName)
	}
```

(Adapt to the function's actual local variable names — the behavior: shell chip wins over both the disk identity and `identityOverride`.)

**(c)** `internal/ui/azure_view.go` — in `syncHeader()`, before the drift branch:

```go
	if v.tab.shellName != "" { // adapt receiver/field path to the real structure
		v.notice = ""
		return
	}
```

(The chip itself renders via the shared `identityStrip`; azure only needs to stop warning about drift the subshell has made moot.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -count=1`
Expected: PASS — all new + pre-existing (no existing header/drift test may regress; if one asserts the old identity slot under an `AZRL_PROFILE` env leak, clear the env in that test with `t.Setenv("AZRL_PROFILE", "")`).

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add internal/ui/
git commit -m "feat(ui): t Shell as… with shell-override header chip"
```

---

### Task 5: `azrl status` reports the shell override

**Files:**
- Modify: `cmd/status.go` (`statusReport` struct, `RunE` build, `printStatusSections`)
- Test: `cmd/status_test.go` (append)

**Interfaces:**
- Produces: plain leading line `shell override: <provider>:<name> — this terminal acts as <name>` when `AZRL_PROFILE` is set; JSON field `"shell_override"` (omitempty).
- Consumes: nothing new.

- [ ] **Step 1: Write the failing test**

Append to `cmd/status_test.go`:

```go
func TestStatusReportsShellOverride(t *testing.T) {
	seedStatusHome(t)
	t.Setenv("AZRL_PROFILE", "azure:work")

	statusJSON = false
	out := runRoot(t, "status")
	if !strings.Contains(out, "shell override: azure:work") {
		t.Fatalf("plain status missing shell override line:\n%s", out)
	}

	out = runRoot(t, "status", "--json")
	statusJSON = false
	if !strings.Contains(out, `"shell_override": "azure:work"`) {
		t.Fatalf("json status missing shell_override:\n%s", out)
	}
}

func TestStatusOmitsShellOverrideWhenUnset(t *testing.T) {
	seedStatusHome(t)
	t.Setenv("AZRL_PROFILE", "")

	statusJSON = false
	out := runRoot(t, "status")
	if strings.Contains(out, "shell override") {
		t.Fatalf("no override line expected:\n%s", out)
	}
	out = runRoot(t, "status", "--json")
	statusJSON = false
	if strings.Contains(out, "shell_override") {
		t.Fatalf("omitempty field leaked into JSON:\n%s", out)
	}
}
```

(Match the JSON indentation style of the existing test assertions — check how `TestStatusCmdJSONEmptySectionsAreArrays` matches keys and mirror it.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run TestStatusReportsShellOverride -v`
Expected: FAIL — no override line.

- [ ] **Step 3: Implement**

In `cmd/status.go`:

- Add to `statusReport`: `ShellOverride string \`json:"shell_override,omitempty"\``.
- In the status RunE, after building the report: `rep.ShellOverride = os.Getenv("AZRL_PROFILE")`.
- In `printStatusSections`, first thing:

```go
	if rep.ShellOverride != "" {
		_, prof, _ := strings.Cut(rep.ShellOverride, ":")
		fmt.Fprintf(w, "shell override: %s — this terminal acts as %s\n\n", rep.ShellOverride, prof)
	}
```

(If `printStatusSections` doesn't receive `rep`, pass the string as a parameter or print it in RunE before calling — pick whichever touches less; keep the exact copy.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/ -count=1`
Expected: PASS.

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add cmd/status.go cmd/status_test.go
git commit -m "feat(status): report the AZRL_PROFILE shell override"
```

---

### Task 6: Docs + manual-verify + final verification

**Files:**
- Modify: `README.md` (new "Shell as a profile" subsection with the prompt snippet), `CLAUDE.md` (cmd/ bullet, internal/ui bullet, configuration-model note for `AZRL_PROFILE`)
- Create: `specs/tui-ux-redesign.manual-verify.md`

**Steps:**

- [ ] **Step 1: README**

Find the section documenting everyday verbs (near where `use`/`login` are described — inspect the README's structure and place it in its voice) and add a short subsection:

```markdown
### Shell as a profile

`azrl shell work` (also `azrl gh|aws|gcp shell <name>`, `ghrl shell <name>`) opens
your `$SHELL` acting as that profile — no directory link is touched, and `exit`
returns you to your normal identity. If the session is dead it signs you in
first. Inside the subshell `AZRL_PROFILE` is set (e.g. `azure:work`) and the
profile's browser mapping is exported as `AZRL_BROWSER_CMD`, so `git push` and
`az login` inside the subshell open the right browser profile.

Show it in your prompt:

```sh
# bash (.bashrc)
PS1='${AZRL_PROFILE:+[$AZRL_PROFILE] }'"$PS1"
# zsh (.zshrc)
setopt PROMPT_SUBST; PROMPT='${AZRL_PROFILE:+[$AZRL_PROFILE] }'"$PROMPT"
```

```toml
# starship.toml
[env_var.AZRL_PROFILE]
format = "[$env_value]($style) "
style = "bold yellow"
```
```

- [ ] **Step 2: CLAUDE.md**

- `cmd/` bullet: add `shell` to the verb lists — top-level Azure (`login`, `capture`, `use`, `rm`, `list`, `shell`), the gh/aws/gcp groups, and one sentence: `shell <name>` opens `$SHELL` as the profile (sign-in first when the session is dead via a child `login`, env map = the .envrc values + `AZRL_PROFILE=<provider>:<name>` + the profile's `AZRL_BROWSER_CMD`; exit status passes through; nested shells warn, innermost wins).
- `internal/ui/` bullet: add `t` Shell as… to the keymap sentence, and note the header shows `⌁ shell: <name>` (suppressing the Azure drift notice) when `AZRL_PROFILE` matches the tab's provider.
- Configuration model: one line noting `AZRL_PROFILE` is the subshell marker set by `azrl shell` (read by the TUI header and `azrl status`; never persisted).

- [ ] **Step 3: Manual-verify checklist**

Create `specs/tui-ux-redesign.manual-verify.md`:

```markdown
# TUI UX redesign — manual verification (real laptop + VM)

Items only a real environment can exercise; extend this file as later
redesign plans (console, mouse) ship.

## azrl shell (Plan 3)

- [ ] `azrl shell <azure-profile>` from a linked repo: subshell `az account show`
      shows the profile's account; `exit` restores the outer identity.
- [ ] Dead-session path: expire/clear the profile's session, `azrl shell` runs
      the full bridged login (browser pops on the local machine) before the
      subshell starts; a failed/cancelled login starts no shell.
- [ ] `git push` inside a subshell with a mapped browser profile opens the
      mapped browser (AZRL_BROWSER_CMD narrowing of the GCM limitation).
- [ ] Nested `azrl shell` warns and the innermost identity wins.
- [ ] Prompt snippet (bash or starship) shows `[azure:work]` inside the shell.
- [ ] TUI `t` suspends into the subshell and reloads cleanly on exit; the
      header chip `⌁ shell: <name>` shows inside a subshell-launched TUI and
      the Azure drift warning stays quiet.
```

- [ ] **Step 4: Whole-branch verification**

```bash
go build ./... && go test ./... && gofmt -l .
git diff main --stat
```

Expected: build OK, full suite PASS, gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add README.md CLAUDE.md specs/tui-ux-redesign.manual-verify.md
git commit -m "docs: azrl shell — usage, prompt snippet, manual-verify checklist"
```

---

## Post-plan checklist (for the executor)

- Surface the six **Recorded assumptions** with the final report.
- Final whole-branch review before shipping; ship via /ship.
- Real-machine items live in `specs/tui-ux-redesign.manual-verify.md` — flag them as pending, don't attempt them headless.
