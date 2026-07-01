# Goal Prompt — GitHub Remote Login

**Spec:** `specs/github-remote-login.md` (read it first; it is authoritative).

## Objective

Add GitHub support to the `azrl` tool: a single **provider-aware binary** with a
**tabbed TUI** (Azure | GitHub) that lets a user on a headless/remote VM manage
and sign into multiple GitHub accounts (github.com + GHE Cloud `*.ghe.com`;
GHE Server as host-agnostic bonus), popping the browser on their local machine
for all three surfaces: `gh` CLI (relay), git-HTTPS via GCM (SSH tunnel), and
VS Code (tunnel). Reuse azrl's `Bridge`, browser-shim, profile model, and UI;
leave Azure behaviour unchanged.

## Hard gate — do Phase 0 first

**Phase 0 is a spike and gates all implementation.** Verify, then confirm or
revise the design before writing feature code:

1. `gh` token storage: keyring vs `hosts.yml` under `GH_CONFIG_DIR`; is
   per-profile isolation clean? force file storage if needed.
2. GCM honors `$BROWSER` on Linux (or what config makes it call our shim)?
3. GCM's authorize URL carries `redirect_uri=127.0.0.1:PORT` (parseable port)?
4. Two same-host accounts: per-repo credential resolution picks the right token.
5. VS Code GitHub auth: honors `$BROWSER` or self-handles → support vs document.

Produce a short findings note. Items needing a real laptop+VM+GCM+VS Code that
can't be verified in this environment must be called out explicitly, not
assumed.

## Build phases (after Phase 0 confirms)

1. Extract shared packages — `internal/bridge`, `internal/browsercapture`,
   parameterized `internal/profile`. **azrl green throughout** (`go build ./...`,
   `go test ./...`, `gofmt -l .`).
2. `Provider` interface; move Azure behind it with zero behaviour change.
3. `internal/github` — profiles, login (relay), use (+ `gh auth setup-git`),
   switch, `AssertAccount`.
4. Smart `__browser` shim — classify (device→relay, loopback→tunnel), wire
   GCM/VS Code per Phase 0 findings.
5. Tabbed TUI — tab container + GitHub tab reusing the profile/action layout.
6. CLI namespacing (`az …`/`gh …`) + `azrl`/`ghrl` alias entrypoints +
   goreleaser targets.
7. Docs + release.

## Constraints

- Conventional commits, scope `ui`/`github`/`bridge`/`profile` as apt.
- Test pattern: shim `gh`/`ssh`/`git` onto `PATH` with fakes; provider contract
  test; TUI model tests. No new deps unless justified.
- Do not regress azrl's Azure flow or its released CLI surface.
- Keep units small and single-purpose (Provider interface boundaries).

## Acceptance criteria

- `go build ./...`, `go test ./...` green; `gofmt -l .` clean at every phase.
- Bare invocation shows a tabbed TUI; GitHub tab lists/creates/uses/switches
  profiles with isolated `GH_CONFIG_DIR`.
- `gh login` relays the device code to the local browser; a git-HTTPS push from
  a pinned repo bridges the GCM loopback (verified per Phase 0 capabilities).
- Existing `azrl login`/`use`/etc. still work via the alias entrypoint.

## Out of scope (v1)

SSH-key git auth; long-lived bridge daemon; GHES-on-private-network reachability
(document); a separate GitHub-only binary.
