# Orphaned az-login Sweep in CleanSlate — Design

**Date:** 2026-07-05
**Status:** Approved (brainstormed interactively)

## Problem

Repeated interactive `az login` attempts (typically manual runs outside azrl —
a broken port-forward, an expired PIM role, a killed terminal) leave background
`az login` processes waiting forever for a browser callback that will never
arrive. These zombies compete for the next login's OAuth callback: if the
browser redirect lands on a stale listener, the fresh login just spins. The
historical incident that motivated azrl had 4+ such zombies and was resolved
with a blind `pkill -f 'az login'` — which would also have killed a login the
user was legitimately running.

azrl already kills the `az login` processes *it* spawns on every exit path
(capture failure, bridge failure, sign-in timeout — `cmd/flow.go`,
`internal/azure/bridge.go`). This design closes the remaining gap: sweeping
zombies from *manual* runs at the start of every azrl login, without touching
real ones.

## Orphan definition

A process is an **orphan** when all three hold:

1. Its cmdline is an `az login` invocation (matcher below).
2. It belongs to the current user (UID match).
3. Its parent has died — `PPid` is 1 (reparented to init/systemd).

A live login in another terminal has a living shell as its parent, so it never
matches. The heuristic fails safe: on setups where orphans reparent to a
subreaper instead of PID 1, an orphan is *missed*, never a real login killed.
A deliberately `nohup`/`setsid`-detached `az login` also reads as an orphan —
acceptable, since a detached interactive login waiting on a browser callback
is unusable anyway.

Rejected alternatives:

- **Age threshold** — wrong in both directions: kills a slow-but-real login,
  misses fresh orphans (the historical zombies caused callback theft within
  minutes).
- **PID registry** (reap only PIDs azrl spawned) — azrl already cleans its
  own; the orphans that bite come from manual runs a registry can never see.

## Cmdline matcher

`az` is a launcher for `python -m azure.cli`, so the visible cmdline varies by
install. A cmdline matches when **both**:

- some argv token is `az`, ends in `/az`, or contains `azure.cli`, and
- a **later** token is exactly `login`.

Matches: `az login`, `/usr/bin/python3 /usr/bin/az login --tenant x`,
`/opt/az/bin/python3 -Im azure.cli login`.
Rejects: `az logout`, `az account show`, `azrl login work` (azrl itself),
unrelated python processes, and anything where `login` precedes the az token.

## Components

**New `internal/azure/orphans.go`:**

- `type loginProc struct { PID int; Age time.Duration }`
- `classifyAzLogins(procRoot string, uid int) (orphans, live []loginProc)` —
  pure. Walks numeric entries under `procRoot` (production: `/proc`), parsing
  per pid:
  - `cmdline` — NUL-separated argv, run through the matcher;
  - `status` — `Uid:` (first field, real UID) and `PPid:`;
  - `stat` field 22 (starttime) + `procRoot/uptime` → process age (best-effort;
    zero on parse failure).
  Malformed or unreadable entries are skipped silently.
- `SweepOrphanedLogins(out io.Writer)` — classifies against `/proc` and the
  real UID, sends SIGTERM to each orphan, and prints:
  - `azrl: reaped orphaned az login (pid 1234)` per kill;
  - `azrl: note: another az login is running (pid 5678, 12m) — it may steal
    the browser callback` per live process (warn, never kill — user decision).
  The kill is injected (`kill func(pid int) error`, default
  `os.FindProcess(pid)` + `p.Signal(syscall.SIGTERM)` — not `syscall.Kill`,
  so the package still compiles for windows/darwin release builds) so tests
  never signal real processes.

**Wiring:** `CleanSlate(cfgDir string, out io.Writer)` gains the writer and
calls `SweepOrphanedLogins(out)` before the existing logout/account-clear/
cache-removal. Both call sites already have `out` in scope
(`cmd/login.go` and `runAzureInit` in `cmd/init.go`).

## Error handling

Best-effort throughout, matching CleanSlate's existing philosophy: a missing
`/proc` (macOS/non-Linux), permission errors, and kill races (process exits
between classify and signal) are all ignored silently. The sweep can never
block or fail a login.

## Testing

- Matcher table test (the match/reject rows above).
- `classifyAzLogins` against fixture proc trees under `t.TempDir()`: an orphan
  (`PPid: 1`), a live login (living parent pid), a wrong-UID entry, a non-az
  process, a malformed/unreadable entry — asserting exact orphan/live split.
- Sweep test with an injected recording `kill` func: orphans signalled, live
  ones not, output lines match.
- Existing CleanSlate tests updated for the new signature.

## Docs

- CLAUDE.md login-flow step 1 and README: "reaps orphaned `az login`
  processes (parent dead) and warns about live ones."

## Out of scope

- Sweeping stale `gcloud auth login` / `aws sso login` processes (same
  pattern; add when it bites).
- Killing live (non-orphaned) az logins, interactively or otherwise.
- Non-Linux process walking (sweep is a silent no-op without `/proc`).
