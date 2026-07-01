# Status Dashboard (Phase 5.5) — Manual Verification

- **Date:** 2026-07-01
- **Purpose:** Items implemented and unit-tested here, but whose end-to-end
  behaviour can only be *closed out* on a real laptop + remote VM with live
  Azure/GitHub sessions. Nothing below is claimed "done" — each is a scripted
  repro for the user to run in the real environment.
- **Related:** `specs/status-dashboard.md`, `specs/status-dashboard.goal.md`.
- **Shipped in:** #21 (feature), #22 (real-world BOM + stdout fixes).

## What IS verified

- Unit + shimmed-integration tests green across all packages (`go test ./...`),
  `gofmt -l .` clean, `go vet` clean. Both providers pass the shared
  `providertest.RunContract` suite, including the **no-network** proof (fake
  `az`/`gh` that `exit 1` and touch a sentinel the test asserts is never
  created).
- Drift detection is unit-tested per provider (`TestStatusDrift`): ambient env
  unset / == isolated / == other dir / cwd-not-pinned — including the
  ambient-unset regression that a prior `ambient != ""` guard masked.
- **Data layer verified against a real machine** (`~/.azure-profiles`, four real
  Azure profiles): `azureProfile.json` (`isDefault` → `user.name`) and
  `msal_token_cache.json` (`AccessToken[].expires_on`) field names confirmed;
  `Identity` and `Expiry` populate for signed-in profiles and blank gracefully
  for empty ones; `azrl status --json` emits a valid array on **stdout**. This
  surfaced and fixed two bugs the synthetic fixtures missed (UTF-8 BOM in
  `azureProfile.json`; output written to stderr) — see #22.

## Items requiring the real laptop + VM + live sessions

### 1. Live TUI dashboard poll + render

**Why manual:** `tea.Tick` is a real wall-clock timer and the TUI needs a TTY;
unit tests feed synthetic `dashboardTickMsg` and assert `View()` only.

**Repro (on the VM, in a repo with no `.azprofile`/`.ghprofile` pin):**
```bash
azrl                       # bare invocation → dashboard is the default (tab 0)
# Expect: a cross-provider table (Provider | Profile | Identity | Dir | Expiry |
# Drift | Last used), sorted by last-used desc, refreshing every 3s
# (or DASHBOARD_POLL_SECS from ~/.azure-profiles/azrl.conf).
```
**Pass:** the table renders real Azure + GitHub rows, relative expiry ("in 42m"
/ "expired") updates on the poll, `[` / `]` still switch tabs, `[r]` refreshes
all and `[w]` rechecks drift with no network call.

### 2. Enter drill-through pre-selects the profile

**Why manual:** needs the interactive TUI.

**Repro:** on the dashboard, move to a non-first row and press `Enter`.
**Pass:** the matching provider tab activates **with that profile's row already
selected** (cursor landed on it, not the top of the list).

### 3. Real drift indicator via an unset / mismatched `AZURE_CONFIG_DIR`

**Why manual:** exercises the live env-vs-pinned comparison against a real
`.azprofile` pin and a real shell environment (e.g. a repo whose `.envrc`
wasn't `direnv allow`-ed).

**Repro:**
```bash
cd <repo pinned with `azrl use work`>
env -u AZURE_CONFIG_DIR azrl status     # ambient unset while pinned to `work`
# Expect: the `work` row shows the ⚠ drift marker (unset ambient ≠ isolated dir).
AZURE_CONFIG_DIR=~/.azure-profiles/work azrl status   # aligned
# Expect: no drift marker on `work`.
```
**Pass:** drift flags exactly the pinned row when the ambient session diverges,
and clears when it matches.

### 4. GitHub identity from a real `hosts.yml`

**Why manual:** needs a real `gh auth login --insecure-storage` under an
isolated `GH_CONFIG_DIR`.

**Repro:**
```bash
ghrl login work --hostname github.com
azrl status --json | jq '.[] | select(.provider=="GitHub")'
```
**Pass:** the GitHub row's `identity` reflects the `user`/host from
`~/.github-profiles/work/hosts.yml`; `expiry` is `null` (gh tokens don't expire).

### 5. `LAST_USED` / `LAST_DIR` bump on real use/login/capture

**Why manual:** confirms the `Scheme.Touch` call sites fire in the real CLI flows
(not just the unit-tested helper).

**Repro:**
```bash
cd <some repo> && azrl use work
grep -E 'LAST_USED|LAST_DIR' ~/.azure-profiles/work.conf
```
**Pass:** both keys are present and current after `use`/`login`/`capture`
(Azure and GitHub), and the dashboard sorts that profile to the top.
