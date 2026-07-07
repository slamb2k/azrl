# Entity-Model Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the TUI's surfaces match the entity model the user articulated: provider tabs manage **personas** (Renew · Shell as… · Open console… · Assign browser… · Delete…), the dashboard manages **directory↔profile edges** (row-`u` link, new `U` unlink), New profile becomes a persistent row atop the PROFILES pane that creates + signs in **without linking**, and Delete becomes link-aware (list linked dirs; Unlink all / Replace with…). New CLI verbs/flags keep CLI-first parity: `azrl unlink` (refuse on parent links), `azrl login --no-link`, `azrl rm --unlink-all|--replace <name>`.

**Architecture:** Everything builds on existing primitives: `Scheme.Locate` (parent detection for unlink), `Scheme.Use`/pointer writes (replace-links), `Scheme.ReadMappings`/`RemoveMapping` (link enumeration/cleanup), the provider registry, and the TUI's naming-prompt/confirm-radio machinery. Azure's pin-on-create lives in `captureSession`'s direct pointer write (gains a `link bool`); gh/aws/gcp pin via `if created { prov.Use }` (gains `&& !noLink`). The New-profile row is a synthetic index-0 entry in the PROFILES pane whose DETAILS pane explains the entity ("a container for tokens and intention").

**Tech Stack:** Go, cobra, Bubble Tea + existing bubblezone wiring, established PATH-shim/`View()`-assertion test patterns.

## Global Constraints

- Gates before every commit: `go build ./...`, `go test ./...`, `gofmt -l .` (empty = clean). Conventional commits with scope.
- **CLI-first**: every TUI verb execs (or mirrors in-process, where tabs already do) a real subcommand. CLI pin-on-create default is UNCHANGED (`azrl login <name>` still links — the optimized shortcut); only the TUI passes `--no-link`.
- **Unlink refuses on parent links**: it only ever deletes `<cwd>/<Pointer>`; when the governing pointer is in a parent, it errors naming that path — exact copy: `azrl: this directory is governed by <dir>/<pointer> (profile <name>) — run unlink there`.
- Never-hide action model stays: tab everyday ACTIONS become exactly 5 — `s` Renew, `t` Shell as…, `c` Open console…, `b` Assign browser…, `delete` Delete… — always listed, disabled-with-reason where inapplicable. `u`/Link here leaves the tabs entirely (dashboard row-`u` owns it). `n`/New profile leaves ACTIONS for the pane row (the `n` accelerator still works).
- Empty-state ACTIONS = 1 (`a` Capture session); the New-profile row covers create.
- Labels/copy exactly: `Assign browser…` (hint `map to a local browser profile` unchanged), `Delete…` (hint `delete profile`), pane row `＋ New profile…`, naming hint for create `token container + sign-in — link it later`, DETAILS blurb for the row: `A profile is a container for tokens and intention — sign in once, link it to any number of directories.`
- Mirror-never-actor and the `Provider` interface unchanged. UI language "link" never "pin".
- Delete-with-links: the confirm lists the linked directories (first 3 + `+ n more`) and offers Cancel / `Unlink N dir(s) + delete` / `Replace links with…` (disabled with reason `no other profile to point them at` when the provider has no other profile).

## Recorded assumptions (surface to the user at the end)

1. **Unlink is row-scoped on the dashboard** (`U` acts on the selected row): mapping row whose dir IS the cwd → unlink; mapping row for another dir → status `links are removed where they live — run unlink in <dir>`; ambient/unmapped rows → status `no directory link on this row`. This generalizes "refuse on parent" to "refuse on elsewhere".
2. **`rm` with links refuses by default** (lists the dirs + the two flags) — the `[y/N]` prompt remains only for the zero-links case on azure; gh/aws/gcp rm stay promptless as today.
3. **`--replace <other>` rewrites each linked dir's pointer file and mapping row** to the other profile (validated to exist, same provider) and then deletes; it does not Touch/SetupRepo/SyncConfig per dir (a later `use` or resolution does the provider-native extras — replace is an edge rewrite, not N use-flows).
4. **Capture keeps linking the cwd** (adopt semantics untouched) — only login grows `--no-link`.
5. **The `n` accelerator and helpOverlay entry survive** even though New profile left the ACTIONS radio — the pane row is the visible affordance, `n` is muscle memory.
6. **TUI Delete drives the same code paths as the CLI flags in-process** (pointer removals + `RemoveMapping` + `prov.Remove`), mirroring how tabs already call `Use`/`Remove` in-process.

---

### Task 1: `Scheme.Unlink` + `azrl unlink` on all four surfaces

**Files:**
- Modify: `internal/profile/scheme.go` (append), `cmd/gh.go`/`cmd/aws.go`/`cmd/gcp.go` (subcommand slices)
- Create: `cmd/unlink.go`
- Test: `internal/profile/scheme_test.go` (append), `cmd/manage_test.go` (append)

**Interfaces:**
- Produces: `Scheme.Unlink(confdir, pwd string) (string, error)` — removes `<pwd>/<Pointer>`, removes the pointer-sourced mapping row, returns the profile name it pointed at; errors per the Global Constraints copy when governed by a parent, and `azrl: nothing linked in <pwd>` (with the scheme's Prefix, not literal "azrl", for non-azure) when nothing governs at all. `runUnlink(providerName string, out io.Writer) error` + `newUnlinkCmd(providerName, short string)` in cmd. Task 6 (TUI Delete) and Task 7 (dashboard `U`) reuse `Scheme.Unlink`.
- Consumes: `Scheme.Locate` (scheme.go:45), `RemoveMapping` (mapping.go:134), `provider.All()` registry (Scheme()+ProfilesDir()).

- [ ] **Step 1: Write the failing tests**

Append to `internal/profile/scheme_test.go` (mirror its existing temp-dir idioms):

```go
func TestUnlinkRemovesCwdPointerAndMapping(t *testing.T) {
	confdir := t.TempDir()
	work := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	s := AzureScheme()
	if err := s.Use("acme", confdir, work); err != nil {
		t.Fatal(err)
	}
	name, err := s.Unlink(confdir, work)
	if err != nil || name != "acme" {
		t.Fatalf("Unlink = %q, %v", name, err)
	}
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); !os.IsNotExist(err) {
		t.Fatal("pointer file should be gone")
	}
	for _, m := range ReadMappings(confdir) {
		if m.Dir == work {
			t.Fatalf("mapping row should be gone: %+v", m)
		}
	}
}

func TestUnlinkRefusesParentGovernedDir(t *testing.T) {
	confdir := t.TempDir()
	parent := t.TempDir()
	child := filepath.Join(parent, "sub")
	os.MkdirAll(child, 0o755)
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	s := AzureScheme()
	if err := s.Use("acme", confdir, parent); err != nil {
		t.Fatal(err)
	}
	_, err := s.Unlink(confdir, child)
	if err == nil || !strings.Contains(err.Error(), parent) || !strings.Contains(err.Error(), "run unlink there") {
		t.Fatalf("parent-governed unlink must refuse naming the parent: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(parent, ".azprofile")); statErr != nil {
		t.Fatal("the parent's pointer must be untouched")
	}
}

func TestUnlinkNothingLinked(t *testing.T) {
	if _, err := AzureScheme().Unlink(t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("unlink with nothing governing should error")
	}
}
```

Append a cmd-level test to `cmd/manage_test.go` (mirror `TestUseCmd` at :26):

```go
func TestUnlinkCmd(t *testing.T) {
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
	RootCmd.SetArgs([]string{"unlink"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); !os.IsNotExist(err) {
		t.Fatal("unlink should remove .azprofile")
	}
}

func TestUnlinkVerbRegisteredOnAllSurfaces(t *testing.T) {
	find := func(cmds []*cobra.Command) bool {
		for _, c := range cmds {
			if strings.HasPrefix(c.Use, "unlink") {
				return true
			}
		}
		return false
	}
	if !find(RootCmd.Commands()) || !find(githubSubcommands()) || !find(awsSubcommands()) || !find(gcpSubcommands()) {
		t.Fatal("unlink missing from a surface")
	}
}
```

(Add imports as needed; check whether manage_test.go already imports cobra/strings.)

- [ ] **Step 2: RED** — `go test ./internal/profile/ ./cmd/ -run TestUnlink -v` → FAIL, `undefined: Unlink`.

- [ ] **Step 3: Implement**

Append to `internal/profile/scheme.go`:

```go
// Unlink removes the calling directory's own pointer file and its mapping
// row, returning the profile it pointed at. It never reaches into parents:
// a directory governed from above is refused with the governing path — links
// are removed where they live.
func (s Scheme) Unlink(confdir, pwd string) (string, error) {
	ptr := filepath.Join(pwd, s.Pointer)
	b, err := os.ReadFile(ptr)
	if err != nil {
		if pdir, ok := s.Locate(pwd); ok {
			name, _ := s.Resolve("", pwd)
			return "", fmt.Errorf("%s: this directory is governed by %s (profile %s) — run unlink there",
				s.Prefix, filepath.Join(pdir, s.Pointer), name)
		}
		return "", fmt.Errorf("%s: nothing linked in %s", s.Prefix, pwd)
	}
	name := strings.TrimSpace(string(b))
	if err := os.Remove(ptr); err != nil {
		return "", err
	}
	_ = RemoveMapping(confdir, pwd, "pointer")
	return name, nil
}
```

(Verify `RemoveMapping`'s exact signature at mapping.go:134 — adapt the call; it's best-effort, hence the discarded error.)

Create `cmd/unlink.go`:

```go
package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/slamb2k/azrl/internal/provider"
	"github.com/spf13/cobra"
)

// runUnlink removes the cwd's directory→profile link for one provider. The
// profile itself — tokens and all — is untouched; only the edge dies.
func runUnlink(providerName string, out io.Writer) error {
	pwd, _ := os.Getwd()
	for _, p := range provider.All() {
		if p.Name() != providerName {
			continue
		}
		name, err := p.Scheme().Unlink(p.ProfilesDir(), pwd)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: unlinked %s from %s (profile kept)\n", providerName, pwd, name)
		return nil
	}
	return fmt.Errorf("azrl: unknown provider %q", providerName)
}

func newUnlinkCmd(providerName, short string) *cobra.Command {
	return &cobra.Command{
		Use:          "unlink",
		Short:        short,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnlink(providerName, cmd.OutOrStdout())
		},
	}
}

func init() {
	RootCmd.AddCommand(newUnlinkCmd("azure", "Remove this directory's Azure profile link (keeps the profile)"))
}
```

(Check the provider registry exposes `Scheme()` — the `Provider` interface does; if the azure provider isn't in `All()` with a Scheme, adapt via `profile.AzureScheme()` directly for the azure case.) Register `newUnlinkCmd("github"|"aws"|"gcp", …)` in the three subcommand slices.

- [ ] **Step 4: GREEN** — `go test ./internal/profile/ ./cmd/ -count=1` PASS.

- [ ] **Step 5: gates + commit**

```bash
go build ./... && go test ./... && gofmt -l .
git add internal/profile/ cmd/
git commit -m "feat(cli): azrl unlink — remove a directory's link, keep the profile"
```

---

### Task 2: `login --no-link` (create without linking)

**Files:**
- Modify: `cmd/login.go` (flag + thread to create path), `cmd/flow.go:65` (`captureSession` gains `link bool`), `cmd/init.go:30-49` (`runAzureInit` threads it), `cmd/capture.go` (passes `true`), `cmd/gh.go:68-79` / `cmd/aws.go:102-111` / `cmd/gcp.go:109-118` (pin-on-create guarded)
- Test: `cmd/login_test.go` or `cmd/flow_test.go` (append; follow whichever holds the create-path tests), `cmd/gh_test.go` (append)

**Interfaces:**
- Produces: `--no-link` flag on `azrl login`, `azrl gh|aws|gcp login`; `captureSession(name, pwd string, link bool, out io.Writer) error` (updated signature — update ALL callers: cmd/capture.go passes `link=true`).
- Consumes: existing create paths.

- [ ] **Step 1: Write the failing tests**

The pattern: azure create-path tests live around `captureSession`/login (find the existing test that asserts `.azprofile` written on create — grep `azprofile` in cmd tests) and gh login tests shim `gh` on PATH. Add:

```go
// azure: --no-link creates the profile without writing .azprofile.
func TestLoginNoLinkSkipsPointer(t *testing.T) {
	// Mirror the existing azure create-path test's seeding/shims exactly
	// (fake az on PATH etc). After: azrl login newprof --yes --no-link
	//   - <confdir>/newprof.conf exists
	//   - <pwd>/.azprofile does NOT exist
}

// gh: created-but-no-link skips prov.Use (no .ghprofile in pwd).
func TestGhLoginNoLinkSkipsPin(t *testing.T) {
	// Mirror the existing gh login create test's shims. After:
	// azrl gh login neu --no-link → conf written, no .ghprofile in pwd.
}
```

**Implementer note:** these are behavioral skeletons — read the existing create-path tests first (`cmd/login_test.go`, `cmd/gh_test.go`), copy their seeding/shim scaffolding verbatim, and pin exactly the two assertions each. The RED state is the unknown `--no-link` flag erroring.

- [ ] **Step 2: RED** — the new tests fail (`unknown flag: --no-link`).

- [ ] **Step 3: Implement**

- `captureSession(name, pwd string, link bool, out io.Writer)`: wrap the `os.WriteFile(filepath.Join(pwd, ".azprofile")…)` block (flow.go:84-86) in `if link { … }`. `Touch` still runs (it only records a mapping when a pointer names the profile — with no pointer it's just LAST_*). Update callers: `cmd/capture.go` → `true`; `runAzureInit` gains a `link bool` param threaded from the login flag.
- `cmd/login.go`: `var loginNoLink bool` + `loginCmd.Flags().BoolVar(&loginNoLink, "no-link", false, "create without linking this directory")`; pass `!loginNoLink` into the create path. Existing-profile sign-in ignores the flag (it never links anyway).
- gh/aws/gcp login: add the same flag per command; guard becomes `if created && !noLink { prov.Use(...) }`.

- [ ] **Step 4: GREEN** — full cmd suite; the pre-existing create tests (which don't pass the flag) must still see the pointer written.

- [ ] **Step 5: gates + commit**

```bash
git add cmd/
git commit -m "feat(cli): login --no-link — create a profile without claiming the directory"
```

---

### Task 3: `rm` link-awareness (`--unlink-all` / `--replace <name>`)

**Files:**
- Modify: `internal/profile/scheme.go` (append helpers), `cmd/rm.go`, `cmd/gh.go`/`cmd/aws.go`/`cmd/gcp.go` (rm commands)
- Test: `internal/profile/scheme_test.go`, `cmd/manage_test.go` (append)

**Interfaces:**
- Produces:
  - `Scheme.LinkedDirs(confdir, name string) []string` — dirs whose revalidated pointer-mapping names the profile.
  - `Scheme.UnlinkAll(confdir, name string) ([]string, error)` — removes each linked dir's pointer file + mapping row; returns the dirs.
  - `Scheme.ReplaceLinks(confdir, oldName, newName string) ([]string, error)` — errors unless `<newName>.conf` exists; rewrites each linked dir's pointer file to newName and heals the mapping rows (write pointer, then `RecordMapping{Dir, newName, "pointer"}`).
  - `rm` flags `--unlink-all` and `--replace <name>` on all four rm commands; bare `rm` with links refuses, listing the dirs and both flags.
- Consumes: `ReadMappings` (revalidating), `RemoveMapping`, `RecordMapping`, pointer writes as in `Use`.

Task 6 (TUI Delete) reuses `LinkedDirs`/`UnlinkAll`/`ReplaceLinks`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/profile/scheme_test.go`:

```go
func TestLinkedDirsUnlinkAllAndReplace(t *testing.T) {
	confdir := t.TempDir()
	d1, d2 := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	os.WriteFile(filepath.Join(confdir, "other.conf"), []byte("AZ_TENANT=other.com\n"), 0o644)
	s := AzureScheme()
	s.Use("acme", confdir, d1)
	s.Use("acme", confdir, d2)

	dirs := s.LinkedDirs(confdir, "acme")
	if len(dirs) != 2 {
		t.Fatalf("LinkedDirs = %v", dirs)
	}

	if _, err := s.ReplaceLinks(confdir, "acme", "missing"); err == nil {
		t.Fatal("replace with a nonexistent profile must error")
	}
	if _, err := s.ReplaceLinks(confdir, "acme", "other"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(d1, ".azprofile")); strings.TrimSpace(string(b)) != "other" {
		t.Fatalf("d1 pointer not replaced: %q", b)
	}
	if got := s.LinkedDirs(confdir, "other"); len(got) != 2 {
		t.Fatalf("mappings should follow the replace: %v", got)
	}

	if _, err := s.UnlinkAll(confdir, "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d2, ".azprofile")); !os.IsNotExist(err) {
		t.Fatal("UnlinkAll should remove pointer files")
	}
	if got := s.LinkedDirs(confdir, "other"); len(got) != 0 {
		t.Fatalf("mappings should be gone: %v", got)
	}
}
```

Append to `cmd/manage_test.go`:

```go
func TestRmRefusesWhileLinkedThenUnlinkAll(t *testing.T) {
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
	RootCmd.SetArgs([]string{"rm", "acme", "-y"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatal("rm must refuse while directories link to the profile")
	}
	RootCmd.SetArgs([]string{"rm", "acme", "-y", "--unlink-all"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); !os.IsNotExist(err) {
		t.Fatal("--unlink-all should remove the linked dir's pointer")
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); !os.IsNotExist(err) {
		t.Fatal("profile should be deleted")
	}
}
```

(Reset any package-level rm flag vars between tests the way `resetAwsCaptureFlags` does, if rm uses package vars — check how cmd/rm.go holds its `-y` flag today and mirror.)

- [ ] **Step 2: RED**; **Step 3: Implement**

Scheme helpers (scheme.go, append):

```go
// LinkedDirs lists the directories whose pointer-sourced mappings name the
// profile — the blast radius of deleting or re-signing it.
func (s Scheme) LinkedDirs(confdir, name string) []string {
	var dirs []string
	for _, m := range s.ReadMappings(confdir) {
		if m.Profile == name && m.Source == "pointer" {
			dirs = append(dirs, m.Dir)
		}
	}
	return dirs
}

// UnlinkAll removes every linked directory's pointer file and mapping row.
func (s Scheme) UnlinkAll(confdir, name string) ([]string, error) {
	dirs := s.LinkedDirs(confdir, name)
	for _, d := range dirs {
		if err := os.Remove(filepath.Join(d, s.Pointer)); err != nil && !os.IsNotExist(err) {
			return dirs, err
		}
		_ = RemoveMapping(confdir, d, "pointer")
	}
	return dirs, nil
}

// ReplaceLinks repoints every linked directory at another profile — an edge
// rewrite only; provider-native extras (git credential setup, config sync)
// happen on the next use/login as usual.
func (s Scheme) ReplaceLinks(confdir, oldName, newName string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(confdir, newName+".conf")); err != nil {
		return nil, fmt.Errorf("%s: no such profile %q to replace links with", s.Prefix, newName)
	}
	dirs := s.LinkedDirs(confdir, oldName)
	for _, d := range dirs {
		if err := os.WriteFile(filepath.Join(d, s.Pointer), []byte(newName+"\n"), 0o644); err != nil {
			return dirs, err
		}
		_ = RecordMapping(confdir, Mapping{Dir: d, Profile: newName, Source: "pointer"})
	}
	return dirs, nil
}
```

`cmd/rm.go` (and the three group rm commands): add `--unlink-all` / `--replace <name>` flags. Before removal: `dirs := scheme.LinkedDirs(confdir, name)`; if `len(dirs) > 0` and neither flag → error listing each dir plus `use --unlink-all to remove the links, or --replace <profile> to repoint them`. With `--unlink-all` → `UnlinkAll` first; with `--replace` → `ReplaceLinks` first. Then the existing Remove path. (For the group commands, get the scheme via `prov.Scheme()`.)

- [ ] **Step 4: GREEN** (full suite — the pre-existing `TestRmCmdYes` must still pass: it has no links). **Step 5: gates + commit**

```bash
git add internal/profile/ cmd/
git commit -m "feat(cli): rm is link-aware — refuse, --unlink-all, or --replace"
```

---

### Task 4: Tabs become persona-only (drop Link here; relabels)

**Files:**
- Modify: `internal/ui/provider_view.go` (`providerActions`, `enabledActions` `u` branch, delete `useAction`), `internal/ui/tabs.go` (helpOverlay)
- Test: `internal/ui/aws_view_test.go` (counts + Link-here tests), others per grep

**Interfaces:**
- Produces: everyday ACTIONS = 5 (`s/t/c/b/delete`); labels `Assign browser…`, `Delete…`. The dashboard's row-`u` is untouched.
- Consumes: nothing new.

- [ ] **Step 1: Update/write the failing tests**

- `TestLinkHereDisabledWhenAlreadyLinked` (aws_view_test.go:351-379): repurpose → `TestLinkHereAbsentFromTabs`: assert the everyday list is `ACTIONS (5)` and contains neither `Link here` nor `already linked here`.
- `TestCaptureAbsentFromNonEmptyActionList` (aws_view_test.go:331): `ACTIONS (7)` → `ACTIONS (5)`.
- Add assertions (same file or the azure view test): `Assign browser…` and `Delete…` render; `Browser profile` and `Remove…` don't.
- Grep for other tests asserting `"Link here"`, `"Browser profile"`, `"Remove…"`, `ACTIONS (7)` and adjust minimally (report each).

- [ ] **Step 2: RED**; **Step 3: Implement**

In `providerActions`: delete the `u` entry; relabel `b` → `Assign browser…`; relabel `delete` → `Delete…`. Keep `n`/`a` entries for now (bootstrap + accelerators — Task 5 restructures `n`). In `enabledActions`: drop the `case "u":` disable branch. Delete `useAction` (tabs-only user). helpOverlay: reword the `u` entry to make its home explicit (e.g. `u · link here (dashboard)`), keep everything else.

- [ ] **Step 4: GREEN**; **Step 5: gates + commit**

```bash
git add internal/ui/
git commit -m "feat(ui): tabs manage personas — Link here moves to the dashboard"
```

---

### Task 5: New profile becomes the PROFILES pane's first row (create, no link)

**Files:**
- Modify: `internal/ui/frame.go` (`renderProfilePane` synthetic row), `internal/ui/provider_view.go` (cursor semantics, `selected()`, DETAILS blurb, enter/click on row 0, naming argv, empty-state copy, `n` accelerator retained, remove `n` from ACTIONS radio)
- Test: `internal/ui/provider_view_test.go`-family (new file or append per existing layout)

**Interfaces:**
- Produces: PROFILES pane row 0 is always `＋ New profile…`; profiles occupy rows 1..n; `selected()` returns `""` on row 0; DETAILS for row 0 shows the entity blurb; `enter`/click on row 0 (and the `n` key anywhere) opens the naming prompt; create argv becomes `login <name> --yes --no-link`.
- Consumes: `--no-link` (Task 2), naming machinery, `prof:` zone marks.

**Behaviors to pin (tests first — adapt scaffolding to the existing view-test idioms, these behaviors are the requirements):**

1. `＋ New profile…` renders as the first PROFILES row on a populated tab AND on an empty tab; the pane count still reads the real profile count (`PROFILES (2)` for 2 profiles).
2. Cursor row 0 + `enter` opens the naming prompt; typing name + enter execs `login <name> --yes --no-link` (assert via the same naming-prompt test pattern as `TestNewProfilePromptsForNameThenExecsCreate` — the cmd is non-nil; if that test asserts argv via a seam, mirror it; otherwise non-nil + prompt-closed is the established assertion).
3. Cursor row 0: DETAILS pane shows `A profile is a container for tokens and intention — sign in once, link it to any number of directories.` and the ACTIONS radio shows no persona actions (or all disabled with `select a profile first` — pick whichever the never-hide model expresses more cleanly; report the choice).
4. Persona accelerators (`s/t/c/b/delete`) on row 0 → status `select a profile first`, no dispatch.
5. Naming prompt hint for create reads `token container + sign-in — link it later`.
6. `n` from anywhere on the tab still opens the prompt; the ACTIONS radio no longer lists New profile (everyday remains 5; empty-state ACTIONS = 1, Capture only — update the `(none yet — …)` line to point at the row: `(no profiles — ↵ on ＋ New profile creates, a adopts a live session)`).
7. Mouse: row 0 gets a zone (`prof:+new` or similar); click selects it, click-again opens the prompt (mirror clickProfile semantics).
8. Keyboard `up` from row 0 still hands focus to the tab bar (the existing top-of-list behavior moves with the new top row).

**Implementation guidance:** introduce the offset at the view level — keep `v.profiles` untouched and let `v.cursor` range over `[0..len(profiles)]` with `selected()` mapping `cursor-1`; `renderProfilePane` takes a `withNewRow bool` (or the row is prepended by the caller). Audit every `v.cursor`/`len(v.profiles)` touchpoint (reset :150, nav :362/:374, wheel :458, scopes map, `clickProfile`, `switchTabMsg` pre-select) — the exploration notes list them. This is the invasive task: run the FULL ui suite frequently.

- [ ] **Steps:** tests (RED) → implement → GREEN → gates + commit

```bash
git add internal/ui/
git commit -m "feat(ui): New profile lives atop the PROFILES pane — create signs in, links later"
```

---

### Task 6: Delete-with-links confirm (Unlink all / Replace with…)

**Files:**
- Modify: `internal/ui/provider_view.go` (`removeAction`, confirm key handling, `doRemove` variants, confirm pane render)
- Test: view tests (append)

**Interfaces:**
- Produces: Delete confirm for a linked profile shows the dirs (first 3 + `+ n more`) and a radio: `Cancel` / `Unlink N dir(s) + delete` / `Replace links with…` (disabled with `no other profile to point them at` when none). Replace flows to a second radio listing the provider's other profiles; choosing one runs `ReplaceLinks` then `Remove`. Unlink-all runs `UnlinkAll` then `Remove`. Unlinked profiles keep today's two-option confirm.
- Consumes: `Scheme.LinkedDirs/UnlinkAll/ReplaceLinks` (Task 3), the radio machinery (`radioOption.disabled` exists), `v.mappingDirs` (already populated in reload).

**Behaviors to pin:**
1. Deleting a profile with 2 links shows both dirs in the confirm text and the three options; `Unlink 2 dirs + delete` removes both pointer files, the mapping rows, and the profile.
2. `Replace links with…` on a provider with another profile opens the picker; choosing it rewrites both pointers to the other profile and deletes the original.
3. With no other profile, Replace is disabled with the exact reason.
4. A link-free profile still gets the simple No/Yes confirm.
5. `esc` cancels at both levels; the `y` fast-path maps to Unlink-all only when links exist (or is dropped for the 3-option dialog — pick, report; the hardcoded `cursor == 1` check must become cursor-switch either way).

- [ ] **Steps:** tests (RED) → implement → GREEN → gates + commit

```bash
git add internal/ui/
git commit -m "feat(ui): Delete is link-aware — unlink all or repoint before removal"
```

---

### Task 7: Dashboard `U` Unlink

**Files:**
- Modify: `internal/ui/dashboard.go` (Update `U` case, footer chip)
- Test: `internal/ui/dashboard_test.go` (append)

**Interfaces:**
- Produces: `U` on a mapping row whose `Dir` is the cwd → `Scheme.Unlink` in-process (via `m.providers` match → `p.Scheme().Unlink(p.ProfilesDir(), cwd)`), status `✓ unlinked <dir> (profile kept)`, reload. Mapping row for another dir → status `links are removed where they live — run unlink in <dir>`. Ambient/unmapped rows → status `no directory link on this row`. Footer chip becomes `s/t/c/u/b/U`.
- Consumes: Task 1's `Scheme.Unlink`; dashItem/ov row correlation (the flat index maps rows — a mapping row's Dir comes from `m.ov.Mappings[i]` for `i < len(m.ov.Mappings)`).

**Behaviors to pin:** the three cases above plus: `U` while naming is ignored; help overlay gains `U · unlink (dashboard)`.

- [ ] **Steps:** tests (RED) → implement → GREEN → gates + commit

```bash
git add internal/ui/
git commit -m "feat(ui): dashboard U unlinks the cwd's governing link"
```

---

### Task 8: Docs + manual-verify + final verification

**Files:**
- Modify: `README.md` (keymap/marks table: new 5-action set + pane row + dashboard `U`; CLI sections: `unlink`, `login --no-link`, `rm --unlink-all/--replace`; the profile mental-model blurb — add a short "The model" paragraph near the top of Usage: profiles are containers for tokens and intention; links are per-directory pointers; many dirs per profile), `CLAUDE.md` (cmd/ + internal/ui bullets updated for all of the above; fix any sentence this branch made false — several will be: Link here on tabs, `n` in ACTIONS, Remove… label, rm semantics), `specs/tui-ux-redesign.manual-verify.md` (entity-model section: delete-with-links round-trip, unlink refusal on parent, no-link create then dashboard link)

- [ ] Make the edits in the files' voice; run `go build ./... && go test ./... && gofmt -l .` + `git diff main --stat`; commit:

```bash
git add README.md CLAUDE.md specs/tui-ux-redesign.manual-verify.md
git commit -m "docs: entity model — personas on tabs, edges on the dashboard, link-aware delete"
```

---

## Post-plan checklist (for the executor)

- Surface the six **Recorded assumptions** with the final report.
- Final whole-branch review (fable) before shipping; pay attention to: the cursor-offset audit in Task 5 (every `len(v.profiles)` touchpoint), mapping-row consistency after Replace/UnlinkAll, and that CLI pin-on-create defaults are byte-identical for users not passing `--no-link`. Ship via /ship.
