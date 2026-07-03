---
title: Resolution Strategies & Mapping-First TUI — read-through ambient identity, native-first directory associations, and a mappings landing view
version: 1.0
date_created: 2026-07-02
last_updated: 2026-07-02
tags: [architecture, design, app, tool, tui, breaking]
---

# Introduction

azrl currently duplicates state the underlying provider CLIs already own: `ghrl switch`
persists a "default profile" to `~/.github-profiles/.current` that nothing operational
consults (only the `ghrl status` display reads it), while every provider CLI already has
its own native default mechanism. Meanwhile azrl's directory associations are expressed
only through azrl-owned pointer files, even where a provider has a native, more
authoritative mechanism (git's repo-local `credential.<host>.username`, which azrl itself
writes in `SetupRepo` but never reads back).

This spec replaces persisted defaults with **read-through of native ambient state**,
makes directory association **strategy-based (native-first)**, introduces a **central
mapping index** so azrl can enumerate its mappings, and restructures the TUI landing
view around three sections: **MAPPINGS → AMBIENT → UNMAPPED PROFILES**. It removes the
`switch` verb and the `.current` file (breaking) and reshapes `azrl status --json`
(breaking). Target release: **v0.7.0**.

## 1. Purpose & Scope

**Purpose.** Make azrl's identity model honest and minimal: azrl owns *mappings*
(name → provider state), *guardrails* (expected tenant/account), *isolation*
(per-profile config dirs), and the *browser bridge*. azrl never stores or mutates a
default identity; it reads and displays the provider's own ambient state.

**In scope**
- Remove `switch` (gh group + promoted ghrl top-level) and the `.current` file.
- New disk-only `Ambient()` on the `Provider` interface + per-provider readers.
- Central per-provider mapping index; write-side hooks; prune/self-heal on read.
- GitHub association read-back from repo-local git config, with conflict handling.
- TUI landing view restructure (three sections, scope markers, adopt action).
- `azrl status` CLI parity, including a reshaped `--json`.

**Out of scope (non-goals)**
- Scanning filesystem roots to discover hand-made pointer files (future work; the
  index self-heals when azrl touches a directory).
- Mutating any provider's native default (no wrapper for `gh auth switch`,
  `az account set`, `gcloud config configurations activate`, or `AWS_PROFILE`).
- A `switch`-equivalent for AWS/GCP.
- New GHES/multi-host handling beyond what the GitHub provider does today.
- Any change to the login/bridge flows shipped through v0.6.0.

**Audience.** The `/build` pipeline and reviewers of this repo. Assumes the codebase
as of `main` @ v0.6.0 (`login` unified, `init` removed, providerTabView shared,
fsnotify dashboard, `WatchDirs()`, per-provider `Status()`).

## 2. Definitions

- **Ambient identity** — the account a provider CLI would use *right now* with no
  azrl involvement: whatever its native default state + the current process
  environment selects.
- **Mapping** — an explicit association between a directory and a profile, via an
  azrl pointer file (`.azprofile`/`.ghprofile`/`.awsprofile`/`.gcpprofile`) or a
  native mechanism (repo-local git config).
- **Mapping index** — a per-provider file listing known mappings so the TUI/CLI can
  enumerate them without scanning the filesystem.
- **Managed identity** — an identity that matches a saved azrl profile.
  **Unmanaged** — one that does not (rendered distinctly; adoptable).
- **Adopt** — create a profile from an unmanaged identity via the provider's
  existing `capture` flow.
- **Scope (relative to cwd)** — whether a mapping governs the directory the TUI/CLI
  was launched from: set in this dir (●), inherited from an ancestor (↑), repo-local
  git config ((git)), or not governing the cwd at all (neutral row). The ambient
  section is the global (🌐) fallback.
- **Drift** — a mapped directory whose ambient environment (in the azrl process)
  selects something other than the mapping. Only meaningful inside mapped dirs.

## 3. Requirements, Constraints & Guidelines

### Principles
- **PAT-001**: azrl wraps a provider verb only where it adds the browser bridge or
  profile isolation. Everything else is read-only observation of native state.
- **PAT-002**: azrl never persists a default identity and never mutates native
  default state as a side effect (the GCP `--no-activate` precedent is binding).
- **PAT-003**: Native mechanisms outrank azrl mechanisms when both express the same
  fact (git config beats pointer file inside a repo).

### Removal of switch/.current (breaking)
- **REQ-001**: Delete `newGhSwitchCmd`, `github.Switch`, `github.Current`, and all
  reads of `~/.github-profiles/.current`. A stale `.current` file is silently
  ignored (and may be pruned when the profiles dir is next written).
- **REQ-002**: Register a hidden deprecated `switch` stub (gh group and the promoted
  ghrl top level) that runs nothing and returns:
  `ghrl: 'switch' was removed — the default account is whatever gh itself is signed
  into; use 'gh auth switch', or map a directory with 'ghrl use <name>'`.
  Same pattern as the removed `init` stub.

### Ambient identity (read-through parachute)
- **REQ-010**: Add to `internal/provider.Provider`:
  `Ambient() (Ambient, error)` — disk + process-env only; MUST NOT spawn a CLI or
  touch the network; best-effort (zero value, nil error on missing/unparseable state).
- **REQ-011**: Per-provider sources (env consulted first where the CLI honors it,
  and the winning source recorded):
  - Azure: `${AZURE_CONFIG_DIR:-~/.azure}/azureProfile.json` (strip UTF-8 BOM),
    default subscription's `user.name` (fallback `tenantId`).
  - GitHub: `${GH_CONFIG_DIR:-~/.config/gh}/hosts.yml` — the active user per host;
    support both the legacy single-`user` shape and the modern multi-account shape
    (top-level `user:` / `users:` map). Identity rendered `user@host`.
  - AWS: `AWS_PROFILE` env if set, else the `[default]` profile in
    `${AWS_CONFIG_FILE:-~/.aws/config}`; enrich with account/role from the SSO
    cache when resolvable, else the profile name alone.
  - GCP: `CLOUDSDK_ACTIVE_CONFIG_NAME` env, else
    `${CLOUDSDK_CONFIG:-~/.config/gcloud}/active_config`; identity from
    `configurations/config_<name>` `[core] account`.
- **REQ-012**: `Ambient` carries at minimum: `Identity string`,
  `Source string` (e.g. `env:AWS_PROFILE`, `file:~/.config/gh/hosts.yml`).
  A shared helper (not the provider) reverse-maps identity → managed profile by
  comparing against saved profiles' conf/Status identities; no match ⇒ unmanaged.
- **CON-001**: Ambient is display/status only. It is never used to choose a login
  target and never written.

### Mapping index
- **REQ-020**: Each provider's profiles dir gains a `mappings` index file. One line
  per mapping: `<abs-dir>\t<profile>\t<source>` with `source ∈ {pointer, gitconfig}`.
  Written atomically (existing order-preserving `writeAtomic` pattern).
- **REQ-021**: Write-side hooks append/update the index whenever azrl creates or
  confirms a mapping: `use` (pointer write and, for GitHub, `SetupRepo`'s git-config
  write), `login`/`capture` pointer writes, and `Scheme.Touch`'s `LAST_DIR` path.
- **REQ-022**: Read-side prune + self-heal: on every index read, drop entries whose
  directory no longer exists or whose pointer/git-config no longer names that
  profile; whenever azrl resolves a mapping in a directory absent from the index
  (any command), add it. Hand-made pointers therefore appear after first contact.
- **CON-002**: No recursive filesystem scanning. The index is the only enumeration
  source in v1.

### Directory association, native-first (GitHub)
- **REQ-030**: GitHub association resolution becomes: repo-local
  `git config credential.https://<host>.username` (mapped to the profile whose
  conf `GH_USER`+`GH_HOST` match) → `.ghprofile` walk-up → none. Azure/AWS/GCP
  remain pointer-only (no native dir mechanism exists).
- **REQ-031**: When git config and a `.ghprofile` disagree in the same repo, the
  git config wins (it is what `git push` obeys) and the row renders with a warning
  marker showing both. A git-config username matching no profile is an unmanaged
  mapping (adoptable).

### TUI landing view
- **REQ-040**: The landing tab (bare `azrl`, tab 0) becomes three sections, in
  order: `MAPPINGS`, `AMBIENT`, `UNMAPPED PROFILES`. It inherits all current
  dashboard behavior: fsnotify + `DASHBOARD_POLL_SECS` tick re-aggregation,
  `[r]` refresh, `[w]` drift recheck, Enter drill-through to the provider tab with
  the profile preselected, width invariant, fill/centered layout.
- **REQ-041**: MAPPINGS rows: scope marker (● cwd / ↑ ancestor / neutral) + source
  icon (pointer vs `(git)`) + dir → `provider:profile` + drift marker. Managed
  identities render normally; unmanaged in a distinct accent with a one-key
  `[a]dopt` action that launches the provider's capture flow (name prompt
  defaulting to the directory name, consistent with first-login).
- **REQ-042**: AMBIENT rows: 🌐 per provider — identity, winning source, and either
  the matching profile name or an explicit `unmanaged` label. No drift on these rows.
- **REQ-043**: UNMAPPED PROFILES: muted one-liners (`provider:name · identity ·
  expiry`) for saved profiles that appear in no mapping — so expiry warnings stay
  visible. Enter drills through like any profile row.
- **REQ-044**: Per-provider tabs remain the management surface, unchanged in role.
  `WatchDirs()` gains the native ambient files (e.g. `~/.config/gh`) so ambient
  rows live-update too.

### CLI parity (breaking JSON)
- **REQ-050**: `azrl status` renders the same three sections in plain text.
  `--json` becomes `{"mappings":[…],"ambient":[…],"unmapped":[…]}` — mappings:
  `{dir, provider, profile, source, scope, drifted}`; ambient:
  `{provider, identity, source, profile|null}`; unmapped: the existing per-profile
  status shape. Document the shape change in the release notes.

### Guidelines
- **GUD-001**: All new reads are best-effort and non-fatal — a broken hosts.yml or
  index line never errors the TUI/status; it degrades to blank/skip.
- **GUD-002**: Reuse existing helpers: BOM-aware JSON read (azure), yaml.v3
  (github), `provider.Drifted`, `writeAtomic`, `profile.DefaultName`,
  `confirmCreateProfile` conventions.
- **GUD-003**: No new dependencies expected (yaml.v3, fsnotify, go-isatty already
  present).

## 4. Interfaces & Data Contracts

```go
// internal/provider — additive interface change (all four providers implement).
type Ambient struct {
    Identity string // "" when none/unreadable
    Source   string // "env:AWS_PROFILE" | "file:~/.config/gh/hosts.yml" | ...
}
// Added to Provider: disk+env only; no network, no CLI spawn; best-effort.
Ambient() (Ambient, error)

// internal/provider (or internal/profile) — shared mapping index helpers.
type Mapping struct {
    Dir     string // absolute
    Profile string
    Source  string // "pointer" | "gitconfig"
}
func ReadMappings(profilesDir string) []Mapping        // prunes stale lines on read
func RecordMapping(profilesDir string, m Mapping) error // append/update, atomic
```

Index file example (`~/.github-profiles/mappings`):

```
/home/slamb2k/work/azrl	work	gitconfig
/home/slamb2k/work	work	pointer
/home/slamb2k/oss/foo	personal	pointer
```

`azrl status --json` (new shape):

```json
{
  "mappings": [
    {"dir": "/home/u/work/azrl", "provider": "github", "profile": "work",
     "source": "gitconfig", "scope": "cwd", "drifted": false}
  ],
  "ambient": [
    {"provider": "azure", "identity": "simon@contoso.com",
     "source": "file:~/.azure/azureProfile.json", "profile": null}
  ],
  "unmapped": [
    {"provider": "azure", "profileName": "fiig", "identity": "…",
     "expiry": "2026-06-23T10:19:55Z"}
  ]
}
```

Contract suite: `providertest.RunContract` gains an `Ambient()` exercise under the
existing `shimNoNetwork` fakes (sentinel must never be touched; call must not
panic or hang). Additive only — existing assertions unchanged.

## 5. Acceptance Criteria

- **AC-001**: Given any state, when `ghrl switch work` runs, then it exits non-zero
  with the removal guidance and mutates nothing; `switch` appears in no help output.
- **AC-002**: Given a stale `.current` file, when any gh command runs, then behavior
  is identical to the file being absent.
- **AC-003**: Given fake native state (fixture `hosts.yml`/`azureProfile.json`/
  `~/.aws/config`/`active_config` + env), when `Ambient()` is called, then it
  returns the expected identity+source without spawning any CLI (no-network
  contract passes for all four providers).
- **AC-004**: Given `AWS_PROFILE=x` in the environment and a different `[default]`
  on disk, when ambient is shown, then `x` wins and the source says `env:AWS_PROFILE`.
- **AC-005**: Given `azrl use work` runs in a dir, when the landing view opens,
  then a MAPPINGS row for that dir exists (index recorded) with source `pointer`
  (and `gitconfig` additionally for GitHub `use`).
- **AC-006**: Given an index entry whose directory was deleted, when the index is
  read, then the entry is gone from both display and file.
- **AC-007**: Given a hand-written `.azprofile` in a dir unknown to the index, when
  any azrl command resolves in that dir, then the mapping joins the index.
- **AC-008**: Given a repo whose git config names user A and whose `.ghprofile`
  names profile-of-user-B, when mappings render, then A's mapping wins and the row
  carries a conflict warning showing both.
- **AC-009**: Given the TUI launched from a dir governed by an ancestor's pointer,
  then that row shows ↑; from the pointer's own dir, ●; ambient rows show 🌐.
- **AC-010**: Given an unmanaged git-config identity, when `[a]` is pressed on its
  row, then the provider capture flow starts with the name defaulting to the dir
  name; on completion the row re-renders as managed.
- **AC-011**: Given a saved profile with no mapping, then it appears (muted, with
  expiry) in UNMAPPED PROFILES and nowhere in MAPPINGS.
- **AC-012**: `azrl status --json` emits exactly the three-section shape above on
  stdout; the plain table shows the same three sections.
- **AC-013**: All existing invariants hold: `TestTabsWidthInvariant` across widths
  on all tabs; fsnotify re-aggregation; `go test -race ./internal/ui/` clean;
  `providertest.RunContract` passes for all four providers.

## 6. Test Automation Strategy

- **Levels**: unit + shimmed integration (no real CLIs), TUI model tests.
- **Patterns** (established in this repo — follow exactly):
  - PATH-shim fakes via `t.Setenv("PATH", tmpDir)`; `shimNoNetwork` sentinel for
    the contract `Ambient()` addition.
  - Temp `HOME` + fixture files for native state: `hosts.yml` (both single-user
    and multi-account shapes), BOM-prefixed `azureProfile.json`, `~/.aws/config`
    with `[default]`, gcloud `active_config` + `config_<name>`; `t.Setenv` for the
    env-wins cases.
  - Index tests: record/read round-trip, prune-on-missing-dir, self-heal-on-touch,
    atomic rewrite preserves unrelated lines.
  - Git-config read-back: real `git` in a temp repo (git is already a test dep for
    github tests) setting `credential.<host>.username`; conflict case with a
    disagreeing `.ghprofile`.
  - TUI: `View()` assertions per section, synthetic `fsEventMsg`/tick, adopt-key
    dispatch, scope markers relative to a `t.Chdir` cwd, width invariant table.
  - CLI: `status` stdout capture (real-fd pattern from v0.5.x) + `--json`
    unmarshal-and-assert.
- **CI**: existing `ci.yml` (build/test/gofmt/vet + race where configured).
- **Coverage**: every REQ has at least one test; no numeric threshold imposed.

## 7. Rationale & Context

- `.current` was written by `switch` but consulted only by the `status` display —
  a persisted default that never took effect. Every provider CLI already owns a
  real default (`gh auth switch`, `~/.azure`, `AWS_PROFILE`/`[default]`,
  `active_config`); duplicating it created a second source of truth that drifted
  from reality. Reading native state through is always correct.
- The wrap rule (PAT-001) is the session's distilled principle: azrl's login
  wrappers are justified by the headless browser bridge and per-profile isolation;
  a `switch` wrapper adds neither.
- Git's repo-local `credential.username` — which azrl already writes — is more
  authoritative than `.ghprofile` for what `git push` will actually do; reading it
  back makes the display match ground truth (PAT-003).
- The index exists because "show all mappings" is otherwise unanswerable without
  filesystem scanning; write-side hooks make it complete for azrl-made mappings
  and self-healing for hand-made ones (CON-002 keeps v1 bounded).
- Scope markers turn the mappings table from "a list of pointer files" into "why
  does this identity apply *here*" — the question the user actually asks.
- Per-provider tabs stay because action sets genuinely diverge (Azure: select
  subscription; AWS: refresh SSO; GCP: set project; GitHub: credential re-wiring)
  and the shared `providerTabView` makes them nearly free to keep.

## 8. Dependencies & External Integrations

### External Systems
- **EXT-001**: Native CLI state files: `~/.azure/azureProfile.json`,
  `~/.config/gh/hosts.yml`, `~/.aws/config`, `~/.config/gcloud/{active_config,
  configurations/}` — read-only; formats as observed (gh multi-account shape
  verified against a fixture, flagged for one real-machine check in manual-verify).
- **EXT-002**: `git` — repo-local config read (`credential.<host>.username`);
  already a dependency of the GitHub provider's `SetupRepo`.

### Infrastructure / Platform
- **PLT-001**: Go 1.24 toolchain; existing deps only (yaml.v3, fsnotify,
  go-isatty, bubbletea/lipgloss).

### Data
- **DAT-001**: New `mappings` index file per provider profiles dir (TSV, atomic
  writes, prunable, no secrets).

## 9. Examples & Edge Cases

Landing view sketch (90 cols, cwd = `~/work/azrl`):

```
 MAPPINGS
  ● ~/work/azrl          → github:work        (git)
  ↑ ~/work               → azure:velrada      .azprofile      ⚠ drift
    ~/clients/acme/api   → aws:acme-prod      .awsprofile
    ~/oss/foo            → github: simon-p@github.com (git)   unmanaged · [a]dopt
 AMBIENT — defaults in effect
  🌐 Azure   simon@contoso.com        ~/.azure                → velrada
  🌐 GitHub  simon-p@github.com       hosts.yml               unmanaged
  🌐 AWS     acme-prod                env:AWS_PROFILE         → acme-prod
 UNMAPPED PROFILES
  azure:fiig    Simon.Lamb@velrada.com    expired
```

Edge cases the implementation must handle:
- gh `hosts.yml` with multiple users per host (modern shape) vs the legacy single
  `user:` — pick the host's active user; unknown shape ⇒ blank, never error.
- Index line whose dir exists but whose pointer now names a different profile ⇒
  replaced on read (self-heal), not duplicated.
- Two pointer mappings on the cwd's ancestor chain ⇒ nearest wins (existing
  walk-up semantics); only the winning one gets ↑.
- Ambient identity matching *multiple* profiles (same account saved twice) ⇒ match
  the most-recently-used; do not error.
- Non-TTY `status` in a dir with mappings ⇒ same sections, no interactive elements.
- Empty everything (fresh machine) ⇒ MAPPINGS empty-state line, AMBIENT shows
  whatever native defaults exist, UNMAPPED empty; no panics at any width.

## 10. Validation Criteria

- `go build ./...`, `go test ./... -count=1`, `gofmt -l .` empty, `go vet ./...`
  clean, `go test -race ./internal/ui/` clean.
- All four providers pass `providertest.RunContract` including the new `Ambient()`
  no-network exercise.
- All acceptance criteria AC-001…AC-013 have passing tests.
- Suggested build phases (each ends green): ① remove switch/.current + stubs →
  ② `Ambient()` + readers + contract → ③ mapping index + hooks → ④ GitHub
  git-config read-back + conflict → ⑤ TUI landing restructure + adopt →
  ⑥ `status` parity + JSON → ⑦ docs (CLAUDE.md/README/release notes; append any
  real-machine checks to specs/multi-cloud-providers.manual-verify.md).
- Ships as **v0.7.0**; release notes call out the two breaking changes
  (`switch` removed; `status --json` reshaped).

## 11. Amendments

- **#79 (2026-07-03, v0.34.0)** — expiry is no longer rendered only on
  UNMAPPED PROFILES rows. Per `docs/ambient-identity-model.md` (expiry
  warnings attach to *mapped* profiles), MAPPINGS rows now carry the mapped
  profile's expiry too: an `⚠ expired` annotation (dashboard + plain
  `status`, alongside the drift/conflict markers) and an always-present
  `"expiry"` timestamp on `mappings` entries in `status --json`. The
  dashboard's next-action hint also fires for an expired profile whose pin
  governs the cwd, ranked conflict > drift > expired governing pin >
  unmanaged > expired unmapped > first-pin nudge. REQ-043/AC-011 still hold
  (unmapped rows keep their expiry text); only the "nowhere in MAPPINGS"
  exclusivity of expiry rendering is superseded.

## 12. Related Specifications / Further Reading

- `specs/status-dashboard.md` — the dashboard this view evolves (Phase 5.5).
- `specs/multi-cloud-providers.md` — AWS/GCP provider architecture (Phases 8–9).
- `specs/multi-cloud-providers.manual-verify.md` — real-machine spike checklist.
- CLAUDE.md — provider/Scheme/TUI architecture and testing conventions.
