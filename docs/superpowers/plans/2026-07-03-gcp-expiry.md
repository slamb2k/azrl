# GCP Token Expiry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fill `gcp.Provider.Status`'s always-nil `Expiry` from gcloud's `access_tokens.db`, disk-only, so GCP profiles get the same expiry UX as Azure/AWS.

**Architecture:** A new unexported `gcpExpiry(gcDir, account string) *time.Time` in `internal/gcp/expiry.go` reads the per-account `token_expiry` row from `<gcDir>/access_tokens.db` using `github.com/alicebob/sqlittle` (pure-Go, read-only SQLite file reader). `Status` calls it with the already-resolved gcloud config dir and identity; every failure mode returns nil (today's shipped behavior). No UI/cmd/interface changes — all expiry UX keys off `Expiry != nil`.

**Tech Stack:** Go, `github.com/alicebob/sqlittle` v1.5.0 (already in go.mod on this branch), committed binary SQLite fixture in `internal/gcp/testdata/`.

**Spec:** `docs/superpowers/specs/2026-07-03-gcp-expiry-design.md` (approved).

## Global Constraints

- `Status` stays disk-only: no network, never spawns `gcloud` (provider contract, `internal/provider/provider.go:52-54`).
- `gcpExpiry` never returns an error and never panics — nil on ANY failure (absent file, corrupt DB, missing row, NULL/unparseable expiry, blank account).
- `token_expiry` format: naive UTC `YYYY-MM-DD HH:MM:SS[.ffffff]` — parse with the single Go layout `"2006-01-02 15:04:05.999999"` in `time.UTC` (`.999999` matches both with- and without-fraction values).
- Existing tests must keep passing unmodified except `TestStatusReadsIdentityAndLastUsedExpiryNil` (its "nil in v1" wording becomes "nil without a token cache" — same assertion, new rationale).
- Conventional commits with scope; `gofmt -l .` must print nothing before every commit; lefthook runs vet/build/test on commit/push.
- Work happens on the existing `feat/gcp-expiry` branch (design doc already committed there).

---

### Task 1: `gcpExpiry` — read one account's expiry from access_tokens.db

**Files:**
- Create: `internal/gcp/testdata/access_tokens.db` (binary fixture, generated once with the sqlite3 CLI)
- Create: `internal/gcp/expiry_test.go` (internal test — `package gcp`, NOT `gcp_test`, because `gcpExpiry` is unexported)
- Create: `internal/gcp/expiry.go`
- Modify: `go.mod` / `go.sum` (commit the sqlittle requirement already added on this branch)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `func gcpExpiry(gcDir, account string) *time.Time` — Task 2 calls it from `Status`. Also `copyFixtureDB(t *testing.T, dir string)` test helper (internal package) that copies the fixture into `dir/access_tokens.db`.

- [ ] **Step 1: Generate the committed fixture**

```bash
cd /home/slamb2k/work/azrl
mkdir -p internal/gcp/testdata
rm -f internal/gcp/testdata/access_tokens.db
sqlite3 internal/gcp/testdata/access_tokens.db <<'SQL'
CREATE TABLE "access_tokens" (account_id TEXT PRIMARY KEY,
  access_token TEXT, token_expiry TIMESTAMP, rapt_token TEXT, id_token TEXT,
  regional_access_boundary TEXT, regional_access_boundary_expiry TIMESTAMP);
INSERT INTO access_tokens VALUES ('expired@acme.com','tok-old','1970-01-02 03:04:05',NULL,NULL,NULL,NULL);
INSERT INTO access_tokens VALUES ('valid@acme.com','tok-new','2999-01-01 00:00:00.123456',NULL,NULL,NULL,NULL);
INSERT INTO access_tokens VALUES ('nullexp@acme.com','tok-null',NULL,NULL,NULL,NULL,NULL);
SQL
sqlite3 internal/gcp/testdata/access_tokens.db 'SELECT account_id, token_expiry FROM access_tokens;'
```

Expected output:
```
expired@acme.com|1970-01-02 03:04:05
valid@acme.com|2999-01-01 00:00:00.123456
nullexp@acme.com|
```

This matches the real schema from gcloud's `creds.py` (trailing columns exist but are NULL, like a current-version gcloud writes).

- [ ] **Step 2: Write the failing tests**

Create `internal/gcp/expiry_test.go`:

```go
package gcp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// copyFixtureDB copies the committed access_tokens.db fixture into dir,
// standing in for a real gcloud config dir.
func copyFixtureDB(t *testing.T, dir string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "access_tokens.db"))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, "access_tokens.db"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGcpExpiryReadsPastExpiry(t *testing.T) {
	dir := t.TempDir()
	copyFixtureDB(t, dir)
	got := gcpExpiry(dir, "expired@acme.com")
	want := time.Date(1970, 1, 2, 3, 4, 5, 0, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Fatalf("gcpExpiry = %v, want %v", got, want)
	}
}

func TestGcpExpiryParsesFractionalSeconds(t *testing.T) {
	dir := t.TempDir()
	copyFixtureDB(t, dir)
	got := gcpExpiry(dir, "valid@acme.com")
	want := time.Date(2999, 1, 1, 0, 0, 0, 123456000, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Fatalf("gcpExpiry = %v, want %v", got, want)
	}
}

func TestGcpExpiryNilCases(t *testing.T) {
	dir := t.TempDir()
	copyFixtureDB(t, dir)
	cases := map[string]struct{ dir, account string }{
		"unknown account": {dir, "nobody@acme.com"},
		"NULL expiry":     {dir, "nullexp@acme.com"},
		"blank account":   {dir, ""},
		"absent file":     {t.TempDir(), "expired@acme.com"},
	}
	for name, c := range cases {
		if got := gcpExpiry(c.dir, c.account); got != nil {
			t.Fatalf("%s: gcpExpiry = %v, want nil", name, got)
		}
	}
}

func TestGcpExpiryCorruptFileIsNil(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "access_tokens.db"), []byte("not a sqlite file"), 0o644)
	if got := gcpExpiry(dir, "expired@acme.com"); got != nil {
		t.Fatalf("gcpExpiry on corrupt db = %v, want nil", got)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail correctly**

Run: `go test ./internal/gcp/ -run 'TestGcpExpiry' 2>&1 | head -10`
Expected: `FAIL` — build error `undefined: gcpExpiry` (the only missing symbol; the helper compiles).

- [ ] **Step 4: Write the minimal implementation**

Create `internal/gcp/expiry.go`:

```go
package gcp

import (
	"path/filepath"
	"time"

	"github.com/alicebob/sqlittle"
)

// gcpExpiry reads the cached access-token expiry for account from
// <gcDir>/access_tokens.db — the SQLite cache gcloud writes in
// rollback-journal mode with token_expiry as naive-UTC
// "YYYY-MM-DD HH:MM:SS[.ffffff]" text. Best-effort and disk-only: nil on
// any read/parse error, a missing row, or a NULL expiry.
func gcpExpiry(gcDir, account string) *time.Time {
	if account == "" {
		return nil
	}
	db, err := sqlittle.Open(filepath.Join(gcDir, "access_tokens.db"))
	if err != nil {
		return nil
	}
	defer db.Close()
	var out *time.Time
	selErr := db.Select("access_tokens", func(r sqlittle.Row) {
		acct, expiry, err := r.ScanStringString()
		if err != nil || acct != account || expiry == "" {
			return
		}
		// .999999 matches both with- and without-fraction values.
		if t, err := time.ParseInLocation("2006-01-02 15:04:05.999999", expiry, time.UTC); err == nil {
			out = &t
		}
	}, "account_id", "token_expiry")
	if selErr != nil {
		return nil
	}
	return out
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/gcp/ -run 'TestGcpExpiry' -v 2>&1 | tail -15`
Expected: all four `TestGcpExpiry*` tests `PASS`.

- [ ] **Step 6: Full package + format check**

Run: `go test ./internal/gcp/ && gofmt -l .`
Expected: `ok`, and gofmt prints nothing.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/gcp/expiry.go internal/gcp/expiry_test.go internal/gcp/testdata/access_tokens.db
git commit -m "$(cat <<'EOF'
feat(gcp): read per-account token expiry from access_tokens.db

gcpExpiry reads gcloud's SQLite token cache via the pure-Go read-only
sqlittle reader (gcloud uses rollback-journal mode, which a cold file
read handles). Best-effort: nil on any failure, matching the disk-only
Status contract.

Claude-Session: https://claude.ai/code/session_01UPQTdTR4tKq5XRX5ewvetk
EOF
)"
```

---

### Task 2: Wire expiry into `Status` (+ LastUsed mtime, docs)

**Files:**
- Modify: `internal/gcp/status.go:11-36` (the `Status` function and its doc comment)
- Modify: `internal/gcp/status_test.go` (adjust the "nil in v1" test; add the token-cache test)
- Modify: `CLAUDE.md` (the `internal/gcp/` bullet's "Expiry nil in v1" clause)

**Interfaces:**
- Consumes: `gcpExpiry(gcDir, account string) *time.Time` from Task 1; existing `gcloudConfigDir(name, confdir string, isolate bool) string`, `gcpIdentity(name, confdir, configName string, isolate bool) string`, `provider.LatestMtime(base time.Time, paths ...string) time.Time`.
- Produces: `Status` returns non-nil `Expiry` when the profile's account has a cached token — consumed by all existing UI/cmd surfaces automatically.

- [ ] **Step 1: Write the failing test**

In `internal/gcp/status_test.go` (external `package gcp_test` — it can reach the fixture via the shared `testdata/` dir), append:

```go
func copyTokenCache(t *testing.T, gcloudDir string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "access_tokens.db"))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(gcloudDir, 0o755)
	if err := os.WriteFile(filepath.Join(gcloudDir, "access_tokens.db"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatusReadsExpiryFromTokenCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := filepath.Join(home, ".gcp-profiles")
	os.MkdirAll(confdir, 0o755)
	os.WriteFile(filepath.Join(confdir, "work.conf"),
		[]byte("GCP_CONFIG_NAME=work\nGCP_PROJECT=acme-prod\n"), 0o644)
	gcloudDir := filepath.Join(home, ".config", "gcloud")
	writeConfigINI(t, gcloudDir, "work", "expired@acme.com")
	copyTokenCache(t, gcloudDir)
	// Pin the config INI's mtime to the past so the LastUsed assertion can
	// only be satisfied by the freshly-written access_tokens.db.
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(filepath.Join(gcloudDir, "configurations", "config_work"), old, old)

	st, err := gcp.NewProvider().Status("work", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Expiry == nil {
		t.Fatal("Expiry should be read from access_tokens.db")
	}
	want := time.Date(1970, 1, 2, 3, 4, 5, 0, time.UTC)
	if !st.Expiry.Equal(want) {
		t.Fatalf("Expiry = %v, want %v", st.Expiry, want)
	}
	if st.LastUsed.Before(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("LastUsed should fold in the fresh access_tokens.db mtime")
	}
}
```

Add `"time"` to the file's imports (it currently imports only os/filepath/testing/gcp).

Also update the stale wording in `TestStatusReadsIdentityAndLastUsedExpiryNil` (same file): rename to `TestStatusExpiryNilWithoutTokenCache` and change the failure message `"Expiry must be nil in v1, got %v"` to `"Expiry must be nil without a token cache, got %v"`. The assertions themselves don't change (its seeded home has no `access_tokens.db`, so nil is still correct).

- [ ] **Step 2: Run test to verify it fails correctly**

Run: `go test ./internal/gcp/ -run 'TestStatusReadsExpiryFromTokenCache' -v 2>&1 | tail -5`
Expected: `FAIL` at `Expiry should be read from access_tokens.db` (Status still hardcodes nil).

- [ ] **Step 3: Wire it in**

In `internal/gcp/status.go`, replace the `Status` function and its doc comment with:

```go
// Status returns a disk-only snapshot of profile name from its conf and the
// gcloud configuration files. It never spawns gcloud or makes a network call.
// Expiry comes from the per-account token_expiry row in access_tokens.db,
// read via the pure-Go sqlittle reader (see gcpExpiry); nil when absent.
func (Provider) Status(name, confdir string) (provider.Status, error) {
	c, _ := LoadConf(name, confdir)
	last, dir := scheme.LastTouch(name, confdir)
	configName := c.ResolvedConfigName(name)
	gcDir := gcloudConfigDir(name, confdir, c.Isolate)
	last = provider.LatestMtime(last,
		filepath.Join(gcDir, "configurations", "config_"+configName),
		filepath.Join(gcDir, "active_config"),
		filepath.Join(gcDir, "credentials.db"),
		filepath.Join(gcDir, "access_tokens.db"))
	drifted := driftedDefault(name, confdir, configName)
	if c.Isolate {
		drifted = driftedIsolate(name, confdir)
	}
	identity := gcpIdentity(name, confdir, configName, c.Isolate)
	return provider.Status{
		ProfileName: name,
		Identity:    identity,
		Directory:   dir,
		Expiry:      gcpExpiry(gcDir, identity),
		Drifted:     drifted,
		LastUsed:    last,
	}, nil
}
```

(Two changes beyond expiry: `access_tokens.db` joins the `LatestMtime` list so
external token refreshes re-sort the dashboard, and `gcpIdentity` moves to a
variable so it feeds both `Identity` and `gcpExpiry`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/gcp/ -v -run 'TestStatus' 2>&1 | tail -12`
Expected: `TestStatusReadsExpiryFromTokenCache` and all other `TestStatus*` PASS (including the renamed `TestStatusExpiryNilWithoutTokenCache`).

- [ ] **Step 5: Update CLAUDE.md**

In the `internal/gcp/` architecture bullet, replace the clause:

`` `Expiry` nil in v1, no network ``

with:

`` `Expiry` read per-account from `access_tokens.db` via the pure-Go read-only sqlittle reader, no network ``

- [ ] **Step 6: Full suite + format check**

Run: `go build ./... && go test ./... 2>&1 | tail -16 && gofmt -l .`
Expected: every package `ok` (the provider contract suite in `internal/provider` and `internal/gcp` must stay green — `Ambient()`/`Status` remain no-network), gofmt prints nothing.

- [ ] **Step 7: Commit**

```bash
git add internal/gcp/status.go internal/gcp/status_test.go CLAUDE.md
git commit -m "$(cat <<'EOF'
feat(gcp): fill Status.Expiry from the gcloud token cache

Status now resolves the profile's account to its access_tokens.db row
(default and --isolate dirs both covered by gcloudConfigDir), closing
the v1 expiry gap; access_tokens.db mtime also folds into LastUsed so
external refreshes re-sort the dashboard, matching Azure.

Claude-Session: https://claude.ai/code/session_01UPQTdTR4tKq5XRX5ewvetk
EOF
)"
```
