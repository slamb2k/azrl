# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`azrl` (Azure Remote Login) is a Bash CLI that runs interactive `az login` from a
headless/remote Linux VM, popping the sign-in browser on your local machine and
forwarding the random OAuth callback port back to the VM — even when Conditional
Access blocks device-code flow.

## Commands

```bash
bats tests/azrl.bats                      # run the unit suite
bats tests/azrl.bats -f "azrl_extract_port"   # run tests matching a name filter
shellcheck azrl azrl-lib.sh azrl-capture  # lint all three scripts
./install.sh                              # symlink onto PATH, gitignore .azprofile, bootstrap config
```

There is no build step — the scripts run in place. `install.sh` symlinks `azrl`
and `azrl-capture` into `~/.local/bin`, so edits to the source files take effect
immediately without reinstalling.

## Architecture

The tool solves three problems with `az login` on a headless box, split across
three files:

- **`azrl`** — the orchestrator (the only thing on PATH besides the capture
  shim). Parses args, loads config, then drives the login lifecycle: clean slate
  → capture the auth URL → bridge the browser → wait with a watchdog timeout →
  assert identity. Holds the `cleanup` EXIT trap that kills the tunnel/watchdog/
  login PIDs and removes the capture file.
- **`azrl-lib.sh`** — pure, sourceable functions, **fully unit-tested** in
  `tests/azrl.bats`. Sourcing it must have no side effects. This is where logic
  belongs: profile resolution, port extraction, identity assertion, the bridge,
  the capture loop. New behaviour should be added here as a testable function and
  called from the orchestrator, not inlined into `azrl`.
- **`azrl-capture`** — a tiny `$BROWSER` shim. `az login` launches it via Python's
  `webbrowser`; it writes the callback URL to `$AZRL_CAPFILE` and exits 0 so MSAL
  believes a real browser opened (this is what prevents the device-code fallback).

### The login flow (in `azrl`)

1. `azrl_clean_slate` — `az logout`/`az account clear` and remove the scoped MSAL
   caches, operating only within the isolated `$AZURE_CONFIG_DIR`.
2. `azrl_login_capture` — runs `az login` in the background with `BROWSER` pointed
   at `azrl-capture`, polls for the capture file, then parses the random callback
   port from the URL.
3. `azrl_bridge` — **path B (zero-paste)**: if the local host is SSH-reachable,
   open a reverse tunnel (`ssh -R port:localhost:port`) and launch the browser
   there. **Path A (fallback / `--paste`)**: print a one-line `ssh -fNL …` for the
   user to paste on their local machine.
4. Watchdog: a background `sleep`+`kill` enforces `AZRL_LOGIN_TIMEOUT` (default
   180s) against the backgrounded login PID.
5. `azrl_assert_account` — after login, verify tenant (by domain **or** GUID) and
   optionally the expected user, failing loudly on mismatch.

### Cross-file conventions

- All scripts use `set -euo pipefail`.
- `azrl-lib.sh` functions communicate back to the orchestrator by setting
  **`AZRL_*` globals** (e.g. `AZRL_PORT`, `AZRL_URL`, `AZRL_LOGIN_PID`,
  `AZRL_TUNNEL_PID`, `AZRL_CAPFILE`). The orchestrator's `cleanup` trap reads
  these PIDs, so any new long-lived background process should export its PID as an
  `AZRL_*` global for teardown.
- Functions take config via parameters and read a few documented env globals
  (`LOCAL_HOST`, `LOCAL_BROWSER_CMD`, `VM_HOST`, `AZRL_FORCE_PASTE`) — keep them
  parameterised so they stay unit-testable with PATH-shimmed `az`/`ssh`.

### Configuration model

- `~/.azure-profiles/azrl.conf` — global: `LOCAL_HOST`, `LOCAL_BROWSER_CMD`,
  `VM_HOST` (sourced by the orchestrator).
- `~/.azure-profiles/<profile>.conf` — per-profile: `AZ_TENANT` (required),
  `AZ_TENANT_ID` (GUID — required for guest/B2B where `tenantDefaultDomain` is
  null), `AZ_DEFAULT_SUB`, `AZ_EXPECT_USER`.
- `<repo>/.azprofile` — one line naming the profile; resolved by walking up from
  `$PWD`. Globally gitignored (never commit it).
- `~/.azure-profiles/<profile>/` — isolated `AZURE_CONFIG_DIR` per profile.

## Testing approach

Pure logic in `azrl-lib.sh` is unit-tested directly. Integration points are tested
by shimming `az` and `ssh` onto `PATH` inside a temp dir (see the `azrl_bridge` and
`azrl_login_capture` tests for the pattern). When adding a function that shells out,
follow that pattern rather than mocking at a higher level. The project was built
TDD-first; see `HANDOVER.md` for full historical context.
