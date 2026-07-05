---
title: Onboarding, Environment Detection & the `azrl setup` Wizard — one-host-or-zero config, auto-detected, interactively chosen
version: 1.0
date_created: 2026-07-06
last_updated: 2026-07-06
tags: [architecture, design, app, tool, tui, onboarding, config]
---

# Introduction

azrl's global config (`~/.azure-profiles/azrl.conf`) currently requires **three**
hand-edited values — `LOCAL_HOST`, `LOCAL_BROWSER_CMD`, `VM_HOST` — even though the two
host values name *different machines in opposite SSH directions* and, on any given
network, typically only **one** of them is usable. The shipped template is full of
placeholders (`LOCAL_HOST=my-laptop`, `VM_HOST=my-vm`); a user who installs and runs
`azrl login` without editing it gets no browser and a confusing "paste this on your
LOCAL machine" fallback, because the placeholder host is unreachable.

A **local mode** already landed this session (uncommitted on `main`): `Global.IsLocal()`
is true when `LOCAL_HOST ∈ {"", localhost, 127.0.0.1}`, `bridge.Bridge` short-circuits to
launch the browser on the same host with no SSH, config validation was relaxed so local
mode needs only `LOCAL_BROWSER_CMD`, and both installers detect WSL and seed
`LOCAL_BROWSER_CMD=wslview`. This spec **extends** that foundation into a complete
onboarding story.

This spec makes the config **minimal and self-configuring**: only one key is required
(`BROWSER_CMD`), the two host keys become optional, `VM_SSH_HOST` is auto-derived from
`$SSH_CONNECTION`, and a new pure `internal/envdetect` package plus an `azrl setup`
Bubble Tea wizard **detect the environment**, present a choice when ambiguous, pre-fill
sensible defaults, and let the user override every field. The installers stop
duplicating shell detection and call `azrl setup --yes`; bad/missing config auto-launches
the wizard at runtime. The config keys are **renamed** (role-based) with the old names
kept as read aliases for backward compatibility. Target release: **v0.8.0**.

## 1. Purpose & Scope

**Purpose.** Make azrl trivial to onboard on the two primary developer environments —
**WSL** (browser opens locally on Windows) and a **remote SSH VM** (browser opens on the
machine you're sitting at) — with the least possible configuration, detected
automatically, and overridable when the guess is wrong.

**In scope**
- Rename the three global keys to role-based names, reading the old names as aliases.
- Reduce required config to `BROWSER_CMD` only; make both host keys optional.
- Tighten `IsLocal()` so a `VM_SSH_HOST`-only box is treated as remote.
- Auto-derive the VM's SSH host from `$SSH_CONNECTION` for the path-A paste line.
- New pure, table-tested `internal/envdetect` producing ranked candidate configs.
- New `azrl setup` command: a Bubble Tea wizard (pick when ambiguous, pre-fill, override,
  back up + overwrite) with `--yes` and `--print` non-interactive modes.
- Replace the duplicated shell detection in `install.sh` and `scripts/install.sh` with a
  call to `azrl setup --yes`.
- Auto-launch the wizard (TTY) / print a hint (non-TTY) on missing/placeholder/invalid
  config.

**Out of scope (non-goals)**
- Renaming per-profile keys (`AZ_BROWSER_CMD`, `GH_BROWSER_CMD`, `AWS_BROWSER_CMD`,
  `GCP_BROWSER_CMD`, and the `*_BROWSER_LABEL` keys) — unchanged.
- Configuring SSH itself (key generation, `known_hosts`, `authorized_keys`, sshd).
- The documented GKE `gke-gcloud-auth-plugin` isolation gap.
- A native-Windows azrl build.
- Changing the login/bridge *protocol* (path A / path B mechanics) beyond making
  `VM_SSH_HOST` optional/derived and honoring local mode.

**Audience.** The `/build` pipeline and reviewers of this repo. Assumes `main` with the
uncommitted local-mode change already present (`IsLocal`, `bridge` local short-circuit,
relaxed validation, installer WSL seeding).

## 2. Definitions

- **Local mode** — azrl runs on the same machine as the browser; the OAuth callback loops
  back over `localhost` and no SSH bridge is used.
- **Remote mode** — azrl runs on a headless VM and the browser opens on the developer's
  machine, connected by an SSH tunnel (path B reverse tunnel, or path A paste).
- **Path B (zero-paste)** — the VM opens `ssh -R … BROWSER_HOST` and launches the browser
  on `BROWSER_HOST`. Needs the VM to be able to SSH *to* the dev machine.
- **Path A (paste)** — azrl prints a one-line `ssh -fNL … VM_SSH_HOST && BROWSER_CMD url`
  that the user runs on their dev machine. Needs the dev machine to be able to SSH *to*
  the VM.
- **Candidate** — a fully-formed config proposal produced by `envdetect` from environment
  signals, with a mode, pre-filled values, a human label, and a `Recommended` flag.
- **Placeholder config** — an `azrl.conf` still carrying the shipped sentinel values
  (`my-laptop` / `my-vm`), i.e. never configured.
- **Derived VM host** — the server IP azrl reads from `$SSH_CONNECTION` (field 3), used as
  the default `VM_SSH_HOST` for the paste line.

## 3. Requirements, Constraints & Guidelines

### Config keys (rename + alias)

- **REQ-01** `azrl.conf` uses role-based keys: `BROWSER_CMD`, `BROWSER_HOST`,
  `VM_SSH_HOST` (plus the unchanged `DASHBOARD_POLL_SECS`, `PROVIDERS`).
- **REQ-02** `LoadGlobal` reads each new key, falling back to its legacy alias when the
  new key is absent: `BROWSER_CMD`←`LOCAL_BROWSER_CMD`, `BROWSER_HOST`←`LOCAL_HOST`,
  `VM_SSH_HOST`←`VM_HOST`. New key wins if both present.
- **REQ-03** The `AZRL_BROWSER_CMD` environment override continues to override
  `BROWSER_CMD` for the process, applied after validation.

### Validation & mode

- **REQ-04** `LoadGlobal` requires only `BROWSER_CMD` (non-empty); it MUST NOT require
  either host. A config with `BROWSER_CMD` set and no hosts is valid (local mode).
- **REQ-05** `Global.IsLocal()` returns true iff `BROWSER_HOST ∈ {"", "localhost",
  "127.0.0.1"}` **and** `VM_SSH_HOST == ""`. (Tightens the shipped local-mode check so a
  `VM_SSH_HOST`-only box is remote.)
- **REQ-06** `config.IsPlaceholder(g)` returns true when a host value equals a shipped
  sentinel (`my-laptop`, `my-vm`), used to trigger the runtime nudge — it is advisory,
  not a hard `LoadGlobal` error.

### Environment detection (`internal/envdetect`)

- **REQ-07** `envdetect` is a **pure** package: `Detect(Env) []Candidate` takes an injected
  `Env` snapshot (no direct OS reads inside the decision logic) so it is fully table-testable.
- **REQ-08** Signal → candidate rules:
  - `/proc/version` contains `microsoft` (case-insensitive) **or** `$WSL_DISTRO_NAME` set
    → **local**, `BROWSER_CMD=wslview`, `BROWSER_HOST=localhost`.
  - `runtime.GOOS == "darwin"` → **local**, `BROWSER_CMD=open`, `BROWSER_HOST=localhost`.
  - `$DISPLAY` or `$WAYLAND_DISPLAY` set **and** `xdg-open` on `PATH` → **local**,
    `BROWSER_CMD=xdg-open`, `BROWSER_HOST=localhost`.
  - `$SSH_CONNECTION` or `$SSH_TTY` set → **remote**, `VM_SSH_HOST=DeriveVMHost(...)`,
    `BROWSER_CMD=""` (the wizard asks the dev-machine OS), `BROWSER_HOST=""`.
- **REQ-09** Ranking / `Recommended`: when an SSH session is present, the remote candidate
  is recommended and ordered first; otherwise the local candidate is recommended. At most
  one local and one remote candidate are returned (deduped).
- **REQ-10** `Detect` always returns ≥1 candidate. A box with no local signal and no SSH
  session yields a single remote candidate with an empty `VM_SSH_HOST` for the user to fill.
- **REQ-11** `DeriveVMHost(sshConnection)` returns field 3 (server IP) of a well-formed
  `$SSH_CONNECTION` ("client_ip client_port server_ip server_port"); returns `""` for
  empty or malformed input.

### `azrl setup` command

- **REQ-12** `azrl setup` (visible, top-level; NOT the deprecated `init` stub) runs
  detection and writes `~/.azure-profiles/azrl.conf`.
- **REQ-13** Interactive flow (Bubble Tea, styled like the existing pickers): (1) if >1
  candidate, a pick list with the recommended candidate pre-selected; (2) a field form —
  local shows `BROWSER_CMD` pre-filled; remote shows a dev-machine-OS picker
  (macOS→`open` / WSL→`wslview` / Linux→`xdg-open`) mapping to `BROWSER_CMD`, an editable
  `VM_SSH_HOST` pre-filled from derivation, and an optional `BROWSER_HOST` (blank = paste
  mode); (3) a confirm step; (4) write.
- **REQ-14** Before writing, if `azrl.conf` exists it is copied to `azrl.conf.bak`
  (overwriting any prior backup); the interactive flow shows a summary and confirms first.
- **REQ-15** `--yes`/`-y`: non-interactive — choose the recommended candidate, apply
  derived/default values, back up, and write with no prompts. Used by installers.
- **REQ-16** `--print`: print the detected/resolved config to stdout and write nothing
  (dry run / diagnostics).
- **REQ-17** `(g Global) Write(path)` emits the new keys with brief guiding comments and is
  the single writer used by the wizard and `--yes`.

### Runtime nudge

- **REQ-18** Commands needing global config load it through a shared helper: on
  missing/placeholder/invalid config **and** an interactive TTY, launch the setup wizard,
  then reload and continue the original command; on a non-TTY, return the underlying error
  plus a "run `azrl setup`" hint. Wired into the provider login paths and bare `azrl` TUI
  launch.

### Bridge / VM-host resolution

- **REQ-19** The path-A paste line resolves the VM host as: `VM_SSH_HOST` if set, else
  `DeriveVMHost($SSH_CONNECTION)`, else the literal `<your-vm-host>` with a printed hint to
  set `VM_SSH_HOST`. `VM_SSH_HOST` is therefore never required to configure.
- **REQ-20** Local mode (`IsLocal()`) continues to bypass SSH entirely (as already shipped).

### Installers

- **REQ-21** `install.sh` and `scripts/install.sh` remove their inline WSL/browser
  detection. When `azrl.conf` is absent or placeholder, they run `azrl setup --yes` to
  seed the recommended config, then print "run `azrl setup` to review/change". They still
  create `~/.azure-profiles/` and gitignore the pointer files.

### Guidelines

- **GUD-01** Detection logic lives once, in Go (`envdetect`); shell scripts are thin callers.
- **GUD-02** Never mutate a provider's native default identity (unchanged PAT-002 stance).
- **GUD-03** Backward compatibility via alias reads — do not break existing three-key configs.
- **GUD-04** TDD-first: pure logic (`envdetect`, `config`) unit-tested directly; the wizard
  tested via `View()`/`Update()` like the other `internal/ui` models.

## 4. Interfaces & Data Contracts

### `internal/envdetect`

```go
type Mode int
const (
    Local Mode = iota
    Remote
)

// Env is an injected snapshot of the host environment (no OS reads in Detect).
type Env struct {
    ProcVersion   string            // contents of /proc/version ("" if unreadable)
    WSLDistro     string            // $WSL_DISTRO_NAME
    GOOS          string            // runtime.GOOS
    Display       string            // $DISPLAY + $WAYLAND_DISPLAY concatenated
    SSHConnection string            // $SSH_CONNECTION
    SSHTTY        string            // $SSH_TTY
    Has           func(bin string) bool // PATH lookup (e.g. exec.LookPath != nil)
}

type Candidate struct {
    Mode        Mode
    Label       string // e.g. "Local (WSL → Windows browser)"
    Reason      string // why this was detected
    BrowserCmd  string // pre-filled default; "" for remote (ask dev-machine OS)
    BrowserHost string // "localhost" for local; "" for remote
    VMSSHHost   string // derived for remote; "" otherwise
    Recommended bool
}

func Detect(e Env) []Candidate      // ranked, recommended-first, ≥1 element
func DeriveVMHost(sshConn string) string // field 3 of $SSH_CONNECTION, or ""

// RealEnv builds an Env from the actual process/OS (the only impure helper).
func RealEnv() Env
```

### `internal/config`

```go
type Global struct {
    BrowserCmd  string // was LocalBrowserCmd
    BrowserHost string // was LocalHost
    VMSSHHost   string // was VMHost
}

func (g Global) IsLocal() bool          // BrowserHost local-ish AND VMSSHHost == ""
func IsPlaceholder(g Global) bool        // shipped sentinels present
func LoadGlobal(dir string) (Global, error) // new keys, alias fallback, BROWSER_CMD required
func (g Global) Write(path string) error     // emit new keys + comments; caller backs up
```

Legacy field names on `Global` may be kept as aliases/accessors during the transition if
it reduces call-site churn, but the canonical fields are the three above.

### `cmd/setup.go`

```
azrl setup            # interactive wizard
azrl setup --yes      # non-interactive: recommended candidate, write
azrl setup --print    # print resolved config, write nothing
```

### `azrl.conf` (written form)

```
# azrl config — local mode: the browser opens on this machine, no SSH bridge.
BROWSER_CMD=wslview
BROWSER_HOST=localhost
# VM_SSH_HOST=            # only needed for the remote path-A paste fallback
# DASHBOARD_POLL_SECS=3
```

```
# azrl config — remote mode: browser opens on your dev machine over SSH.
BROWSER_CMD=open              # runs on your dev machine
# BROWSER_HOST=my-laptop     # set for zero-paste (VM must reach your machine)
VM_SSH_HOST=203.0.113.10     # derived from $SSH_CONNECTION; edit for NAT/jump hosts
```

## 5. Acceptance Criteria

- **AC-01** *Given* a config with `BROWSER_CMD=wslview` and no host keys, *When*
  `LoadGlobal` runs, *Then* it succeeds and `IsLocal()` is true.
- **AC-02** *Given* a config using only legacy keys (`LOCAL_BROWSER_CMD`/`LOCAL_HOST`/
  `VM_HOST`), *When* `LoadGlobal` runs, *Then* the values populate the new fields via
  alias fallback and behavior is unchanged.
- **AC-03** *Given* `BROWSER_HOST=""` and `VM_SSH_HOST=my-vm`, *When* `IsLocal()` is
  evaluated, *Then* it returns false (remote), and Bridge produces a path-A paste line.
- **AC-04** *Given* an `Env` with `WSLDistro="Ubuntu"`, *When* `Detect` runs, *Then* the
  sole/recommended candidate is local with `BROWSER_CMD=wslview`, `BROWSER_HOST=localhost`.
- **AC-05** *Given* an `Env` with `SSHConnection="198.51.100.2 51000 203.0.113.10 22"`,
  *When* `Detect` runs, *Then* a remote candidate is recommended with
  `VMSSHHost="203.0.113.10"` and empty `BrowserCmd`.
- **AC-06** *Given* an `Env` that is both WSL and in an SSH session, *When* `Detect` runs,
  *Then* both a local and a remote candidate are returned and the **remote** one is
  `Recommended`/first.
- **AC-07** *Given* `azrl setup --yes` on a WSL box with no existing conf, *When* it runs,
  *Then* it writes `azrl.conf` with `BROWSER_CMD=wslview` + `BROWSER_HOST=localhost` and
  exits 0 with no prompt.
- **AC-08** *Given* an existing `azrl.conf`, *When* `azrl setup` writes, *Then*
  `azrl.conf.bak` contains the previous contents and `azrl.conf` the new.
- **AC-09** *Given* `azrl setup --print`, *When* it runs, *Then* the resolved config is
  printed and no file is written/modified.
- **AC-10** *Given* a missing or placeholder config and a TTY, *When* `azrl login` runs,
  *Then* the setup wizard launches, and after completion the login proceeds; *Given* no
  TTY, *Then* it errors with a "run `azrl setup`" hint.
- **AC-11** *Given* remote mode with `VM_SSH_HOST` unset but `$SSH_CONNECTION` present,
  *When* a path-A paste line is produced, *Then* it contains the derived server IP.
- **AC-12** *Given* `install.sh`/`scripts/install.sh` on a fresh machine, *When* run,
  *Then* they call `azrl setup --yes`, `azrl.conf` exists and is valid, and no inline
  shell OS-detection remains in the scripts.

## 6. Test Automation Strategy

- **Unit (pure).** `envdetect.Detect` — table-driven across every signal combination
  (WSL, macOS, Linux-desktop, SSH-only, WSL+SSH, empty); `DeriveVMHost` valid/empty/
  malformed. `config` — alias fallback, tightened `IsLocal`, `IsPlaceholder`, required-
  `BROWSER_CMD` validation, `Write` round-trip (write→`LoadGlobal` equals input).
- **TUI.** The setup model tested like existing `internal/ui` models: assert `View()`
  content and `Update()` transitions for candidate pick, field override, confirm →
  writes conf + `.bak`; plus `--yes` and `--print` code paths (exercised at the `cmd`
  layer with a temp `HOME`/profiles dir).
- **Bridge.** Extend existing tests: path-A VM-host resolution falls back derived →
  placeholder; local-mode short-circuit unchanged.
- **cmd.** `azrl setup --yes`/`--print` via the existing `cmd` test harness (temp dirs,
  fake `PATH`). Runtime-nudge branch tested by pointing at a missing/placeholder conf with
  a non-TTY writer and asserting the hint.
- **Installers.** Not unit-tested (shell); add a line to
  `specs/*.manual-verify.md` for the WSL and remote fresh-install paths.
- **Gate.** `go test ./...` and `gofmt -l .` clean, per CI `validate`.

## 7. Rationale & Context

`LOCAL_HOST` and `VM_SSH_HOST` name different machines from opposite SSH vantage points,
so they cannot collapse to one value — but on a real network only one direction is
reachable (cloud VM with public SSH + NAT'd laptop → path A; LAN/VPN → path B; same
machine → neither). Requiring both is therefore always at least one value of dead weight
and a frequent source of "it didn't pop the browser." Making both optional, deriving the
VM host from `$SSH_CONNECTION`, and detecting the environment reduces the required config
to a single browser command (often zero, seeded by the installer). Role-based key names
(`BROWSER_*`) read better than device-flavored ones (`LAPTOP_HOST` was rejected as
skeuomorphic) and pair naturally (`BROWSER_CMD` opens the browser on `BROWSER_HOST`).
Detection lives in Go so the two installers and the runtime nudge share one tested
implementation rather than duplicating shell heuristics.

## 8. Dependencies & External Integrations

### External Systems
- **OpenSSH client** — unchanged; `$SSH_CONNECTION` is read for host derivation.

### Third-Party Services
- None.

### Infrastructure / Platform
- **Bubble Tea / Lip Gloss** — already vendored; the wizard reuses existing UI patterns.

### Data
- **`~/.azure-profiles/azrl.conf`** and its `.bak` sibling — the only files written.

## 9. Examples & Edge Cases

- **WSL fresh install** → installer runs `azrl setup --yes` → `BROWSER_CMD=wslview`,
  `BROWSER_HOST=localhost`; `azrl login` pops the Windows browser, no SSH.
- **Cloud VM over SSH, NAT'd dev machine** → detection recommends remote; wizard asks
  dev-machine OS (`open`), pre-fills `VM_SSH_HOST` from `$SSH_CONNECTION`; path-A paste.
- **NAT/jump host makes the derived IP wrong** → the field is editable in the wizard and
  `VM_SSH_HOST` overrides the derivation at runtime.
- **SSH into a WSL box** → both candidates offered; remote recommended (you're logged in
  from elsewhere); user can pick local to open the Windows browser on that box.
- **Headless VM on serial console (no `$SSH_CONNECTION`)** → single remote candidate with
  blank `VM_SSH_HOST`; wizard requires the user to type it.
- **Container for local dev** → treated as headless-remote unless WSL/display signals are
  present; user overrides in the wizard.
- **Legacy three-key config** → keeps working untouched via alias reads; `azrl setup`
  re-run migrates it to the new keys (with `.bak`).

## 10. Validation Criteria

- All Acceptance Criteria (AC-01…AC-12) pass as automated tests where feasible.
- `go test ./...` and `gofmt -l .` are clean.
- Manual verify (fresh WSL install and fresh remote-VM install) recorded in the
  manual-verify sheet.
- No inline OS/browser detection remains in `install.sh` / `scripts/install.sh`.
- An existing three-key `azrl.conf` continues to work with no changes.

## 11. Amendments

_None yet._

## 12. Related Specifications / Further Reading

- `specs/resolution-strategies.md` — identity/mapping model and the ambient read-through
  stance this builds alongside.
- `docs/ambient-identity-model.md` — why azrl never mutates a native default (PAT-002).
- `CLAUDE.md` — configuration model, bridge path A/B, and the (now-extended) local mode.
