# Goal Prompt — Status Dashboard ("who am I, everywhere")

> Status: implemented (Phase 5.5, branch `feat/status-dashboard`).

**Spec:** `specs/status-dashboard.md` (read it first; it is authoritative).

## Objective

Add a top-level **status dashboard** as the default landing view of the
provider-aware TUI. It iterates every registered provider and shows, per profile,
identity / last-bound directory / expiry / ambient drift / last-used — a single
glanceable "who am I, everywhere?" table, sorted by most-recently-used. It is
**composition over the `Provider` interface**, not new auth logic: one new
interface method (`Status()`), one persisted timestamp (`LastUsed`), one new TUI
view. Per-provider tabs and action panes are unchanged; the dashboard drills
through to them. Provable today against two shipped providers (Azure + GitHub).

## Hard constraints (design-defining, not optional)

1. **`Status()` reads local cache/config only — zero network calls.** No
   `az account show`, `gh api user`, `aws sts get-caller-identity`, or
   `gcloud auth list`, ever. The dashboard polls on a 2–5s timer and must be safe
   to leave open indefinitely; a network call is a bug. Enforce this in the
   contract test by shimming the provider CLIs as fakes that fail if invoked.
2. **Read-only + drill-through, no inline write actions.** `Enter` jumps to the
   provider's tab with the profile pre-selected; the existing scoped action pane
   takes over. The only inline actions are the global read-only `[r]` (refresh
   all) and `[w]` (recheck drift). Do not put per-provider action menus on rows.
3. **One `LastUsed` timestamp per profile, not an audit log.** Persist a single
   RFC 3339 key in the profile conf; bump it on use/login/dir-bind via the shared
   `internal/profile` writer. A full history subsystem is out of scope for v1.

## Build steps

1. **Contract-test-first:** extend `providertest.RunContract` to require
   `Status()` — assert it returns populated identity/last-used from seeded temp
   dirs, and that it makes **no** network call (provider CLIs shimmed to `exit 1`
   on `PATH`). Watch it fail for the current Azure provider.
2. Add `Status` struct + `Status(name, confdir string) (Status, error)` to
   `internal/provider.Provider`. Implement it for Azure (read MSAL cache /
   `AZURE_CONFIG_DIR`, compute drift from ambient env vs pinned pointer). Then
   for GitHub (read `hosts.yml` under `GH_CONFIG_DIR`).
3. Add `LastUsed` + `LAST_DIR` persistence to `internal/profile`: bump **both**
   keys together on `use`/`login`/`capture`/dir-bind (`LAST_DIR` = the bound dir);
   `Status()` reads them back; blank `LAST_USED` sorts last.
4. Introduce `provider.All() []provider.Provider` (one ordered slice: Azure,
   GitHub) and repoint the existing hardcoded `internal/ui/tabs.go` slice at it —
   the dashboard, tab bar, and future providers then share one list. Then build
   the dashboard TUI view (sibling to the tabs, owned by the Phase 5 tab
   container): aggregate `provider.All()` → sorted table (by `LastUsed` desc),
   drift marker, relative expiry, `tea.Tick` poll at **3s default**
   (`DASHBOARD_POLL_SECS` in `azrl.conf` overrides), `Enter` → switch-tab +
   preselect, `[r]`/`[w]` read-only refresh.
5. Make the dashboard the **default landing view** for the bare unified
   invocation; keep alias entrypoints (`azrl`/`ghrl`) preselecting their tab for
   back-compat.
6. `<bin> status` one-shot CLI printing the same aggregation — plain aligned
   table by default, `--json` for scripting.

## Constraints

- **Strict TDD** — red/green/refactor; no production code without a failing test
  first. Use `superpowers:test-driven-development`.
- Conventional commits, scope `provider`/`profile`/`ui` as apt.
- Test pattern: seed temp config dirs; shim provider CLIs onto `PATH` as fakes;
  provider contract test; TUI model `View()` tests.
- Do not regress Azure or GitHub behaviour; `Status()` must be additive.
- Keep `Status()` cheap and side-effect-free.

## Acceptance criteria

- `go build ./...`, `go test ./...` green; `gofmt -l .` clean.
- `providertest.RunContract` requires `Status()`; Azure + GitHub both pass,
  including the no-network assertion.
- Bare unified invocation lands on the dashboard; it lists Azure + GitHub
  profiles in one table sorted by last-used, shows drift, and refreshes on a
  disk-only timer.
- `Enter` drills through to the correct provider tab with the profile
  preselected; `[r]`/`[w]` refresh without any network call.

## Exit — auto-ship

When the Definition of Done is met and everything is green, **auto-ship** via
`mad-skills:ship` — commit/push, open the PR, wait for CI, squash-merge, sync
`main`. Report the merged PR URL. Skip shipping only if CI can't be made green.

## Out of scope (v1)

Full audit-log/history subsystem; network-backed liveness/validity checks;
inline per-provider write actions on rows; cross-provider unified-profile
binding.
