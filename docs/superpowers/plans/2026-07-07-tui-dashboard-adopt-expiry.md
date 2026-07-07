# TUI Plan 2: Dashboard Adopt-from-Ambient + Expiry Semantics — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unmanaged AMBIENT dashboard rows become adoptable via a prefilled name prompt (backed by capture commands that actually derive from the ambient session), and expiry display becomes per-provider guidance: AWS three-state (amber <15 min / expired / nothing), Azure/GCP nothing on rows with a truthful DETAILS line, GitHub nothing.

**Architecture:** Plan 2 of the TUI UX redesign spec (`docs/superpowers/specs/2026-07-07-tui-ux-redesign-design.md`, sections "Dashboard adopt, extended" and "Expiry: guidance, not telemetry"). Backend first: `capture` learns to derive missing fields from each provider's ambient state (AWS config stanza, GCP active configuration, GitHub ambient session fallback; Azure already derives via `az account show`). Then the dashboard: a name prompt replaces silent auto-naming for adopt, unmanaged ambient rows join the adoptable set, and all expiry rendering routes through a single `ExpiryActionable(provider)` gate (`true` only for `"aws"`).

**Tech Stack:** Go, Bubble Tea (`bubbles/textinput`), existing PATH-shim test pattern, existing `View()`-assertion TUI test pattern.

## Global Constraints

- Build/test/format gates: `go build ./...`, `go test ./...`, `gofmt -l .` (empty output = clean). Run all three before every commit.
- Conventional commits with scope: `feat(scope): message`.
- **Mirror, never actor (PAT-002):** `Status`/`Ambient`/`BuildOverview` stay disk-only, never spawn CLIs, never network. `capture` (a deliberate CLI command) may spawn CLIs — it already does for Azure/GitHub.
- **No `Provider` interface changes** (spec Testing section). New derivation funcs are plain package-level functions.
- UI language is "link", never "pin" (spec Terminology section).
- Expiring-soon threshold: **15 minutes** (spec: "expiring soon (< 15 min)").
- The only expiry-actionable provider is `"aws"`. Azure/GCP access tokens refresh silently on next use; GitHub has no expiry.
- Prompt copy (matches provider-tab capture prompt): `"Name for the adopted profile:"`, confirm hint `"adopt session + link"`.
- Do not touch `azrl status --json` (raw data stays for scripts) and do not touch the UNMAPPED section's `plainExpiry` column in plain `azrl status` (a deliberately-invoked inspection, like DETAILS).

## Recorded assumptions (surface to the user at the end)

1. **Amber marker copy** is `⚠ expires in <dur>` in `accentStyle`; spec says only "small amber marker".
2. **In-play** = any MAPPINGS row (a link is a dir linkage) or a managed AMBIENT row. UNMAPPED rows are not in play: their expiry text is removed entirely; their expired *hint* is kept but AWS-gated (spec says hint prioritization "stays as shipped").
3. **No amber/expired markers on provider-tab profile rows** — they never showed expiry and the spec puts exact truth in DETAILS only.
4. **Plain `azrl status`**: the MAPPINGS `expired` note gets the AWS gate (it is guidance-shaped); the UNMAPPED expiry column is left as-is (deliberate check).
5. **Capture derivation does not auto-arm guardrails** (`AWS_EXPECT_*`, `GCP_EXPECT_ACCOUNT` stay flag-only). GCP derivation *does* bind the ambient config name (`GCP_CONFIG_NAME`) so the adopted profile maps onto the live session.
6. **No new dashboard hint** for unmanaged ambient rows or expired ambient rows (spec: "hint prioritization stays as shipped"); the row tags carry the signal.

---

### Task 1: AWS capture derives SSO fields from the ambient config

**Files:**
- Modify: `internal/aws/ambient.go` (append)
- Modify: `cmd/aws.go` (`newAwsCaptureCmd` RunE, ~lines 206-248)
- Test: `internal/aws/ambient_test.go` (append)

**Interfaces:**
- Produces: `aws.CaptureDefaults() Conf` — zero-value `Conf` fields when nothing derivable. Fields used: `SSOStartURL`, `SSORegion`, `AccountID`, `RoleName`.
- Consumes: existing `configSection(path, section string) map[string]string` (ambient.go:46), `provider.EnvOrHome("AWS_CONFIG_FILE", ".aws", "config")`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/aws/ambient_test.go`:

```go
func TestCaptureDefaultsResolvesSsoSession(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	os.WriteFile(cfg, []byte(`[default]
sso_session = corp
sso_account_id = 111122223333
sso_role_name = Dev

[sso-session corp]
sso_start_url = https://corp.awsapps.com/start
sso_region = us-east-1
`), 0o644)
	t.Setenv("AWS_CONFIG_FILE", cfg)
	t.Setenv("AWS_PROFILE", "")

	c := CaptureDefaults()
	if c.SSOStartURL != "https://corp.awsapps.com/start" || c.SSORegion != "us-east-1" {
		t.Fatalf("sso-session indirection not resolved: %+v", c)
	}
	if c.AccountID != "111122223333" || c.RoleName != "Dev" {
		t.Fatalf("stanza fields missing: %+v", c)
	}
}

func TestCaptureDefaultsLegacyInlineAndAwsProfile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	os.WriteFile(cfg, []byte(`[profile prod]
sso_start_url = https://legacy.awsapps.com/start
sso_region = eu-west-1
sso_account_id = 999988887777
sso_role_name = Admin
`), 0o644)
	t.Setenv("AWS_CONFIG_FILE", cfg)
	t.Setenv("AWS_PROFILE", "prod")

	c := CaptureDefaults()
	if c.SSOStartURL != "https://legacy.awsapps.com/start" || c.AccountID != "999988887777" {
		t.Fatalf("AWS_PROFILE stanza not read: %+v", c)
	}
}

func TestCaptureDefaultsZeroOnMissingState(t *testing.T) {
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("AWS_PROFILE", "")
	if c := CaptureDefaults(); c != (Conf{}) {
		t.Fatalf("expected zero Conf, got %+v", c)
	}
}
```

If `ambient_test.go` lacks `os`/`filepath` imports, add them.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/aws/ -run TestCaptureDefaults -v`
Expected: FAIL — `undefined: CaptureDefaults`

- [ ] **Step 3: Implement `CaptureDefaults`**

Append to `internal/aws/ambient.go`:

```go
// CaptureDefaults returns the SSO fields behind the ambient aws identity —
// the stanza AWS_PROFILE names, else [default] — resolving an sso_session
// indirection when present. It backs flag-less `azrl aws capture`, so adopting
// the ambient identity records a profile that can actually sign in.
// Best-effort and disk-only: missing or unparseable state yields zero values.
func CaptureDefaults() Conf {
	path, _, ok := provider.EnvOrHome("AWS_CONFIG_FILE", ".aws", "config")
	if !ok {
		return Conf{}
	}
	section := "default"
	if p := os.Getenv("AWS_PROFILE"); p != "" {
		section = "profile " + p
	}
	sec := configSection(path, section)
	if sec == nil {
		return Conf{}
	}
	c := Conf{
		SSOStartURL: sec["sso_start_url"], SSORegion: sec["sso_region"],
		AccountID: sec["sso_account_id"], RoleName: sec["sso_role_name"],
	}
	if s := sec["sso_session"]; s != "" {
		if ses := configSection(path, "sso-session "+s); ses != nil {
			if c.SSOStartURL == "" {
				c.SSOStartURL = ses["sso_start_url"]
			}
			if c.SSORegion == "" {
				c.SSORegion = ses["sso_region"]
			}
		}
	}
	return c
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/aws/ -run TestCaptureDefaults -v`
Expected: PASS (all three)

- [ ] **Step 5: Wire into the capture command**

In `cmd/aws.go`, `newAwsCaptureCmd` RunE, immediately after the `conf := aws.Conf{...}` literal (before the `aws.LoadConf` block):

```go
			def := aws.CaptureDefaults()
			if conf.SSOStartURL == "" {
				conf.SSOStartURL = def.SSOStartURL
			}
			if conf.SSORegion == "" {
				conf.SSORegion = def.SSORegion
			}
			if conf.AccountID == "" {
				conf.AccountID = def.AccountID
			}
			if conf.RoleName == "" {
				conf.RoleName = def.RoleName
			}
```

And change the final print from `cmd.Printf("aws: captured %s into profile %q\n", startURL, name)` to:

```go
			cmd.Printf("aws: captured %s into profile %q\n", conf.SSOStartURL, name)
```

- [ ] **Step 6: Full gates**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: build OK, all tests PASS, no gofmt output.

- [ ] **Step 7: Commit**

```bash
git add internal/aws/ambient.go internal/aws/ambient_test.go cmd/aws.go
git commit -m "feat(aws): capture derives SSO fields from the ambient config"
```

---

### Task 2: GCP capture derives project/config from the ambient configuration

**Files:**
- Modify: `internal/gcp/ambient.go` (append)
- Modify: `cmd/gcp.go` (`newGcpCaptureCmd` RunE, lines 214-256)
- Test: `internal/gcp/ambient_test.go` (append)

**Interfaces:**
- Produces: `gcp.CaptureDefaults() Conf` — fields used: `ConfigName`, `Project`, `Region`. Zero-value when nothing derivable.
- Consumes: existing `iniValue(body, section, key string) string` (status.go:53), `provider.EnvOrHome("CLOUDSDK_CONFIG", ".config", "gcloud")`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/gcp/ambient_test.go` (mirror the env/dir seeding of `TestAmbientReadsActiveConfig` at line 47):

```go
func TestCaptureDefaultsReadsActiveConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", dir)
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	os.WriteFile(filepath.Join(dir, "active_config"), []byte("work\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "configurations"), 0o755)
	os.WriteFile(filepath.Join(dir, "configurations", "config_work"), []byte(`[core]
account = dev@example.com
project = acme-prod

[compute]
region = australiaeast
`), 0o644)

	c := CaptureDefaults()
	if c.ConfigName != "work" || c.Project != "acme-prod" || c.Region != "australiaeast" {
		t.Fatalf("ambient config not derived: %+v", c)
	}
}

func TestCaptureDefaultsZeroOnMissingState(t *testing.T) {
	t.Setenv("CLOUDSDK_CONFIG", t.TempDir())
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	if c := CaptureDefaults(); c != (Conf{}) {
		t.Fatalf("expected zero Conf, got %+v", c)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/gcp/ -run TestCaptureDefaults -v`
Expected: FAIL — `undefined: CaptureDefaults`

- [ ] **Step 3: Implement `CaptureDefaults`**

Append to `internal/gcp/ambient.go`:

```go
// CaptureDefaults returns the ambient gcloud configuration's name, project and
// region — the configuration Ambient() resolves. It backs flag-less
// `azrl gcp capture`, so adopting the ambient identity binds the profile to
// the live configuration instead of an empty, not-yet-created one.
// Best-effort and disk-only: missing or unparseable state yields zero values.
func CaptureDefaults() Conf {
	dir, _, ok := provider.EnvOrHome("CLOUDSDK_CONFIG", ".config", "gcloud")
	if !ok {
		return Conf{}
	}
	name := os.Getenv("CLOUDSDK_ACTIVE_CONFIG_NAME")
	if name == "" {
		b, err := os.ReadFile(filepath.Join(dir, "active_config"))
		if err != nil {
			return Conf{}
		}
		name = strings.TrimSpace(string(b))
	}
	if name == "" {
		return Conf{}
	}
	b, err := os.ReadFile(filepath.Join(dir, "configurations", "config_"+name))
	if err != nil {
		return Conf{}
	}
	return Conf{
		ConfigName: name,
		Project:    iniValue(string(b), "core", "project"),
		Region:     iniValue(string(b), "compute", "region"),
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/gcp/ -run TestCaptureDefaults -v`
Expected: PASS (both)

- [ ] **Step 5: Wire into the capture command**

In `cmd/gcp.go`, `newGcpCaptureCmd` RunE, replace:

```go
			cn := configName
			if cn == "" {
				cn = name
			}
			conf := gcp.Conf{
				ConfigName: cn, Project: project, Region: region, ExpectAccount: expectAccount,
			}
```

with:

```go
			def := gcp.CaptureDefaults()
			cn := configName
			if cn == "" {
				cn = def.ConfigName
			}
			if cn == "" {
				cn = name
			}
			conf := gcp.Conf{
				ConfigName: cn, Project: project, Region: region, ExpectAccount: expectAccount,
			}
			if conf.Project == "" {
				conf.Project = def.Project
			}
			if conf.Region == "" {
				conf.Region = def.Region
			}
```

- [ ] **Step 6: Full gates**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: build OK, all tests PASS, no gofmt output.

- [ ] **Step 7: Commit**

```bash
git add internal/gcp/ambient.go internal/gcp/ambient_test.go cmd/gcp.go
git commit -m "feat(gcp): capture derives project and config from the ambient configuration"
```

---

### Task 3: GitHub capture falls back to the ambient gh session

**Files:**
- Modify: `internal/github/assert.go:12-27` (refactor `WhoAmI`, add `AmbientWhoAmI`)
- Modify: `cmd/gh.go` (`newGhCaptureCmd` RunE, lines 180-206)
- Test: `internal/github/assert_test.go` (append)

**Interfaces:**
- Produces: `github.AmbientWhoAmI(host string) (string, error)` — same JSON-login result as `WhoAmI`, but against gh's own ambient config (no `GH_CONFIG_DIR` override).
- Consumes: nothing new.

**Why:** `WhoAmI(profilesDir, name, host)` sets `GH_CONFIG_DIR` to the profile's *isolated* dir (assert.go:13-15). Adopting the ambient identity targets a profile whose isolated dir has no session yet, so capture always failed for a fresh adopt.

- [ ] **Step 1: Write the failing test**

Append to `internal/github/assert_test.go` (mirror the existing PATH-shim pattern in that file — a fake `gh` script logging its env and printing JSON):

```go
func TestAmbientWhoAmIDoesNotOverrideConfigDir(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "env.log")
	script := "#!/bin/sh\nenv | grep '^GH_CONFIG_DIR=' > " + log + "\necho '{\"login\":\"octocat\"}'\n"
	os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755)
	t.Setenv("PATH", dir)
	t.Setenv("GH_CONFIG_DIR", "")

	login, err := AmbientWhoAmI("github.com")
	if err != nil || login != "octocat" {
		t.Fatalf("AmbientWhoAmI = %q, %v", login, err)
	}
	b, _ := os.ReadFile(log)
	if strings.Contains(string(b), "GH_CONFIG_DIR=/") {
		t.Fatalf("ambient whoami must not point GH_CONFIG_DIR at an isolated dir: %s", b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/github/ -run TestAmbientWhoAmI -v`
Expected: FAIL — `undefined: AmbientWhoAmI`

- [ ] **Step 3: Refactor `WhoAmI` and add `AmbientWhoAmI`**

In `internal/github/assert.go`, replace the existing `WhoAmI` (lines 12-27) with:

```go
func WhoAmI(profilesDir, name, host string) (string, error) {
	login, err := whoAmI(host, ConfigDir(profilesDir, name))
	if err != nil {
		return "", fmt.Errorf("ghrl: gh api user failed for %q: %w", name, err)
	}
	return login, nil
}

// AmbientWhoAmI returns the login gh's own ambient config is signed in as —
// the capture fallback when the profile's isolated GH_CONFIG_DIR has no
// session yet (adopting the native default identity).
func AmbientWhoAmI(host string) (string, error) {
	login, err := whoAmI(host, "")
	if err != nil {
		return "", fmt.Errorf("ghrl: gh api user failed for the ambient session: %w", err)
	}
	return login, nil
}

func whoAmI(host, configDir string) (string, error) {
	cmd := exec.Command("gh", "api", "user", "--hostname", host)
	cmd.Env = os.Environ()
	if configDir != "" {
		cmd.Env = append(cmd.Env, "GH_CONFIG_DIR="+configDir)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(out, &u); err != nil {
		return "", fmt.Errorf("could not parse gh api user: %w", err)
	}
	return u.Login, nil
}
```

Note: preserve the exact original error prefix `ghrl: gh api user failed for %q` — existing tests may assert it.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/github/ -v`
Expected: PASS (new test and all pre-existing github tests — the refactor must not change `WhoAmI` behavior).

- [ ] **Step 5: Wire the fallback into gh capture**

In `cmd/gh.go`, `newGhCaptureCmd` RunE, replace:

```go
			login, err := github.WhoAmI(dir, name, hostname)
			if err != nil {
				return err
			}
```

with:

```go
			login, err := github.WhoAmI(dir, name, hostname)
			if err != nil {
				// Fresh adopt: the isolated dir has no session yet — record the
				// ambient identity instead (capture is metadata-only; sign-in
				// into the isolated dir happens later via `s`).
				login, err = github.AmbientWhoAmI(hostname)
			}
			if err != nil {
				return err
			}
```

- [ ] **Step 6: Full gates**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: build OK, all tests PASS, no gofmt output.

- [ ] **Step 7: Commit**

```bash
git add internal/github/assert.go internal/github/assert_test.go cmd/gh.go
git commit -m "feat(github): capture falls back to the ambient gh session"
```

---

### Task 4: Dashboard adopt prompts for a name

**Files:**
- Modify: `internal/ui/dashboard.go` (model struct ~line 40, `overviewItems` ~line 88, `adoptArgs` ~line 169, `Update` ~line 181, `View` ~line 238)
- Test: `internal/ui/dashboard_test.go` (replace `TestDashboardAdoptKeyDispatch` line 170 and `TestAdoptArgsDefaultToDirName` line 187; add prompt tests)

**Interfaces:**
- Produces: `captureArgs(providerName, name string) []string` (replaces `adoptArgs(providerName, dir string)`); `dashItem` gains `adopt bool` (adoptDir keeps its meaning: prefill dir, `""` ⇒ cwd at prompt time). Task 5 relies on both.
- Consumes: `runHandoff(args []string) tea.Cmd`, `groupArgs`, `profile.DefaultName(arg, dir string) string`, `profile.SanitizeName`, `keyHelp` — all existing.

- [ ] **Step 1: Write the failing tests**

In `internal/ui/dashboard_test.go`, replace `TestDashboardAdoptKeyDispatch` (lines 170-185) and `TestAdoptArgsDefaultToDirName` (lines 187-201) with:

```go
func TestDashboardAdoptOpensNamePrompt(t *testing.T) {
	// [a] on an adoptable row opens the name prompt prefilled from the row's
	// dir; enter hands off to capture; any other row ignores the key.
	m := dashboardModel{width: 100, items: []dashItem{
		{provider: "github", adopt: true, adoptDir: "/home/u/oss/foo"},
		{provider: "azure", profile: "acme"},
	}}
	mod, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = mod.(dashboardModel)
	if cmd != nil {
		t.Fatal("[a] should open the prompt, not exec immediately")
	}
	if !m.naming || m.nameInput.Placeholder != "foo" {
		t.Fatalf("naming=%v placeholder=%q, want prompt prefilled with dir basename", m.naming, m.nameInput.Placeholder)
	}
	if v := m.View(); !strings.Contains(v, "Name for the adopted profile:") {
		t.Fatalf("prompt missing from view:\n%s", v)
	}
	// Enter with the empty input falls back to the placeholder and execs.
	mod, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mod.(dashboardModel)
	if cmd == nil || m.naming {
		t.Fatalf("enter should exec the capture handoff and close the prompt (cmd=%v naming=%v)", cmd, m.naming)
	}
}

func TestDashboardAdoptPromptEscCancels(t *testing.T) {
	m := dashboardModel{width: 100, items: []dashItem{{provider: "github", adopt: true, adoptDir: "/w/foo"}}}
	mod, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = mod.(dashboardModel)
	mod, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mod.(dashboardModel)
	if m.naming || cmd != nil {
		t.Fatal("esc should close the prompt without running anything")
	}
}

func TestDashboardAdoptIgnoredOnManagedRow(t *testing.T) {
	m := dashboardModel{width: 100, items: []dashItem{{provider: "azure", profile: "acme"}}}
	mod, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = mod.(dashboardModel)
	if cmd != nil || m.naming {
		t.Fatal("[a] on a managed row should be a no-op")
	}
}

func TestCaptureArgsPerProvider(t *testing.T) {
	cases := []struct {
		provider string
		want     []string
	}{
		{"azure", []string{"capture", "foo"}},
		{"github", []string{"gh", "capture", "foo"}},
		{"aws", []string{"aws", "capture", "foo"}},
		{"gcp", []string{"gcp", "capture", "foo"}},
	}
	for _, c := range cases {
		got := captureArgs(c.provider, "foo")
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Fatalf("captureArgs(%s) = %v, want %v", c.provider, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestDashboardAdopt|TestCaptureArgs' -v`
Expected: FAIL — `unknown field adopt`, `undefined: captureArgs`, `m.naming undefined`

- [ ] **Step 3: Implement**

In `internal/ui/dashboard.go`:

**(a)** Add the import `"github.com/charmbracelet/bubbles/textinput"`.

**(b)** Extend `dashItem`:

```go
type dashItem struct {
	provider string
	profile  string
	adopt    bool   // [a] opens the adopt name prompt for this row
	adoptDir string // prefill dir for the prompt; "" = cwd at prompt time
}
```

**(c)** Extend `dashboardModel` with three fields:

```go
	naming    bool
	nameInput textinput.Model
	adoptItem dashItem
```

**(d)** In `overviewItems`, replace `it.adoptDir = r.Dir` with:

```go
		if r.Unmanaged != "" {
			it.adopt, it.adoptDir = true, r.Dir
		}
```

**(e)** Replace `adoptArgs` (and its doc comment) with:

```go
// captureArgs maps a provider to the azrl subcommand that captures the
// current session into profile <name> (Azure's capture is top-level; the
// other providers sit under their command group).
func captureArgs(providerName, name string) []string {
	switch providerName {
	case "azure":
		return []string{"capture", name}
	case "github":
		return groupArgs("gh", "capture", name)
	default:
		return []string{providerName, "capture", name}
	}
}
```

**(f)** In `Update`, at the top of the `case tea.KeyMsg:` branch (before the existing `switch msg.String()`):

```go
		if m.naming {
			switch msg.String() {
			case "esc":
				m.naming = false
			case "enter":
				name := strings.TrimSpace(m.nameInput.Value())
				if name == "" {
					name = m.nameInput.Placeholder
				}
				name = profile.SanitizeName(name)
				if name == "" {
					return m, nil
				}
				m.naming = false
				return m, runHandoff(captureArgs(m.adoptItem.provider, name))
			default:
				var cmd tea.Cmd
				m.nameInput, cmd = m.nameInput.Update(msg)
				_ = cmd
			}
			return m, nil
		}
```

**(g)** Replace the `case "a":` block with:

```go
		case "a":
			if m.cursor >= 0 && m.cursor < len(m.items) {
				if it := m.items[m.cursor]; it.adopt {
					dir := it.adoptDir
					if dir == "" {
						dir, _ = os.Getwd()
					}
					ti := textinput.New()
					ti.Placeholder = profile.DefaultName("", dir)
					ti.Focus()
					m.nameInput = ti
					m.adoptItem = it
					m.naming = true
				}
			}
			return m, nil
```

**(h)** In `View`, right after `short, notice := dashboardHints(m.ov)`:

```go
	if m.naming {
		short = accentStyle.Render("adopt " + m.adoptItem.provider)
		notice = ""
	}
```

and immediately after `var body []string`, prepend the prompt block:

```go
	if m.naming {
		body = append(body,
			mutedStyle.Render("Name for the adopted profile:"),
			"",
			m.nameInput.View(),
			"",
			keyHelp("↵", "adopt session + link", "esc", "cancel"),
			"")
	}
```

(the existing `if notice != "" { ... }` block stays as-is below it).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS — including all pre-existing dashboard tests.

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add internal/ui/dashboard.go internal/ui/dashboard_test.go
git commit -m "feat(ui): dashboard adopt prompts for a profile name"
```

---

### Task 5: Unmanaged ambient rows become adoptable

**Files:**
- Modify: `internal/ui/dashboard.go` (`overviewItems` ~line 88, `ambientLine` ~line 391)
- Test: `internal/ui/dashboard_test.go` (append)

**Interfaces:**
- Consumes: `dashItem.adopt` + the name prompt from Task 4 (`adoptDir == ""` ⇒ cwd prefill); `AmbientRow{Provider, Title, Identity, Source, Profile}` (overview.go:45).
- Produces: nothing new for later tasks.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/dashboard_test.go`:

```go
func TestOverviewItemsMarksUnmanagedAmbientAdoptable(t *testing.T) {
	ov := Overview{Ambient: []AmbientRow{
		{Provider: "aws", Identity: "111122223333/Dev"},           // unmanaged
		{Provider: "azure", Identity: "me@x.com", Profile: "acme"}, // managed
	}}
	items := overviewItems(ov)
	if !items[0].adopt || items[0].adoptDir != "" {
		t.Fatalf("unmanaged ambient row should be adoptable with cwd prefill: %+v", items[0])
	}
	if items[1].adopt {
		t.Fatalf("managed ambient row must not be adoptable: %+v", items[1])
	}
}

func TestAmbientLineOffersAdoptOnUnmanaged(t *testing.T) {
	line := ambientLine(AmbientRow{Provider: "aws", Title: "AWS", Identity: "1111/Dev", Source: "file:~/.aws/config"}, 10, 20, 20)
	if !strings.Contains(line, "[a]dopt") {
		t.Fatalf("unmanaged ambient line missing [a]dopt: %q", line)
	}
	managed := ambientLine(AmbientRow{Provider: "aws", Title: "AWS", Identity: "1111/Dev", Source: "s", Profile: "prod"}, 10, 20, 20)
	if strings.Contains(managed, "[a]dopt") {
		t.Fatalf("managed ambient line must not offer adopt: %q", managed)
	}
}

func TestDashboardAdoptAmbientPrefillsCwdBasename(t *testing.T) {
	m := dashboardModel{width: 100, items: []dashItem{{provider: "aws", adopt: true}}}
	mod, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = mod.(dashboardModel)
	cwd, _ := os.Getwd()
	if !m.naming || m.nameInput.Placeholder != profile.DefaultName("", cwd) {
		t.Fatalf("ambient adopt should prefill from cwd: naming=%v placeholder=%q", m.naming, m.nameInput.Placeholder)
	}
}
```

Add `"github.com/slamb2k/azrl/internal/profile"` to the test file's imports if not present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestOverviewItemsMarks|TestAmbientLineOffers|TestDashboardAdoptAmbient' -v`
Expected: FAIL — ambient items not adoptable, no `[a]dopt` in the line.

- [ ] **Step 3: Implement**

In `internal/ui/dashboard.go`:

**(a)** In `overviewItems`, replace the ambient loop with:

```go
	for _, r := range ov.Ambient {
		it := dashItem{provider: r.Provider, profile: r.Profile}
		// An unmanaged native default is adoptable; the prompt prefills from
		// the cwd (capture links the cwd, so its basename is the natural name).
		if r.Profile == "" {
			it.adopt = true
		}
		items = append(items, it)
	}
```

**(b)** In `ambientLine`, change the unmanaged tail from:

```go
	return line + accentStyle.Render("unmanaged")
```

to:

```go
	return line + accentStyle.Render("unmanaged") + mutedStyle.Render(" · [a]dopt")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS.

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add internal/ui/dashboard.go internal/ui/dashboard_test.go
git commit -m "feat(ui): unmanaged ambient rows adoptable from the dashboard"
```

---

### Task 6: Expiry markers become AWS-only guidance on dashboard rows, hints, and plain status

**Files:**
- Modify: `internal/ui/overview.go` (add `ExpiryActionable`)
- Modify: `internal/ui/dashboard.go` (`mappingLine` ~line 353, `unmappedLine` ~line 404, delete `expiryText` ~line 454, add `expiringSoon`, gate `dashboardHints` ~lines 514-536)
- Modify: `cmd/status.go` (gate the MAPPINGS `expired` note, line 129)
- Test: `internal/ui/dashboard_test.go` (update 3 tests, add 2), `cmd/status_test.go` (update `TestStatusCmdMappingExpiry` line 199)

**Interfaces:**
- Produces: `ui.ExpiryActionable(provider string) bool` (exported — `cmd/status.go` and Tasks 7/8 use it); `expiringSoon(exp *time.Time) bool` (package-private, Task 7 uses it).
- Consumes: `expired(exp *time.Time) bool`, `shortDur`, `accentStyle`/`failureStyle` — all existing.

- [ ] **Step 1: Update/write the failing tests**

In `internal/ui/dashboard_test.go`:

**(a)** Replace the body of `TestDashboardMappingRowShowsExpired` (line 340) — the semantics flip: an expired *azure* mapping shows nothing; an expired *aws* mapping shows the marker:

```go
func TestDashboardMappingRowShowsExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	m := dashboardModel{width: 120, ov: Overview{
		Mappings: []MappingRow{
			// Azure's access token refreshes silently on next use — never a row marker.
			{Provider: "azure", Title: "Azure", Dir: "/work/acme", Profile: "acme",
				Source: "pointer", Scope: ScopeNone, Pointer: ".azprofile", Expiry: &past},
			// The AWS SSO session dying is real guidance.
			{Provider: "aws", Title: "AWS", Dir: "/work/api", Profile: "prod",
				Source: "pointer", Scope: ScopeNone, Pointer: ".awsprofile", Expiry: &past},
		},
	}}
	m.items = overviewItems(m.ov)
	v := m.View()
	if strings.Count(v, "⚠ expired") != 1 {
		t.Fatalf("expected exactly one expired marker (aws only):\n%s", v)
	}
}
```

**(b)** In `TestDashboardHintExpiredGoverningPin` (line 362), change the expired mapping's provider from `"azure"` to `"aws"` (and the two assertions from `azure:acme` to `aws:acme`).

**(c)** In `TestDashboardDriftAndExpiryRendering` (line 269) and `TestDashboardHintIgnoresExpiredNonGoverningMapping` (line 383): wherever a row's `Expiry` is expected to produce visible output, set that row's `Provider` to `"aws"`; leave rows asserting absence on azure. (Read each test and adjust minimally — expected-output strings stay otherwise identical.)

**(d)** Append new tests:

```go
func TestMappingRowExpiringSoonAmberAwsOnly(t *testing.T) {
	soon := time.Now().Add(10 * time.Minute)
	awsLine := mappingLine(MappingRow{Provider: "aws", Dir: "/w/a", Profile: "prod",
		Source: "pointer", Scope: ScopeCwd, Expiry: &soon}, 20, 20)
	if !strings.Contains(awsLine, "⚠ expires in") {
		t.Fatalf("aws row expiring soon missing amber marker: %q", awsLine)
	}
	azLine := mappingLine(MappingRow{Provider: "azure", Dir: "/w/a", Profile: "acme",
		Source: "pointer", Scope: ScopeCwd, Expiry: &soon}, 20, 20)
	if strings.Contains(azLine, "expires") || strings.Contains(azLine, "expired") {
		t.Fatalf("azure row must never show expiry: %q", azLine)
	}
	farAws := time.Now().Add(3 * time.Hour)
	quiet := mappingLine(MappingRow{Provider: "aws", Dir: "/w/a", Profile: "prod",
		Source: "pointer", Scope: ScopeCwd, Expiry: &farAws}, 20, 20)
	if strings.Contains(quiet, "expires") {
		t.Fatalf("healthy aws row must show nothing: %q", quiet)
	}
}

func TestUnmappedRowShowsNoExpiry(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	line := unmappedLine(UnmappedRow{Provider: "aws",
		Status: provider.Status{ProfileName: "stale", Identity: "1111/Dev", Expiry: &past}})
	if strings.Contains(line, "expired") || strings.Contains(line, "in ") {
		t.Fatalf("unmapped rows are not in play — no expiry display: %q", line)
	}
}

func TestExpiredHintGatedToAws(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	ov := Overview{Mappings: []MappingRow{
		{Provider: "azure", Dir: "/w/x", Profile: "acme", Scope: ScopeCwd, Expiry: &past},
	}}
	if short, _ := dashboardHints(ov); strings.Contains(short, "expired") {
		t.Fatalf("azure expiry must not drive the hint: %q", short)
	}
	ov.Unmapped = []UnmappedRow{{Provider: "gcp",
		Status: provider.Status{ProfileName: "old", Expiry: &past}}}
	if short, _ := dashboardHints(ov); strings.Contains(short, "expired") {
		t.Fatalf("gcp unmapped expiry must not drive the hint: %q", short)
	}
}
```

In `cmd/status_test.go`, `TestStatusCmdMappingExpiry` (line 199): change the plain-output assertion so azure no longer carries the note but the JSON expiry survives:

```go
	statusJSON = false
	out := runRoot(t, "status")
	if !strings.Contains(out, "azure:acme") {
		t.Fatalf("mapping row missing:\n%s", out)
	}
	if strings.Contains(out, "expired") {
		t.Fatalf("azure mapping must not carry the expired note (AWS-only guidance):\n%s", out)
	}
```

(keep the JSON half of the test unchanged — raw expiry stays in `--json`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ ./cmd/ -run 'Expir|expired|TestDashboard' -v 2>&1 | tail -30`
Expected: FAIL — markers still render for azure, `expiringSoon` undefined, hints fire for azure/gcp.

- [ ] **Step 3: Implement**

**(a)** In `internal/ui/overview.go`, after the `Overview` struct:

```go
// ExpiryActionable reports whether a provider's tracked expiry is guidance
// the user must act on. Only AWS qualifies: its SSO session genuinely dies
// and `aws sso login` is required. Azure and GCP track the access token,
// which az/gcloud refresh silently on next use, and GitHub has no expiry —
// their rows never show expiry (the DETAILS pane tells the truth on demand).
func ExpiryActionable(provider string) bool { return provider == "aws" }
```

**(b)** In `internal/ui/dashboard.go`, next to `expired` (~line 466):

```go
// expiringSoon reports whether a live expiry is inside the amber window —
// close enough that the next longish task would hit the wall.
func expiringSoon(exp *time.Time) bool {
	return exp != nil && !expired(exp) && time.Until(*exp) < 15*time.Minute
}
```

**(c)** In `mappingLine`, replace:

```go
	if expired(r.Expiry) {
		line += "  " + failureStyle.Render("⚠ expired")
	}
```

with:

```go
	if ExpiryActionable(r.Provider) {
		switch {
		case expired(r.Expiry):
			line += "  " + failureStyle.Render("⚠ expired")
		case expiringSoon(r.Expiry):
			line += "  " + accentStyle.Render("⚠ expires in "+shortDur(time.Until(*r.Expiry)))
		}
	}
```

**(d)** In `unmappedLine`, drop the expiry tail — replace the return with:

```go
	return lipgloss.NewStyle().Foreground(grayDeep).Render("●") + " " +
		mutedStyle.Render(r.Provider+":"+st.ProfileName+" · "+orDash(st.Identity))
```

**(e)** Delete the now-unused `expiryText` function (keep `shortDur` — the amber marker uses it).

**(f)** In `dashboardHints`, gate the two expired loops:

```go
	for _, r := range ov.Mappings {
		if ExpiryActionable(r.Provider) && r.Scope != ScopeNone && expired(r.Expiry) {
```

and

```go
	for _, u := range ov.Unmapped {
		if ExpiryActionable(u.Provider) && expired(u.Status.Expiry) {
```

**(g)** In `cmd/status.go` line 129, gate the note:

```go
		if ui.ExpiryActionable(m.Provider) && m.Expiry != nil && time.Until(*m.Expiry) <= 0 {
			note += "  expired"
		}
```

(`cmd/status.go` already imports the `ui` package for `BuildOverview`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ ./cmd/ -v 2>&1 | tail -15`
Expected: PASS. If any other pre-existing test asserted the old azure/gcp expiry rendering, update it to the AWS-gated semantics (same minimal provider flip as Step 1c).

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add internal/ui/overview.go internal/ui/dashboard.go internal/ui/dashboard_test.go cmd/status.go cmd/status_test.go
git commit -m "feat(ui): expiry markers become AWS-only guidance with a 15m amber window"
```

---

### Task 7: Ambient rows carry expiry (AWS in-play tags)

**Files:**
- Modify: `internal/ui/overview.go` (`AmbientRow` struct line 45, `BuildOverview` ambient block ~line 110)
- Modify: `internal/ui/dashboard.go` (`ambientLine` managed branch)
- Test: `internal/ui/dashboard_test.go` (append), `internal/ui/overview_test.go` (append)

**Interfaces:**
- Produces: `AmbientRow.Expiry *time.Time` — populated only when the ambient identity matches a managed profile.
- Consumes: `ExpiryActionable`, `expiringSoon`, `expired`, `shortDur` (Task 6); `statuses map[string]provider.Status` already in scope in `BuildOverview`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/dashboard_test.go`:

```go
func TestAmbientLineExpiryTagsAwsOnly(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	soon := time.Now().Add(5 * time.Minute)
	expiredAws := ambientLine(AmbientRow{Provider: "aws", Title: "AWS", Identity: "1/Dev",
		Source: "s", Profile: "prod", Expiry: &past}, 10, 20, 20)
	if !strings.Contains(expiredAws, "⚠ expired") {
		t.Fatalf("expired aws default missing tag: %q", expiredAws)
	}
	soonAws := ambientLine(AmbientRow{Provider: "aws", Title: "AWS", Identity: "1/Dev",
		Source: "s", Profile: "prod", Expiry: &soon}, 10, 20, 20)
	if !strings.Contains(soonAws, "⚠ expires in") {
		t.Fatalf("soon-expiring aws default missing amber tag: %q", soonAws)
	}
	azure := ambientLine(AmbientRow{Provider: "azure", Title: "Azure", Identity: "me@x",
		Source: "s", Profile: "acme", Expiry: &past}, 10, 20, 20)
	if strings.Contains(azure, "expire") {
		t.Fatalf("azure default must never show expiry: %q", azure)
	}
}
```

Append to `internal/ui/overview_test.go`. Background: `seedDashHome` (dashboard_test.go:27) seeds profile `acme` whose isolated `azureProfile.json` signs in `u@acme.com`, but seeds no *ambient* azure state (`~/.azure`), so no azure ambient row exists by default. Azure's `Ambient()` reads `${AZURE_CONFIG_DIR:-~/.azure}/azureProfile.json` and `provider.MatchProfile` matches on the exact identity string — seed the ambient file with the same user and the match is deterministic:

```go
func TestBuildOverviewAmbientCarriesMatchedProfileExpiry(t *testing.T) {
	seedDashHome(t)
	home := os.Getenv("HOME")
	// The ambient az default (~/.azure) signed in as the same user as the
	// saved acme profile, whose MSAL cache carries a long-past expiry.
	os.MkdirAll(filepath.Join(home, ".azure"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure", "azureProfile.json"),
		[]byte(`{"subscriptions":[{"user":{"name":"u@acme.com"},"isDefault":true,"tenantId":"g1"}]}`), 0o644)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme", "msal_token_cache.json"),
		[]byte(`{"AccessToken":{"k":{"expires_on":"1000000"}}}`), 0o644)

	ov := BuildOverview(provider.All(), home)
	for _, a := range ov.Ambient {
		if a.Provider == "azure" {
			if a.Profile != "acme" {
				t.Fatalf("ambient azure default should match acme, got %q", a.Profile)
			}
			if a.Expiry == nil {
				t.Fatal("managed ambient row should carry the matched profile's expiry")
			}
			return
		}
	}
	t.Fatal("no azure ambient row produced — check the ~/.azure seeding")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestAmbientLineExpiry|TestBuildOverviewAmbientCarries' -v`
Expected: FAIL — `unknown field Expiry in AmbientRow`.

- [ ] **Step 3: Implement**

**(a)** In `internal/ui/overview.go`, extend `AmbientRow`:

```go
type AmbientRow struct {
	Provider string
	Title    string
	Identity string
	Source   string
	Profile  string
	Expiry   *time.Time // the matched managed profile's expiry; nil = unmanaged/none
}
```

**(b)** In `BuildOverview`, replace the ambient block with:

```go
		if amb, err := p.Ambient(); err == nil && amb.Identity != "" {
			row := AmbientRow{
				Provider: p.Name(), Title: p.Title(), Identity: amb.Identity,
				Source: amb.Source, Profile: provider.MatchProfile(listed, amb.Identity),
			}
			if st, ok := statuses[row.Profile]; ok {
				row.Expiry = st.Expiry
			}
			ov.Ambient = append(ov.Ambient, row)
		}
```

**(c)** In `internal/ui/dashboard.go` `ambientLine`, replace the managed branch:

```go
	if r.Profile != "" {
		// The default isn't associated with any folder, so no profile/dir
		// target — just whether azrl manages this identity.
		line += successStyle.Render("managed")
		if ExpiryActionable(r.Provider) {
			switch {
			case expired(r.Expiry):
				line += "  " + failureStyle.Render("⚠ expired")
			case expiringSoon(r.Expiry):
				line += "  " + accentStyle.Render("⚠ expires in "+shortDur(time.Until(*r.Expiry)))
			}
		}
		return line
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS.

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add internal/ui/overview.go internal/ui/overview_test.go internal/ui/dashboard.go internal/ui/dashboard_test.go
git commit -m "feat(ui): ambient default rows carry AWS expiry tags"
```

---

### Task 8: DETAILS pane tells the per-provider expiry truth

**Files:**
- Modify: `internal/ui/panes.go` (`profileInfoBlock` line 82, `expiryWord` line 104)
- Modify: `internal/ui/provider_view.go:630` (the single `profileInfoBlock` caller)
- Test: `internal/ui/panes_test.go` (create)

**Interfaces:**
- Produces: `expiryWord(prov string, t *time.Time) string`; `profileInfoBlock(prov string, pr profile.Listed, st provider.Status, browser, linked, driftNote string, w int) string`.
- Consumes: `ExpiryActionable` (Task 6).

- [ ] **Step 1: Write the failing test**

Create `internal/ui/panes_test.go`:

```go
package ui

import (
	"strings"
	"testing"
	"time"
)

func TestExpiryWordPerProvider(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	live := time.Now().Add(30 * time.Minute)

	// Azure/GCP: a stale access token is refreshed silently on next use.
	for _, prov := range []string{"azure", "gcp"} {
		if w := expiryWord(prov, &past); !strings.Contains(w, "token stale · refreshes on next use") {
			t.Fatalf("%s stale word = %q", prov, w)
		}
		if w := expiryWord(prov, &live); !strings.Contains(w, "left") {
			t.Fatalf("%s live expiry should still show the countdown: %q", prov, w)
		}
	}
	// AWS: expired means sign in again.
	if w := expiryWord("aws", &past); !strings.Contains(w, "expired") {
		t.Fatalf("aws expired word = %q", w)
	}
	// GitHub / none tracked: empty.
	if w := expiryWord("github", nil); w != "" {
		t.Fatalf("nil expiry should render empty, got %q", w)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestExpiryWordPerProvider -v`
Expected: FAIL — `too many arguments in call to expiryWord`.

- [ ] **Step 3: Implement**

**(a)** In `internal/ui/panes.go`, replace `expiryWord`:

```go
// expiryWord renders the DETAILS expiry truthfully per provider: AWS expiry
// is actionable ("expired" = sign in again), while a stale Azure/GCP access
// token is refreshed silently by az/gcloud on next use — say so instead of
// crying wolf. nil (GitHub / nothing tracked) renders empty.
func expiryWord(prov string, t *time.Time) string {
	if t == nil {
		return ""
	}
	d := time.Until(*t)
	if d <= 0 {
		if !ExpiryActionable(prov) {
			return mutedStyle.Render("token stale · refreshes on next use")
		}
		return failureStyle.Render("expired")
	}
	if d < 2*time.Hour {
		return accentStyle.Render(d.Round(time.Minute).String() + " left")
	}
	return d.Round(time.Hour).String() + " left"
}
```

**(b)** Change `profileInfoBlock`'s signature to lead with the provider name and pass it through:

```go
func profileInfoBlock(prov string, pr profile.Listed, st provider.Status, browser, linked, driftNote string, w int) string {
```

and inside, the Expiry row becomes:

```go
		row("Expiry", expiryWord(prov, st.Expiry)),
```

**(c)** In `internal/ui/provider_view.go:630`, update the caller:

```go
		info = profileInfoBlock(v.prov.Name(), pr, st, browser, linked, note, rightW)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS (if any existing view test asserted the old red "expired" in an azure/gcp DETAILS pane, update it to expect `token stale · refreshes on next use`).

- [ ] **Step 5: Full gates + commit**

Run: `go build ./... && go test ./... && gofmt -l .`
Expected: clean.

```bash
git add internal/ui/panes.go internal/ui/panes_test.go internal/ui/provider_view.go
git commit -m "feat(ui): DETAILS expiry tells the per-provider truth"
```

---

### Task 9: Docs + final whole-branch verification

**Files:**
- Modify: `CLAUDE.md` (the `internal/ui/` bullet and the `cmd/` bullet in Architecture)

- [ ] **Step 1: Update CLAUDE.md**

In the **`cmd/`** bullet, after the sentence describing the four capture verbs (search for "an `aws` group"), append one sentence:

```
`capture` derives missing fields from the ambient session when flags are
omitted (azure: `az account show`; github: falls back to the ambient gh
session when the isolated dir has no session; aws: SSO fields from the
ambient config stanza incl. `sso-session` indirection; gcp: binds the
ambient active configuration's name/project/region).
```

In the **`internal/ui/`** bullet, find the dashboard sentence ("The dashboard (`overview.go`'s `BuildOverview`, shared with `cmd status`) is live via fsnotify…") and extend it:

```
Adoptable rows (unmanaged GitHub git-config mappings and unmanaged AMBIENT
defaults on any provider) take `a`, which prompts for a name (prefilled
`profile.DefaultName`) before exec-ing the provider's `capture`. Expiry is
guidance, not telemetry: only AWS rows show markers (amber `⚠ expires in …`
inside 15 min, `⚠ expired` after; MAPPINGS + managed AMBIENT rows only, and
the expired dashboard hint + plain-status note are AWS-gated via
`ui.ExpiryActionable`); Azure/GCP rows never show expiry — the DETAILS pane
says `token stale · refreshes on next use` (their CLIs refresh silently);
GitHub shows nothing. UNMAPPED rows carry no expiry.
```

Keep both edits in the flow/voice of the surrounding text (they are dense single-paragraph bullets — splice, don't bullet-point).

- [ ] **Step 2: Whole-branch verification**

```bash
go build ./... && go test ./... && gofmt -l .
git diff main --stat
grep -rn "expiryText" internal/ cmd/        # expect: no hits
grep -rn "adoptArgs" internal/ cmd/         # expect: no hits
```

Expected: build OK, full suite PASS, gofmt clean, both greps empty.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: dashboard adopt-from-ambient + per-provider expiry semantics"
```

---

## Post-plan checklist (for the executor)

- Surface the six **Recorded assumptions** to the user with the final report.
- Real-machine items (actual `aws sso` ambient adopt, gcloud active-config adopt) belong on the manual-verify checklist if the user wants them tracked — flag it, don't add unasked.
- Ship via `/ship` (or superpowers:finishing-a-development-branch) once the user approves.
