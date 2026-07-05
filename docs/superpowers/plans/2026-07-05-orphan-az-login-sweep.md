# Orphaned az-login Sweep Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `azrl login` reaps orphaned `az login` processes (parent dead) before starting, and warns about live ones that could steal the browser callback — never killing a legitimately running login.

**Architecture:** A new pure classifier in `internal/azure/orphans.go` walks a proc filesystem root (production `/proc`) and splits the current user's `az login` processes into orphans (`PPid: 1`) and live. `SweepOrphanedLogins` SIGTERMs orphans and prints a note per live process. `CleanSlate` gains an `io.Writer` and runs the sweep first. Everything is best-effort: no `/proc` (macOS/Windows) or any read/kill error is a silent no-op.

**Tech Stack:** Go stdlib only (`os`, `strings`, `strconv`, `syscall.SIGTERM` via `os.Process.Signal`). Tests use fixture proc trees under `t.TempDir()` and swap package-level `procFS`/`killProc` vars.

**Spec:** `docs/superpowers/specs/2026-07-05-orphan-az-login-sweep-design.md`

## Global Constraints

- Cross-compilation: GoReleaser builds `goos: [linux, darwin, windows]`. NEVER call `syscall.Kill` (undefined on Windows). Use `os.FindProcess(pid)` + `p.Signal(syscall.SIGTERM)` — `syscall.SIGTERM` is defined on all three OSes.
- The sweep must never block or fail a login: all errors ignored, no retries, no escalation to SIGKILL.
- Only processes owned by the current real UID are ever considered.
- Conventional commits with scope; append the trailer line `Claude-Session: https://claude.ai/code/session_01UPQTdTR4tKq5XRX5ewvetk` to each commit message.
- Verify with `go build ./... && gofmt -l . && go vet ./... && go test ./...` (gofmt output must be empty). lefthook runs gofmt/vet on pre-commit, build/test on pre-push.
- Work on branch `feat/orphan-az-login-sweep` (already created; the spec is committed on it).

---

### Task 1: Pure classifier — matcher, /proc walk, age

**Files:**
- Create: `internal/azure/orphans.go`
- Create: `internal/azure/orphans_test.go`

**Interfaces:**
- Consumes: nothing from this feature (stdlib only).
- Produces (Task 2 relies on these exact names):
  - `type loginProc struct { PID int; Age time.Duration }`
  - `func isAzLoginCmdline(argv []string) bool`
  - `func classifyAzLogins(procRoot string, uid int) (orphans, live []loginProc)`
  - `func formatAge(d time.Duration) string`

- [ ] **Step 1: Write the failing tests**

Create `internal/azure/orphans_test.go`:

```go
package azure

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIsAzLoginCmdline(t *testing.T) {
	cases := []struct {
		argv []string
		want bool
	}{
		{[]string{"az", "login"}, true},
		{[]string{"az", "login", "--tenant", "x"}, true},
		{[]string{"/usr/bin/python3", "/usr/bin/az", "login", "--tenant", "x"}, true},
		{[]string{"/opt/az/bin/python3", "-Im", "azure.cli", "login"}, true},
		{[]string{"az", "logout"}, false},
		{[]string{"az", "account", "show"}, false},
		{[]string{"azrl", "login", "work"}, false},
		{[]string{"/home/u/.local/bin/azrl", "login"}, false},
		{[]string{"bash"}, false},
		{[]string{}, false},
		{[]string{"login", "az"}, false}, // login must FOLLOW the az token
	}
	for _, c := range cases {
		if got := isAzLoginCmdline(c.argv); got != c.want {
			t.Errorf("isAzLoginCmdline(%q) = %v, want %v", c.argv, got, c.want)
		}
	}
}

// writeProcEntry fabricates /proc/<pid>/{cmdline,status,stat} under root.
// startTicks == "" omits the stat file (age parsing must degrade to zero).
func writeProcEntry(t *testing.T, root string, pid int, argv []string, uid, ppid int, startTicks string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmdline := strings.Join(argv, "\x00")
	if len(argv) > 0 {
		cmdline += "\x00"
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0o644); err != nil {
		t.Fatal(err)
	}
	status := fmt.Sprintf("Name:\taz\nState:\tS (sleeping)\nPPid:\t%d\nUid:\t%d\t%d\t%d\t%d\n", ppid, uid, uid, uid, uid)
	if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
	if startTicks != "" {
		// Field 22 (starttime) is the 20th field after the ")" — comm contains
		// a space to prove the parser anchors on the LAST ")".
		stat := fmt.Sprintf("%d (python3 az) S %d 1 1 0 -1 4194304 0 0 0 0 0 0 0 0 20 0 1 0 %s 0 0",
			pid, ppid, startTicks)
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClassifyAzLogins(t *testing.T) {
	root := t.TempDir()
	// uptime 5000s; ticks are USER_HZ=100.
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("5000.00 9000.00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uid := 1000
	writeProcEntry(t, root, 101, []string{"az", "login", "--tenant", "x"}, uid, 1, "380000")            // orphan, started 3800s → age 1200s
	writeProcEntry(t, root, 102, []string{"/usr/bin/python3", "/usr/bin/az", "login"}, uid, 500, "")   // live (parent 500), no stat → age 0
	writeProcEntry(t, root, 103, []string{"az", "login"}, 2000, 1, "380000")                            // wrong uid → skipped
	writeProcEntry(t, root, 104, []string{"bash"}, uid, 1, "380000")                                    // not az login → skipped
	// Malformed entry: empty cmdline.
	writeProcEntry(t, root, 105, nil, uid, 1, "")
	// Non-numeric entry must be ignored.
	if err := os.MkdirAll(filepath.Join(root, "self"), 0o755); err != nil {
		t.Fatal(err)
	}

	orphans, live := classifyAzLogins(root, uid)
	if len(orphans) != 1 || orphans[0].PID != 101 {
		t.Fatalf("orphans = %+v, want exactly pid 101", orphans)
	}
	if orphans[0].Age != 1200*time.Second {
		t.Errorf("orphan age = %s, want 20m0s", orphans[0].Age)
	}
	if len(live) != 1 || live[0].PID != 102 {
		t.Fatalf("live = %+v, want exactly pid 102", live)
	}
	if live[0].Age != 0 {
		t.Errorf("live age = %s, want 0 (missing stat degrades to zero)", live[0].Age)
	}
}

func TestClassifyAzLoginsMissingRoot(t *testing.T) {
	orphans, live := classifyAzLogins(filepath.Join(t.TempDir(), "nope"), 1000)
	if orphans != nil || live != nil {
		t.Fatalf("expected nil/nil on missing proc root, got %+v / %+v", orphans, live)
	}
}

func TestFormatAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "age unknown"},
		{45 * time.Second, "45s"},
		{20 * time.Minute, "20m"},
		{90 * time.Minute, "1h30m"},
	}
	for _, c := range cases {
		if got := formatAge(c.d); got != c.want {
			t.Errorf("formatAge(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/azure/ -run 'TestIsAzLoginCmdline|TestClassifyAzLogins|TestFormatAge' -v`
Expected: compile error — `isAzLoginCmdline`, `classifyAzLogins`, `formatAge`, `loginProc` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/azure/orphans.go`:

```go
package azure

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// loginProc is one az-login process found during the orphan sweep.
type loginProc struct {
	PID int
	Age time.Duration
}

// isAzLoginCmdline reports whether argv is an interactive `az login`
// invocation. `az` is a launcher for `python -m azure.cli`, so the visible
// cmdline varies by install: match when some token is the az launcher (`az`,
// `*/az`, or contains `azure.cli`) and a LATER token is exactly `login`.
// Rejects `az logout`, `azrl login` (azrl itself), and unrelated pythons.
func isAzLoginCmdline(argv []string) bool {
	azAt := -1
	for i, a := range argv {
		if a == "az" || strings.HasSuffix(a, "/az") || strings.Contains(a, "azure.cli") {
			azAt = i
			break
		}
	}
	if azAt < 0 {
		return false
	}
	for _, a := range argv[azAt+1:] {
		if a == "login" {
			return true
		}
	}
	return false
}

// classifyAzLogins walks the numeric entries of procRoot (production: /proc)
// and splits the given user's az-login processes into orphans — parent dead,
// reparented to PID 1 — and live ones. Malformed or unreadable entries are
// skipped; a missing procRoot (non-Linux) yields nothing.
func classifyAzLogins(procRoot string, uid int) (orphans, live []loginProc) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, nil
	}
	uptime := readUptime(procRoot)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "cmdline"))
		if err != nil || len(raw) == 0 {
			continue
		}
		argv := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		if !isAzLoginCmdline(argv) {
			continue
		}
		puid, ppid, ok := readStatus(filepath.Join(procRoot, e.Name(), "status"))
		if !ok || puid != uid {
			continue
		}
		p := loginProc{PID: pid, Age: procAge(filepath.Join(procRoot, e.Name(), "stat"), uptime)}
		if ppid == 1 {
			orphans = append(orphans, p)
		} else {
			live = append(live, p)
		}
	}
	return orphans, live
}

// readStatus extracts the real UID and PPid from a /proc/<pid>/status file.
func readStatus(path string) (uid, ppid int, ok bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, false
	}
	uid, ppid = -1, -1
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "Uid:":
			uid, _ = strconv.Atoi(f[1])
		case "PPid:":
			ppid, _ = strconv.Atoi(f[1])
		}
	}
	return uid, ppid, uid >= 0 && ppid >= 0
}

// readUptime returns the system uptime in seconds, zero on any failure.
func readUptime(procRoot string) float64 {
	raw, err := os.ReadFile(filepath.Join(procRoot, "uptime"))
	if err != nil {
		return 0
	}
	f := strings.Fields(string(raw))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}

// procAge derives a process age from /proc/<pid>/stat field 22 (starttime, in
// clock ticks since boot) and the system uptime. The comm field (2) may
// contain spaces, so parsing anchors on the LAST ')'. Zero on any failure.
// ponytail: USER_HZ hardcoded to 100 (the Linux default); age is display-only.
func procAge(statPath string, uptime float64) time.Duration {
	raw, err := os.ReadFile(statPath)
	if err != nil || uptime == 0 {
		return 0
	}
	i := strings.LastIndexByte(string(raw), ')')
	if i < 0 {
		return 0
	}
	f := strings.Fields(string(raw)[i+1:])
	if len(f) < 20 {
		return 0
	}
	ticks, err := strconv.ParseFloat(f[19], 64)
	if err != nil {
		return 0
	}
	age := uptime - ticks/100
	if age < 0 {
		return 0
	}
	return time.Duration(age * float64(time.Second))
}

// formatAge renders a process age for the live-login warning line.
func formatAge(d time.Duration) string {
	switch {
	case d <= 0:
		return "age unknown"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/azure/ -run 'TestIsAzLoginCmdline|TestClassifyAzLogins|TestFormatAge' -v`
Expected: all PASS. Then `gofmt -l .` (empty) and `go vet ./...` (clean).

- [ ] **Step 5: Commit**

```bash
git add internal/azure/orphans.go internal/azure/orphans_test.go
git commit -m "feat(azure): classify orphaned az-login processes from /proc

Claude-Session: https://claude.ai/code/session_01UPQTdTR4tKq5XRX5ewvetk"
```

---

### Task 2: Sweep + CleanSlate wiring + docs

**Files:**
- Modify: `internal/azure/orphans.go` (append sweep)
- Modify: `internal/azure/orphans_test.go` (append sweep test)
- Modify: `internal/azure/account.go:13-23` (`CleanSlate` signature + sweep call)
- Modify: `internal/azure/account_test.go:22-38` (`TestCleanSlate`)
- Modify: `cmd/login.go:87` (call site)
- Modify: `cmd/init.go:40` (call site, inside `runAzureInit`)
- Modify: `CLAUDE.md:56` (login-flow step 1)
- Modify: `README.md:173` (insert a sentence after the `.envrc` paragraph)

**Interfaces:**
- Consumes (from Task 1): `classifyAzLogins(procRoot string, uid int) (orphans, live []loginProc)`, `formatAge(d time.Duration) string`, `loginProc{PID, Age}`.
- Produces: `func SweepOrphanedLogins(out io.Writer)`; `func CleanSlate(cfgDir string, out io.Writer) error`; package vars `procFS string`, `killProc func(pid int) error` (test seams).

- [ ] **Step 1: Write the failing sweep test**

Append to `internal/azure/orphans_test.go` (add `"bytes"` to imports):

```go
func TestSweepOrphanedLogins(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "uptime"), []byte("5000.00 9000.00\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uid := os.Getuid()
	writeProcEntry(t, root, 101, []string{"az", "login", "--tenant", "x"}, uid, 1, "380000")          // orphan, age 20m
	writeProcEntry(t, root, 102, []string{"/usr/bin/python3", "/usr/bin/az", "login"}, uid, 500, "460000") // live, age 6m

	oldFS, oldKill := procFS, killProc
	var killed []int
	procFS = root
	killProc = func(pid int) error { killed = append(killed, pid); return nil }
	t.Cleanup(func() { procFS, killProc = oldFS, oldKill })

	var buf bytes.Buffer
	SweepOrphanedLogins(&buf)

	if len(killed) != 1 || killed[0] != 101 {
		t.Fatalf("killed = %v, want exactly [101]", killed)
	}
	out := buf.String()
	if !strings.Contains(out, "azrl: reaped orphaned az login (pid 101)") {
		t.Errorf("missing reap line, got: %s", out)
	}
	if !strings.Contains(out, "azrl: note: another az login is running (pid 102, 6m)") {
		t.Errorf("missing live-login note, got: %s", out)
	}
}

func TestSweepOrphanedLoginsNoProc(t *testing.T) {
	oldFS, oldKill := procFS, killProc
	procFS = filepath.Join(t.TempDir(), "nope")
	killProc = func(pid int) error { t.Fatalf("kill(%d) called with no proc root", pid); return nil }
	t.Cleanup(func() { procFS, killProc = oldFS, oldKill })

	var buf bytes.Buffer
	SweepOrphanedLogins(&buf)
	if buf.Len() != 0 {
		t.Fatalf("expected silence without /proc, got: %s", buf.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/azure/ -run TestSweepOrphanedLogins -v`
Expected: compile error — `procFS`, `killProc`, `SweepOrphanedLogins` undefined.

- [ ] **Step 3: Implement the sweep**

Append to `internal/azure/orphans.go` (add `"io"` and `"syscall"` to its imports):

```go
// procFS and killProc are package seams so tests can redirect the sweep away
// from the real /proc and real signals. killProc uses os.Process.Signal (NOT
// syscall.Kill) so the package still compiles for windows/darwin release
// builds; on hosts without /proc it is never reached.
var (
	procFS   = "/proc"
	killProc = func(pid int) error {
		p, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		return p.Signal(syscall.SIGTERM)
	}
)

// SweepOrphanedLogins reaps the current user's orphaned az-login processes —
// parent dead, reparented to PID 1, typically leftovers of manual `az login`
// runs whose terminal died — and warns about live ones, which can steal the
// next login's browser callback. Best-effort: without /proc (macOS, Windows)
// and on any read or kill error it is a silent no-op, never blocking a login.
func SweepOrphanedLogins(out io.Writer) {
	orphans, live := classifyAzLogins(procFS, os.Getuid())
	for _, p := range orphans {
		if killProc(p.PID) == nil {
			fmt.Fprintf(out, "azrl: reaped orphaned az login (pid %d)\n", p.PID)
		}
	}
	for _, p := range live {
		fmt.Fprintf(out, "azrl: note: another az login is running (pid %d, %s) — it may steal the browser callback\n", p.PID, formatAge(p.Age))
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/azure/ -run TestSweepOrphanedLogins -v`
Expected: both PASS.

- [ ] **Step 5: Wire into CleanSlate (red then green)**

In `internal/azure/account.go`, replace the `CleanSlate` doc comment and function (lines 13-23) with:

```go
// CleanSlate reaps orphaned az-login processes (warning about live ones),
// logs out, clears accounts, and removes the scoped MSAL caches in cfgDir.
// The az errors are intentionally ignored (a fresh box has nothing to
// clear); only filesystem errors surface — and those are ignored too since
// the files may legitimately be absent.
func CleanSlate(cfgDir string, out io.Writer) error {
	SweepOrphanedLogins(out)
	_ = exec.Command("az", "logout").Run()
	_ = exec.Command("az", "account", "clear").Run()
	os.Remove(filepath.Join(cfgDir, "msal_token_cache.json"))
	os.Remove(filepath.Join(cfgDir, "service_principal_entries.json"))
	return nil
}
```

Add `"io"` to `account.go`'s imports.

In `internal/azure/account_test.go`, update `TestCleanSlate`: point `procFS` at an empty temp dir (so the test never scans the real `/proc` of the machine running the suite) and pass the new writer:

```go
func TestCleanSlate(t *testing.T) {
	log := filepath.Join(t.TempDir(), "az.log")
	shimAz(t, log)
	oldFS := procFS
	procFS = t.TempDir()
	t.Cleanup(func() { procFS = oldFS })
	cfg := t.TempDir()
	os.WriteFile(filepath.Join(cfg, "msal_token_cache.json"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(cfg, "service_principal_entries.json"), []byte("x"), 0o644)
	if err := CleanSlate(cfg, io.Discard); err != nil {
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
```

Add `"io"` to `account_test.go`'s imports.

Update the two call sites (both already have `out` in scope):
- `cmd/login.go:87`: `azure.CleanSlate(cfgDir)` → `azure.CleanSlate(cfgDir, out)`
- `cmd/init.go:40` (in `runAzureInit`): `azure.CleanSlate(cfgDir)` → `azure.CleanSlate(cfgDir, out)`

- [ ] **Step 6: Run the full suite**

Run: `go build ./... && gofmt -l . && go vet ./... && go test ./...`
Expected: build OK, gofmt empty, vet clean, all packages PASS (any other test calling `CleanSlate` with the old signature will surface here — fix by passing `io.Discard`).

- [ ] **Step 7: Docs**

`CLAUDE.md` line 56 — replace the login-flow step 1 bullet with:

```markdown
1. `CleanSlate` — reaps orphaned `az login` processes (same user, parent dead — leftovers of earlier attempts that would steal the browser callback) and warns about live ones, then `az logout` + `az account clear`, remove scoped MSAL caches within the isolated `AZURE_CONFIG_DIR`.
```

`README.md` — after the paragraph ending at line 173 (“…follows the profile from then on.”), insert:

```markdown
`login` also starts from a clean slate: it reaps orphaned `az login`
processes (same user, parent process dead) left behind by earlier attempts —
zombies that would otherwise steal the OAuth browser callback — and prints a
note about any *live* `az login` it finds, without killing it.
```

- [ ] **Step 8: Commit**

```bash
git add internal/azure/orphans.go internal/azure/orphans_test.go internal/azure/account.go internal/azure/account_test.go cmd/login.go cmd/init.go CLAUDE.md README.md
git commit -m "feat(azure): sweep orphaned az logins in CleanSlate

Reap same-user az-login processes whose parent died (PPid 1) before each
login; warn about live ones that could steal the browser callback.

Claude-Session: https://claude.ai/code/session_01UPQTdTR4tKq5XRX5ewvetk"
```
