# Per-Profile Browser Command Override — Implementation Plan (Part 1)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An optional per-profile conf key (`AZ_BROWSER_CMD` / `GH_BROWSER_CMD` / `AWS_BROWSER_CMD` / `GCP_BROWSER_CMD`) that overrides the global `LOCAL_BROWSER_CMD` for that profile's logins.

**Architecture:** One env hook (`AZRL_BROWSER_CMD`) inside `config.LoadGlobal` is the single override point; every login path already funnels through a `config.Global`. Azure overrides the struct field directly (it holds both `Global` and `Conf`); AWS/GCP setenv before their `Login` (which call `LoadGlobal` internally); GitHub passes the env var to the `gh` process, which propagates it to the `azrl __browser` shim. No signature changes; `internal/bridge`, `internal/browsercapture`, `cmd/browser.go` untouched.

**Tech Stack:** Go stdlib only. Spec: `docs/superpowers/specs/2026-07-05-browser-profile-mapping-design.md` (Part 1).

## Global Constraints

- New keys are OPTIONAL and additive: unset key ⇒ behaviour byte-identical to today. Empty values serialize as `KEY=` (existing AZ_LABEL style).
- Exact key names: `AZ_BROWSER_CMD`, `GH_BROWSER_CMD`, `AWS_BROWSER_CMD`, `GCP_BROWSER_CMD`; env var: `AZRL_BROWSER_CMD`.
- No new dependencies. `gofmt -l .` clean. Conventional commits with scope.
- Test isolation: any test file whose package touches login flows must neutralize a leaked `AZRL_BROWSER_CMD` via `t.Setenv("AZRL_BROWSER_CMD", "")` in its seed helper (production code uses `os.Setenv`, which persists across tests in one binary).

## Setup (controller, before Task 1)

```bash
cd /home/slamb2k/work/azrl && git checkout -b feat/browser-cmd-override
# local main carries the unpushed spec commit ea759b1; the branch inherits it
# (it ships in this PR). After the PR merges, hard-sync local main to origin.
```

---

### Task 1: `AZRL_BROWSER_CMD` env hook in `config.LoadGlobal`

**Files:**
- Modify: `internal/config/config.go:87-105` (LoadGlobal)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `LoadGlobal` honours env `AZRL_BROWSER_CMD` as an override of `Global.LocalBrowserCmd` (applied after validation). Tasks 3-5 rely on exactly this behaviour.

- [ ] **Step 1: Write the failing test** — append to `internal/config/config_test.go`:

```go
func TestLoadGlobalBrowserCmdEnvOverride(t *testing.T) {
	dir := t.TempDir()
	conf := "LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm\n"
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AZRL_BROWSER_CMD", "chrome-work")
	g, err := LoadGlobal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.LocalBrowserCmd != "chrome-work" {
		t.Fatalf("env override not applied: %+v", g)
	}
}
```

Also add `t.Setenv("AZRL_BROWSER_CMD", "")` as the first line of the existing `TestLoadGlobal` (config_test.go:21) so an inherited env can never flip its result.

- [ ] **Step 2: Run** `go test ./internal/config/ -run TestLoadGlobal -v` — expect `TestLoadGlobalBrowserCmdEnvOverride` FAIL (`got wslview`).

- [ ] **Step 3: Implement** — in `LoadGlobal` (internal/config/config.go), between the validation `if` and `return g, nil`:

```go
	// AZRL_BROWSER_CMD overrides the browser command for this process only:
	// set per-profile by the cmd layer, or exported by the user as an escape
	// hatch. Applied after validation so azrl.conf must still be complete.
	if v := os.Getenv("AZRL_BROWSER_CMD"); v != "" {
		g.LocalBrowserCmd = v
	}
	return g, nil
```

Update the LoadGlobal doc comment to: `// LoadGlobal reads <dir>/azrl.conf and validates all three fields are present. The AZRL_BROWSER_CMD env var, when set, overrides LocalBrowserCmd.`

- [ ] **Step 4: Run** `go test ./internal/config/ -v` — expect all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): AZRL_BROWSER_CMD env override for the browser command"
```

---

### Task 2: Azure — `AZ_BROWSER_CMD` conf key + login wiring

**Files:**
- Modify: `internal/profile/conf.go:33-43` (struct), `:87` (LoadConf), `:112` (Write)
- Modify: `cmd/login.go` (after the `profile.LoadConf` error block, ~line 79)
- Test: `internal/profile/conf_test.go`, `cmd/login_test.go`

**Interfaces:**
- Consumes: nothing new (Task 1's hook is NOT used here — azure overrides the field directly).
- Produces: `profile.Conf.BrowserCmd string` read from/written as `AZ_BROWSER_CMD`.

- [ ] **Step 1: Write the failing round-trip test** — in `internal/profile/conf_test.go`, inside `TestBuildAndWriteConf` (line 43), after `c := BuildConf(acct, doms)` add:

```go
	c.BrowserCmd = `google-chrome --profile-directory="Profile 2"`
```

and extend the final assertion:

```go
	if rd.Tenant != "onenrg.onmicrosoft.com" || rd.ExpectUser != "u@onenrg.onmicrosoft.com" ||
		rd.BrowserCmd != `google-chrome --profile-directory="Profile 2"` {
		t.Fatalf("roundtrip got %+v", rd)
	}
```

- [ ] **Step 2: Run** `go test ./internal/profile/ -run TestBuildAndWriteConf -v` — expect FAIL to compile (`unknown field BrowserCmd`). That compile error is the RED.

- [ ] **Step 3: Implement the conf field** — in `internal/profile/conf.go`:

Struct (line ~37) gains:

```go
	BrowserCmd string // optional local browser command overriding the global LOCAL_BROWSER_CMD
```

LoadConf assignment (line ~87) becomes:

```go
	c = Conf{Tenant: m["AZ_TENANT"], TenantID: m["AZ_TENANT_ID"], DefaultSub: m["AZ_DEFAULT_SUB"], ExpectUser: m["AZ_EXPECT_USER"], Label: m["AZ_LABEL"], BrowserCmd: m["AZ_BROWSER_CMD"]}
```

Write body (line ~112) becomes:

```go
	body := fmt.Sprintf("AZ_TENANT=%s\nAZ_TENANT_ID=%s\nAZ_DEFAULT_SUB=%s\nAZ_EXPECT_USER=%s\nAZ_LABEL=%s\nAZ_BROWSER_CMD=%s\n",
		c.Tenant, c.TenantID, c.DefaultSub, c.ExpectUser, c.Label, c.BrowserCmd)
```

- [ ] **Step 4: Run** `go test ./internal/profile/ -v` — expect PASS.

- [ ] **Step 5: Write the failing login-path test** — append to `cmd/login_test.go`:

```go
func TestLoginProfileBrowserCmdOverridesGlobal(t *testing.T) {
	confs := map[string]string{"work": "AZ_TENANT=contoso.com\nAZ_BROWSER_CMD=chrome-work\n"}
	seedAzLoginEnv(t, confs)
	t.Setenv("AZ_ACCT", acctJSON("contoso.com", "simon"))
	chdirClean(t)

	out, err := execRoot(t, "login", "work")
	if err != nil {
		t.Fatalf("login: %v (out=%q)", err, out)
	}
	// ssh -R fails in the seed, so the bridge prints the paste line — it must
	// carry the profile's browser command, not the global wslview.
	if !strings.Contains(out, "chrome-work") {
		t.Fatalf("paste line should use the profile browser cmd:\n%s", out)
	}
	if strings.Contains(out, "wslview") {
		t.Fatalf("global browser cmd leaked into the paste line:\n%s", out)
	}
}
```

Also add `t.Setenv("AZRL_BROWSER_CMD", "")` to `seedAzLoginEnv` (cmd/login_test.go:24, next to the `AZURE_CONFIG_DIR` reset) per Global Constraints.

- [ ] **Step 6: Run** `go test ./cmd/ -run TestLoginProfileBrowserCmd -v` — expect FAIL (`wslview` in the paste line).

- [ ] **Step 7: Wire the override** — in `cmd/login.go`, immediately after the `profile.LoadConf` error-handling block (before `cfgDir := ...`):

```go
		if conf.BrowserCmd != "" {
			g.LocalBrowserCmd = conf.BrowserCmd
		}
```

- [ ] **Step 8: Run** `go test ./cmd/ -run TestLogin -v` — expect PASS (all login tests).

- [ ] **Step 9: Commit**

```bash
git add internal/profile/conf.go internal/profile/conf_test.go cmd/login.go cmd/login_test.go
git commit -m "feat(azure): AZ_BROWSER_CMD per-profile browser command"
```

---

### Task 3: AWS — `AWS_BROWSER_CMD` conf key + setenv wiring

**Files:**
- Modify: `internal/aws/conf.go:19-27` (struct), `:44-53` (LoadConf), `:67` (Write)
- Modify: `cmd/aws.go` (login RunE, before the `aws.Login(...)` call at ~line 84)
- Test: `internal/aws/conf_test.go`, `cmd/aws_test.go`

**Interfaces:**
- Consumes: Task 1's `AZRL_BROWSER_CMD` hook (aws.Login calls `config.LoadGlobal` internally).
- Produces: `aws.Conf.BrowserCmd string` read from/written as `AWS_BROWSER_CMD`.

- [ ] **Step 1: Extend the round-trip test** — in `internal/aws/conf_test.go` `TestConfWriteAndLoad` (line 10), add to the fixture struct literal:

```go
		BrowserCmd:    "chrome-work",
```

(The test compares `got != c` whole-struct, so this is the only edit.)

- [ ] **Step 2: Run** `go test ./internal/aws/ -run TestConfWriteAndLoad -v` — expect compile FAIL (`unknown field BrowserCmd`).

- [ ] **Step 3: Implement** — in `internal/aws/conf.go`: struct gains `BrowserCmd string // optional local browser command overriding the global LOCAL_BROWSER_CMD` (after `Label`); LoadConf literal gains `BrowserCmd: m["AWS_BROWSER_CMD"],` (after `Label:`); Write's format string appends `AWS_BROWSER_CMD=%s\n` at the end with `c.BrowserCmd` as the final arg:

```go
	body := fmt.Sprintf("AWS_SSO_START_URL=%s\nAWS_SSO_REGION=%s\nAWS_ACCOUNT_ID=%s\nAWS_ROLE_NAME=%s\nAWS_EXPECT_ACCOUNT=%s\nAWS_EXPECT_ARN=%s\nAWS_LABEL=%s\nAWS_ISOLATE=%s\nAWS_BROWSER_CMD=%s\n",
		c.SSOStartURL, c.SSORegion, c.AccountID, c.RoleName, c.ExpectAccount, c.ExpectARN, c.Label, isolate, c.BrowserCmd)
```

- [ ] **Step 4: Run** `go test ./internal/aws/ -v` — expect PASS.

- [ ] **Step 5: Write the failing cmd-level test** — append to `cmd/aws_test.go`:

```go
func TestAwsLoginProfileBrowserCmdOverridesGlobal(t *testing.T) {
	confBody := "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_BROWSER_CMD=chrome-work\n"
	awsLog, _ := seedAwsLoginEnv(t, confBody)
	// Replace the seed's dying ssh with one whose -R tunnel stays alive, so
	// the bridge takes path B and the browser launch lands in ssh.log.
	bin := filepath.Dir(awsLog)
	sshLog := filepath.Join(bin, "ssh.log")
	alive := "#!/usr/bin/env bash\necho \"$*\" >> \"" + sshLog + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && { sleep 2; exit 0; }; done\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(alive), 0o755)

	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	if err := runRootErr(t, "aws", "login", "work"); err != nil {
		t.Fatalf("login: %v", err)
	}
	b, _ := os.ReadFile(sshLog)
	if !strings.Contains(string(b), "chrome-work") {
		t.Fatalf("browser launch should use the profile cmd:\n%s", b)
	}
	if strings.Contains(string(b), "wslview") {
		t.Fatalf("global browser cmd leaked:\n%s", b)
	}
}
```

Also add `t.Setenv("AZRL_BROWSER_CMD", "")` to `seedAwsLoginEnv` (cmd/aws_test.go:89, next to the `HOME` setenv).

- [ ] **Step 6: Run** `go test ./cmd/ -run TestAwsLoginProfileBrowserCmd -v` — expect FAIL (`wslview` in ssh.log, no `chrome-work`).

- [ ] **Step 7: Wire the setenv** — in `cmd/aws.go` login RunE, immediately before `cmd.Printf("aws: signing in to %s ...` / the `aws.Login(...)` call:

```go
			if conf.BrowserCmd != "" {
				// aws.Login loads config.Global itself; the env hook in
				// LoadGlobal picks this up (same pattern as AZURE_CONFIG_DIR).
				os.Setenv("AZRL_BROWSER_CMD", conf.BrowserCmd)
			}
```

- [ ] **Step 8: Run** `go test ./cmd/ -run TestAws -v` — expect PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/aws/conf.go internal/aws/conf_test.go cmd/aws.go cmd/aws_test.go
git commit -m "feat(aws): AWS_BROWSER_CMD per-profile browser command"
```

---

### Task 4: GCP — `GCP_BROWSER_CMD` conf key + setenv wiring

Identical shape to Task 3, GCP names.

**Files:**
- Modify: `internal/gcp/conf.go:19-25` (struct), `:53-60` (LoadConf), `:75` (Write)
- Modify: `cmd/gcp.go` (login RunE, before the `gcp.Login(...)` call at ~line 86)
- Test: `internal/gcp/conf_test.go`, `cmd/gcp_test.go`

**Interfaces:**
- Consumes: Task 1's `AZRL_BROWSER_CMD` hook.
- Produces: `gcp.Conf.BrowserCmd string` read from/written as `GCP_BROWSER_CMD`.

- [ ] **Step 1: Extend the round-trip test** — `internal/gcp/conf_test.go` `TestConfWriteAndLoad` fixture gains `BrowserCmd: "chrome-work",` (whole-struct `got != c` covers it).

- [ ] **Step 2: Run** `go test ./internal/gcp/ -run TestConfWriteAndLoad -v` — expect compile FAIL.

- [ ] **Step 3: Implement** — `internal/gcp/conf.go`: struct gains `BrowserCmd string // optional local browser command overriding the global LOCAL_BROWSER_CMD` (after `Label`); LoadConf gains `BrowserCmd: m["GCP_BROWSER_CMD"],`; Write becomes:

```go
	body := fmt.Sprintf("GCP_CONFIG_NAME=%s\nGCP_PROJECT=%s\nGCP_REGION=%s\nGCP_EXPECT_ACCOUNT=%s\nGCP_LABEL=%s\nGCP_ISOLATE=%s\nGCP_BROWSER_CMD=%s\n",
		c.ConfigName, c.Project, c.Region, c.ExpectAccount, c.Label, isolate, c.BrowserCmd)
```

- [ ] **Step 4: Run** `go test ./internal/gcp/ -v` — expect PASS.

- [ ] **Step 5: Write the failing cmd-level test** — append to `cmd/gcp_test.go` (mirror of Task 3 Step 5; `seedGcpLoginEnv` returns `gcloudLog, confPath`, and GCP login asserts the account guardrail only when `GCP_EXPECT_ACCOUNT` is set — this fixture omits it):

```go
func TestGcpLoginProfileBrowserCmdOverridesGlobal(t *testing.T) {
	confBody := "GCP_PROJECT=acme-prod\nGCP_BROWSER_CMD=chrome-work\n"
	gcloudLog, _ := seedGcpLoginEnv(t, confBody)
	bin := filepath.Dir(gcloudLog)
	sshLog := filepath.Join(bin, "ssh.log")
	alive := "#!/usr/bin/env bash\necho \"$*\" >> \"" + sshLog + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && { sleep 2; exit 0; }; done\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(alive), 0o755)

	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	if err := runRootErr(t, "gcp", "login", "work"); err != nil {
		t.Fatalf("login: %v", err)
	}
	b, _ := os.ReadFile(sshLog)
	if !strings.Contains(string(b), "chrome-work") {
		t.Fatalf("browser launch should use the profile cmd:\n%s", b)
	}
	if strings.Contains(string(b), "wslview") {
		t.Fatalf("global browser cmd leaked:\n%s", b)
	}
}
```

Also add `t.Setenv("AZRL_BROWSER_CMD", "")` to `seedGcpLoginEnv` (cmd/gcp_test.go:93).

- [ ] **Step 6: Run** `go test ./cmd/ -run TestGcpLoginProfileBrowserCmd -v` — expect FAIL.

- [ ] **Step 7: Wire the setenv** — in `cmd/gcp.go` login RunE, immediately before the `gcp.Login(...)` call:

```go
			if conf.BrowserCmd != "" {
				os.Setenv("AZRL_BROWSER_CMD", conf.BrowserCmd)
			}
```

- [ ] **Step 8: Run** `go test ./cmd/ -run TestGcp -v` — expect PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/gcp/conf.go internal/gcp/conf_test.go cmd/gcp.go cmd/gcp_test.go
git commit -m "feat(gcp): GCP_BROWSER_CMD per-profile browser command"
```

---

### Task 5: GitHub — `GH_BROWSER_CMD` conf key, gh env plumbing, shim env test

**Files:**
- Modify: `internal/github/conf.go:15-20` (struct), `:36` (LoadConf), `:55` (Write)
- Modify: `internal/github/login.go:43-47` (gh env)
- Test: `internal/github/conf_test.go`, `internal/github/login_test.go`, `cmd/browser_test.go`

**Interfaces:**
- Consumes: Task 1's hook (the `__browser` shim's `config.LoadGlobal` applies the env var — `cmd/browser.go` needs NO edit).
- Produces: `github.Conf.BrowserCmd string` read from/written as `GH_BROWSER_CMD`; `github.Login` exports `AZRL_BROWSER_CMD` to the `gh` process when set.

- [ ] **Step 1: Write the failing round-trip test** — in `internal/github/conf_test.go` `TestConfWriteAndLoad` (line 9): change the fixture to `c := Conf{Host: "github.com", User: "octocat", Label: "Work", Protocol: "https", BrowserCmd: "chrome-work"}` and extend the assertion with `|| rd.BrowserCmd != "chrome-work"`.

- [ ] **Step 2: Run** `go test ./internal/github/ -run TestConfWriteAndLoad -v` — expect compile FAIL.

- [ ] **Step 3: Implement the conf field** — `internal/github/conf.go`: struct gains `BrowserCmd string // optional local browser command overriding the global LOCAL_BROWSER_CMD`; LoadConf literal gains `BrowserCmd: m["GH_BROWSER_CMD"]`; Write becomes:

```go
	body := fmt.Sprintf("GH_HOST=%s\nGH_USER=%s\nGH_LABEL=%s\nGH_PROTOCOL=%s\nGH_BROWSER_CMD=%s\n",
		c.Host, c.User, c.Label, protocol, c.BrowserCmd)
```

- [ ] **Step 4: Run** `go test ./internal/github/ -run TestConf -v` — expect PASS.

- [ ] **Step 5: Write the failing env-plumbing test** — in `internal/github/login_test.go`, extend the `fakeGh` script (line 17) to also log the new env var:

```go
	script := "#!/usr/bin/env bash\n" +
		"{ echo \"ARGS: $*\"; echo \"GH_CONFIG_DIR=$GH_CONFIG_DIR\"; echo \"BROWSER=$BROWSER\"; echo \"AZRL_BROWSER_CMD=$AZRL_BROWSER_CMD\"; } >> \"" + log + "\"\n" +
		"exit 0\n"
```

Append the test:

```go
func TestLoginPassesProfileBrowserCmdEnv(t *testing.T) {
	log := fakeGh(t)
	profilesDir := t.TempDir()
	c := Conf{Host: "github.com", BrowserCmd: "chrome-work"}
	if err := Login(profilesDir, "work", c); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "AZRL_BROWSER_CMD=chrome-work") {
		t.Fatalf("gh env missing the profile browser cmd:\n%s", b)
	}
}
```

- [ ] **Step 6: Run** `go test ./internal/github/ -run TestLoginPassesProfileBrowserCmdEnv -v` — expect FAIL (`AZRL_BROWSER_CMD=` empty in log).

- [ ] **Step 7: Implement the env plumbing** — in `internal/github/login.go` `Login`, replace the `cmd.Env = append(...)` block:

```go
	env := append(os.Environ(),
		"GH_CONFIG_DIR="+dir,
		"BROWSER="+browserCommand(),
	)
	if c.BrowserCmd != "" {
		// Propagates to the azrl __browser shim gh spawns; LoadGlobal's
		// AZRL_BROWSER_CMD hook applies it there.
		env = append(env, "AZRL_BROWSER_CMD="+c.BrowserCmd)
	}
	cmd.Env = env
```

- [ ] **Step 8: Run** `go test ./internal/github/ -v` — expect PASS.

- [ ] **Step 9: Write the failing shim end-to-end test** — append to `cmd/browser_test.go` (clone of the existing test at line 11 with the env set):

```go
func TestBrowserShimEnvBrowserCmdOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := filepath.Join(home, ".azure-profiles")
	if err := os.MkdirAll(confdir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(confdir, "azrl.conf"),
		[]byte("LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm\n"), 0o644)
	t.Setenv("AZRL_BROWSER_CMD", "chrome-work")

	var out bytes.Buffer
	RootCmd.SetOut(&out)
	RootCmd.SetArgs([]string{"__browser", "--paste", "https://github.com/login/device"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "chrome-work https://github.com/login/device") {
		t.Fatalf("shim should honour AZRL_BROWSER_CMD; got %q", out.String())
	}
}
```

- [ ] **Step 10: Run** `go test ./cmd/ -run TestBrowserShim -v` — expect PASS immediately (Task 1 already implemented the hook — this is a regression pin, not a RED; note that in the report).

- [ ] **Step 11: Commit**

```bash
git add internal/github/conf.go internal/github/conf_test.go internal/github/login.go internal/github/login_test.go cmd/browser_test.go
git commit -m "feat(gh): GH_BROWSER_CMD per-profile browser command via env plumbing"
```

---

### Task 6: Docs + full verification

**Files:**
- Modify: `CLAUDE.md` (configuration-model bullets), `README.md` (per-profile configuration key rows, ~line 403)

**Interfaces:** none (docs only).

- [ ] **Step 1: CLAUDE.md** — in the "Configuration model" section, extend each per-profile bullet's key list: azure gains `AZ_BROWSER_CMD` (optional; local browser command overriding the global `LOCAL_BROWSER_CMD`, e.g. `google-chrome --profile-directory="Profile 2"`); github gains `GH_BROWSER_CMD`; aws gains `AWS_BROWSER_CMD`; gcp gains `GCP_BROWSER_CMD`. On the `azrl.conf` bullet, note: "the `AZRL_BROWSER_CMD` env var overrides `LOCAL_BROWSER_CMD` per process". Add one sentence on the known limitation: GCM prompts at git-push time fall back to the global command.

- [ ] **Step 2: README.md** — add the four keys to the per-profile configuration documentation, same wording; include the Chrome/Edge caveat that `--profile-directory` takes the internal directory name from `chrome://version` / `edge://version`, not the display name.

- [ ] **Step 3: Full verification**

```bash
go build ./... && gofmt -l . && go vet ./... && go test ./...
```

Expected: build clean, `gofmt -l .` empty, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: document per-profile BROWSER_CMD keys and AZRL_BROWSER_CMD"
```

---

## Verification (whole plan)

```bash
go build ./... && gofmt -l . && go vet ./... && go test ./...
```

Manual (real machine, optional — fold into the v0.7.0 manual-verify pass): set `AZ_BROWSER_CMD='google-chrome --profile-directory="Profile 2"'` in a profile conf, `azrl login <name>` → the matching browser profile opens; remove the key → unchanged global behaviour.

## Ship

`/ship` from `feat/browser-cmd-override` (PR title `feat: per-profile browser command override`). After merge, hard-sync local main (`git checkout main && git reset --hard origin/main` — it carried the unpushed spec commit).
