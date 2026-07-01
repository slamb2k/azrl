# Status Dashboard ("who am I, everywhere") — Design Spec

- **Date:** 2026-07-01
- **Status:** Approved (scoped); slots into the provider-aware roadmap as
  **Phase 5.5**. Phases 1–7 (GitHub Remote Login) shipped in #17, so this lands
  as the next follow-on and can be proven against two shipped providers
  (Azure + GitHub).
- **Author:** brainstormed with Claude
- **Related:** extends `specs/github-remote-login.md` (the provider-aware binary +
  tabbed TUI). Formalizes the README roadmap's "Richer auditability — a quick
  'who am I, everywhere?' view" line into a real phase.

## Summary

Add a top-level **status dashboard** as the default landing view of the
provider-aware TUI: a single glanceable table that iterates every registered
provider (Azure + GitHub today; AWS/GCP later) and shows, per profile, **who you
are, where it's bound, when you last touched it, and whether the ambient CLI
session has drifted from the pinned profile**. It answers "which account is live
in which directory, everywhere?" without leaving the terminal and without a
single network call.

This is **composition over the `Provider` interface**, not new auth logic. It
adds one required interface method (`Status()`), one persisted timestamp
(`LastUsed`), and one new TUI view. The per-provider tabs and their action panes
are unchanged — the dashboard drills through to them.

## Motivation

azrl exists to remove "which account am I about to act on?" ambiguity. Now that
the binary hosts multiple providers and multiple profiles per provider, that
ambiguity returns at a higher altitude: a user with two Azure tenants, three
GitHub accounts, and (soon) AWS/GCP profiles has no single place to see the whole
picture. The tabbed TUI shows one provider at a time; nothing shows the union.
The most common real question — *"is my shell currently pointing at the right
account for this repo, across all my clouds?"* — needs a cross-provider view.

Two hard constraints shape the design:

- **Cheap enough to leave open.** The intended usage is "keep it up and glance
  at it." That is only viable if refreshing costs nothing — so `Status()` reads
  **local cache/config state only**. No `az account show`, no `gh api user`, no
  `aws sts get-caller-identity`, no `gcloud auth list` on a timer. Network calls
  would trip SSO rate limits, silently refresh/rotate tokens while the dashboard
  just sits there, and make a 2–5s poll loop antisocial.
- **No new ambiguity.** A dense multi-provider table must not become a place
  where you fire heterogeneous actions at the wrong row. Azure's `capture`
  doesn't map to GitHub's device flow; AWS SSO has its own shape. So the
  dashboard is **read-only + drill-through** — `Enter` jumps to the provider's
  tab with the profile pre-selected, where the existing scoped action pane takes
  over. (Two exceptions, both read-only: refresh-all and drift-recheck.)

## Goals

- One default view that lists every profile across every registered provider,
  sorted by most-recently-used, glanceable at a glance.
- Surface per profile: identity, last-bound directory, expiry (if the provider
  has one), **drift** (ambient session ≠ pinned profile), and last-used time.
- Refresh cheaply from disk on a 2–5s cadence; safe to leave open indefinitely.
- Drill through to the owning provider's tab/action pane; never fire
  provider-specific actions inline.
- Force every current and future provider to implement `Status()` correctly via
  the shared contract suite.

## Non-goals (v1)

- **A full audit-log / history subsystem.** One `LastUsed` timestamp per profile
  is enough to answer "what have I touched recently." Per-event history (who was
  active in which dir over time) is explicitly deferred — it's the *next* rung of
  the README's "history of which account was active in which directory" line, not
  this phase.
- **Network-backed liveness.** The dashboard never proves a token is *valid* —
  only what local state says. Validity is what per-provider `AssertAccount`
  (network) is for, reached by drilling into the tab.
- **Inline per-provider action menus** on dashboard rows (see B4 rationale).
- **Cross-provider "unified profile"** binding (that's the separate roadmap
  "Unified profiles" direction).

## Architecture

### `Provider.Status()` — new required interface method

Extend `internal/provider.Provider` with one method. It joins the existing
**profile-mechanic surface** (`ListProfiles`/`Resolve`/`Use`/`Remove`/`SetLabel`)
— i.e. it lives on the interface, is provider-agnostic in shape, and is driven
generically by the dashboard. It reads only local state.

```go
// Status is a normalized, disk-only snapshot of one profile for the dashboard.
type Status struct {
    ProfileName string     // slug (matches Listed.Name)
    Identity    string     // provider-defined: email / account / tenant / login
    Directory   string     // last-bound dir, if any ("" if never bound)
    Expiry      *time.Time // token/session expiry; nil if provider has none or unknown
    Drifted     bool       // ambient CLI session != pinned profile
    LastUsed    time.Time   // persisted; bumped on use/login/dir-bind
}

// Status returns a per-profile snapshot from local cache/config only.
// It MUST NOT make network calls (no `az account show`, `gh api user`,
// `aws sts get-caller-identity`, `gcloud auth list`). Callers poll it on a
// short timer, so it must be cheap and side-effect-free.
Status(name, confdir string) (Status, error)
```

Field semantics, provider by provider (all read from files already on disk under
the profile's config dir / conf file):

| Field | Azure | GitHub | AWS (later) | GCP (later) |
|---|---|---|---|---|
| `Identity` | signed-in user / tenant from MSAL cache in `AZURE_CONFIG_DIR` | `GH_USER` + host from `hosts.yml` under `GH_CONFIG_DIR` | SSO account/role from `~/.aws/sso/cache` + profile | active account from `gcloud` config / `credentials.db` |
| `Directory` | `LAST_DIR` conf key (see tracking below) | same | same | same |
| `Expiry` | `accessToken` expiry in MSAL cache, if present | `gh` tokens don't expire → `nil` | SSO cached token `expiresAt` (plain JSON) | `nil` in v1 (expiry is in SQLite `access_tokens.db`; see multi-cloud spec) |
| `Drifted` | ambient `AZURE_CONFIG_DIR`/active-sub ≠ this profile's | ambient `GH_CONFIG_DIR` ≠ this profile's | ambient `AWS_PROFILE` ≠ this profile's | ambient `CLOUDSDK_ACTIVE_CONFIG_NAME` ≠ this profile's |

**Drift is computed from environment + disk only** — compare the ambient env var
the provider would honor (`AZURE_CONFIG_DIR`, `GH_CONFIG_DIR`, `AWS_PROFILE`,
`CLOUDSDK_ACTIVE_CONFIG_NAME`) against the profile the pinned `.xxprofile`
resolves to. No process is spawned. If a provider genuinely cannot determine a
field without the network, it returns the zero value (`Expiry == nil`,
`Identity == ""`) rather than reaching out — a blank cell is honest; a network
call is a bug.

### `LastUsed` + `Directory` tracking

Two scalar keys per profile, persisted to the profile's conf file
(`~/.<provider>-profiles/<name>.conf`) and bumped **together** on every event that
makes a profile "the one you touched":

- `LAST_USED` — an RFC 3339 timestamp (feeds `Status().LastUsed` and the sort).
- `LAST_DIR` — the absolute directory of the touch event (feeds
  `Status().Directory` / the `Dir` column). Without this the `Directory` field has
  no source: a profile can be pinned by `.xxprofile` in *many* directories across
  a tree, so "last-bound dir" is specifically the most-recent bind, which only a
  persisted value can answer. `Status()` reads it straight back; blank → `""`.

Both are bumped on:

- `use` (pin a directory to the profile) — `LAST_DIR` = the pinned dir,
- `login` / `capture` (establish or record a session) — `LAST_DIR` = `$PWD`,
- any directory-bind that rewrites `.xxprofile` — `LAST_DIR` = that dir.

The bump is a single small helper in the parameterized `internal/profile` package
(it already owns conf I/O and the `.envrc`/pointer writes), so every provider gets
both keys for free by going through the shared writer rather than each
re-implementing it. A missing/blank `LAST_USED` sorts last (zero time).

Scope discipline: **two scalars, not a log.** One timestamp + one directory per
profile — no append-only history, no per-directory ledger. That keeps the conf
file human-readable and the feature small; the richer "history" ambition (every
dir a profile was ever active in, over time) stays a future phase.

### Dashboard view (TUI)

A new **top-level view**, sibling to the per-provider tabs — *not* nested inside
any one tab. It is owned by the tab container introduced in Phase 5.

- On mount and on each poll tick, iterate all registered providers, call
  `ListProfiles` then `Status(name, confdir)` per profile, and flatten into one
  slice of `(providerTitle, Status)` rows.
- Render as a single table, **sorted by `LastUsed` descending** by default,
  columns: `Provider | Profile | Identity | Dir | Expiry | Drift | Last used`.
  Drift renders as a loud marker (e.g. `⚠ drift`) so it's the thing your eye
  catches. Expiry renders relative ("in 42m", "expired") from the cached
  timestamp — still no network, just arithmetic on the stored value.
- **Default landing view:** bare `azrl` (no args) opens the dashboard, not a
  provider tab. Per-provider tabs are one keypress away (the existing tab-switch
  keybind), matching "keep it up and glance at it." A bare `ghrl`/`azrl`-alias
  invocation still preselects its provider's tab (back-compat), but the neutral
  unified entrypoint lands on the dashboard.
- **Refresh cadence:** poll local cache every **3s by default** (disk-only per
  `Status()`), overridable via a `DASHBOARD_POLL_SECS` key in `azrl.conf`; no
  network refresh loop. A Bubble Tea `tea.Tick` drives it; the poll is cheap
  enough that leaving the dashboard open overnight costs nothing but stat calls.
  3s is the midpoint of the 2–5s range — responsive without being busy, and it's
  all disk either way.

### Actions — drill-through, not inline

Dashboard rows carry **no** per-provider action menu. Rationale (B4): providers'
actions are heterogeneous (`capture` vs device flow vs SSO), and cramming them
into one dense multi-provider table reintroduces exactly the "which account am I
about to act on?" ambiguity azrl exists to remove.

- **`Enter` on a row** → jump to that provider's tab with the profile
  pre-selected; the existing per-provider ACTION pane takes over from there. The
  dashboard is a router, not an actuator.
- **Global inline exceptions (read-only, safe to keep un-scoped):**
  - `[r]` — **refresh all** rows now (re-read disk immediately instead of
    waiting for the next tick).
  - `[w]` — **recheck ambient drift** for all rows (re-evaluate env vs pinned;
    still disk/env-only, no network).

Both exceptions are read-only and provider-agnostic, so they don't reintroduce
the targeting ambiguity that per-provider write actions would.

## Command surface

- Bare unified invocation (no args) → **dashboard** (new default landing view).
- Tab-switch keybind cycles dashboard ↔ Azure ↔ GitHub ↔ … (dashboard is the
  leftmost/first view).
- CLI parity: `<bin> status` prints the same aggregation non-interactively (one
  shot, disk-only) for scripting / `watch` — a **plain aligned table by default**,
  or machine-readable JSON with **`--json`** (array of the `Status` struct plus
  the provider name). Reuses the exact same `Status()` aggregation the TUI uses.
  The plain form is the default because the primary consumer is a human running
  `watch <bin> status`; `--json` exists from day one so the disk-only snapshot is
  scriptable without screen-scraping.

## Error handling

- **A provider's `Status()` errors for one profile** → render that row with an
  error marker in the Identity cell; never let one bad conf blank the whole
  table. Aggregation is per-profile fault-isolated.
- **Missing/blank `LastUsed`** → sort last (zero time), render `—`.
- **Stale/expired cached `Expiry`** → render `expired` (still no network; it's
  just a past timestamp). Refreshing validity is a drill-through concern.
- **No profiles anywhere** → friendly empty state pointing at how to create the
  first profile per provider.

## Testing

- **Contract-test-first:** add `Status()` exercise to
  `providertest.RunContract` so **every** provider (Azure + GitHub now; AWS/GCP
  as they land) is forced to implement it — asserting it returns populated
  identity/last-used from seeded temp dirs and, critically, that it makes **no
  network call** (verify by running with `az`/`gh`/`aws`/`gcloud` shimmed onto
  `PATH` as fakes that `exit 1` if invoked — a passing `Status()` must never
  touch them).
- **`LastUsed` + `LAST_DIR` unit tests** in `internal/profile`: `use`/login bumps
  both keys together (`LAST_DIR` = the bound dir); `Status()` reads them back;
  blank `LAST_USED` sorts last, blank `LAST_DIR` renders `""`.
- **Drift unit tests:** seed ambient env var vs pinned pointer; assert `Drifted`
  toggles correctly with no process spawn.
- **TUI model tests** (`internal/ui`): dashboard `View()` renders sorted rows,
  drift marker, relative expiry; `Enter` emits the "switch to tab + preselect"
  message; `[r]`/`[w]` trigger re-read; poll tick re-aggregates. Same
  `View()`-assertion pattern as the existing tab tests.

## Sequencing & phase placement

**Phase 5.5** — logically sits after Phase 5 (tab container) and is
**independent of** Phase 6 (CLI namespacing): it depends only on the tab
container existing and on having ≥2 providers to prove the aggregation against
(Azure + GitHub, both now shipped in #17), so it can land at any point after the
GitHub work. Doing it before AWS/GCP exist means `Status()` and the contract test
are already in place, so AWS/GCP are *forced* to implement the dashboard contract
the day they're written rather than being retrofitted.

## Deferred / open questions

- **History subsystem** — per-event / per-directory ledger beyond the single
  `LastUsed` timestamp (the README's "history of which account was active in
  which directory"). Explicitly a later phase.
- **Unified `[u]se`-from-dashboard** — tempting but rejected for v1 as it starts
  down the "inline write action" path B4 warns against; revisit only if a single
  provider-agnostic bind action proves unambiguous.

*Resolved during refinement (were open):*

- **Poll interval** → **3s default**, configurable via `DASHBOARD_POLL_SECS` in
  `azrl.conf`. (Midpoint of the 2–5s range; disk-only, so cost is negligible.)
- **CLI `status` output** → **plain aligned table by default, `--json` flag** for
  scripting, shipped together in the phase (not deferred) since JSON is trivial
  over the same `Status()` aggregation and avoids screen-scraping later.

## Roadmap position

Inserts into the `specs/github-remote-login.md` roadmap as **Phase 5.5**
(Phases 1–7 already shipped in #17):

```
5.   Tabbed TUI — tab container + GitHub tab.               (shipped, #17)
5.5. Status dashboard — Provider.Status(), LastUsed,        (this spec)
     default landing view, drill-through.
6.   CLI namespacing + alias entrypoints + goreleaser.      (shipped, #17)
7.   Docs + release.                                        (shipped, #17)
```
