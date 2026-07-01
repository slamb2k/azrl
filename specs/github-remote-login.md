# GitHub Remote Login — Design Spec

- **Date:** 2026-07-01
- **Status:** Approved (brainstorm complete); Phase 0 spike gates implementation
- **Author:** brainstormed with Claude
- **Related:** extends the existing `azrl` (Azure Remote Login) tool

## Summary

Add first-class GitHub support to the `azrl` tool so a user on a headless/remote
VM can log into and operate multiple GitHub accounts — **github.com**, **GHE
Cloud (`*.ghe.com` / EMU)**, and (bonus) **GHE Server** — with the sign-in
browser popping on their local machine. The tool consolidates into a single
**provider-aware binary** with a **tabbed TUI** (Azure | GitHub | …), reusing
azrl's SSH browser-bridge, profile model, and UI.

## Motivation & key finding

`azrl` exists because Azure Conditional Access can block device-code flow,
forcing a browser + a random localhost OAuth **callback port** that must be
forwarded from the VM to the laptop over SSH. GitHub is different:

- `gh auth login` uses the **OAuth device flow** (one-time code + poll, no
  localhost callback). It already works headless and **has no
  Conditional-Access-style kill switch** — orgs/enterprises cannot disable
  device flow. So for `gh` itself, the SSH port-forward bridge is unnecessary;
  we only need to **relay** the code/URL to the laptop.
- The genuine gap is **loopback-callback tools** that DO use `127.0.0.1:PORT`:
  - **Git Credential Manager (GCM)** for `git push/pull` over HTTPS
  - **VS Code** built-in GitHub auth (URI-handler / loopback)
  - `cli/oauth`'s web-app-flow fallback (older GHES)
  - custom OAuth/GitHub Apps with a loopback `redirect_uri`

The user interacts with GitHub on the VM via **`gh` CLI + git-HTTPS + VS Code**
(not SSH-key git). So the tool must do **both**: manage accounts *and*
transparently bridge loopback callbacks.

## Goals

- Manage multiple GitHub accounts on a remote VM: per-account isolated config,
  fast switching, per-repo pinning.
- Sign in with the browser on the *local* machine: `gh` (relay via `$BROWSER`),
  git-HTTPS via GCM (tunnel via `xdg-open` shadow). VS Code needs no work —
  Remote-SSH already handles its GitHub sign-in.
- Support github.com and GHE Cloud (`*.ghe.com`) as primary; GHE Server
  (`--hostname`) as a host-agnostic bonus.
- Reuse azrl's `Bridge`, browser-shim, profile model, and TUI; leave azrl's
  Azure behaviour unchanged.

## Non-goals (v1)

- SSH-key git auth (no browser involved — out of scope).
- Engineering around GHES-on-private-network reachability (documented gotcha).
- A long-lived bridge daemon (Approach B) — rejected for complexity.
- Independent GitHub-only distribution with no Azure code (superseded by the
  tabbed TUI; see Architecture).

## Architecture

### Provider-plugin core

Define a `Provider` interface implemented per account type:

```
Name() string
ListProfiles() ([]Profile, error)
Login(profile) error
Use(profile, dir) error      // pin a repo to a profile + wire credentials
Switch(profile) error
AssertAccount(profile) error
// plus provider-specific bridge/host details
```

`internal/azure` and `internal/github` each implement it. TUI tabs and CLI
dispatch are written **once** against the interface; a future third account
type is one new self-registering package.

### One binary, tabbed TUI

A **tabbed TUI** is a single Bubble Tea process that owns the terminal
alt-screen; rendering an Azure tab and a GitHub tab requires both providers
linked into one binary. Therefore the tool consolidates to **one
provider-aware binary** rather than two independent binaries. A second binary
would be pure redundancy (once the binary imports `internal/github` it already
contains all GitHub logic) and cost double packaging for zero code separation.
The isolation we want lives at the **package boundary** (`Provider` interface),
not the binary boundary.

- Bare invocation → tabbed TUI, one tab per registered provider.
- Each tab reuses the polished profile-list + action-pane layout as a
  per-provider view. Tab switch keybind (e.g. `[`/`]` or `1`/`2`).

### Shared packages (extracted from today's azrl; Azure behaviour unchanged)

- `internal/bridge` — SSH reverse-tunnel (`ssh -R` path B zero-paste, `ssh -fNL`
  path A paste) + local-host reachability, lifted from `internal/azure`.
- `internal/browsercapture` — the smart `BROWSER` shim: classify a URL and
  relay-or-tunnel (promoted from the hidden `__browser-capture`).
- `internal/profile` — parameterized (config-dir root, profile filename, conf
  keys) so azrl keeps `.azprofile`/`AZ_*` and ghrl gets `.ghprofile`/`GH_*`.
- `internal/config` — global host/browser config (already shared).
- `internal/ui` — gains a tab container hosting provider views.

### New packages

- `internal/github` — the `gh`/GCM lifecycle (`Login`, `Use`, `Switch`,
  `AssertAccount`, host handling).
- `cmd/*` — provider-namespaced Cobra tree + hidden `__browser` shim.

## Account / profile model

Direct analogue of azrl's model.

- **Profile store** — `~/.github-profiles/<name>.conf`:
  - `GH_HOST` — `github.com`, a `*.ghe.com` tenant, or GHES hostname (`gh
    --hostname`)
  - `GH_USER` — expected login, for post-auth assertion
  - `GH_LABEL` — optional display name (reuses the `*`-marker / rename-via-label)
  - `GH_PROTOCOL` — `https` (GCM/bridge path) or `ssh` (informational)
- **Isolated auth per account** — `GH_CONFIG_DIR = ~/.github-profiles/<name>/`.
  `gh` honors `GH_CONFIG_DIR`, giving each profile its own `hosts.yml`/token —
  mirror of azrl's per-profile `AZURE_CONFIG_DIR`. **Spike correction:** `gh`'s
  default credential store is the OS keyring, which is *global* (keyed by
  host+user, ignoring `GH_CONFIG_DIR`). So `Login` must call
  `gh auth login --insecure-storage` to force the token into the per-profile
  `hosts.yml`, which is what actually gives clean per-account isolation.
- **Repo pin** — `<repo>/.ghprofile` (one line, gitignored), resolved by walking
  up from `$PWD` (reuses the extracted resolver). A repo may carry `.azprofile`
  and `.ghprofile` independently.
- **Activation (`gh use`)** — writes `.ghprofile`, runs `gh auth setup-git`
  scoped to that profile's `GH_CONFIG_DIR`, **and** sets the repo-local
  `credential.https://<host>.username = GH_USER` (GCM's prescribed multi-account
  method), so `git push/pull` over HTTPS resolves to *this account's* token even
  when two accounts share a host.
- **Assertion** — `gh api user` under the profile's `GH_CONFIG_DIR`/`GH_HOST`
  confirms the login matches `GH_USER`.

## Browser-shim + bridge (the core mechanism)

A single hidden shim subcommand — `<bin> __browser <url>` — receives whatever URL
a tool tries to open, classifies it, and relays-or-tunnels. **Spike-confirmed
install points differ per tool:**
- `gh` honors `$BROWSER` → set `BROWSER=<bin> __browser`.
- **GCM does NOT honor `$BROWSER` on Linux** — it execs `xdg-open` → the shim
  must **shadow `xdg-open`** on the session `PATH` (a small wrapper that forwards
  to `<bin> __browser`). (Alternatively `GCM_GITHUB_AUTHMODES=device` forces
  device flow and skips the loopback entirely — kept as a fallback, but shadowing
  `xdg-open` is the default so git-HTTPS gets the zero-paste bridge.)

Whenever a tool opens a browser it invokes the shim, which classifies and acts:

- **Loopback OAuth** — URL carries `redirect_uri=http://127.0.0.1:PORT/…` (or
  `localhost:PORT`). Parse `PORT`, then reuse `Bridge`:
  - Path B (zero-paste): VM runs `ssh -R PORT:localhost:PORT <local-host>` and
    launches the laptop browser at the authorize URL; the laptop redirect to
    `127.0.0.1:PORT` tunnels back to the VM's listener.
  - Path A (paste): print the one-line `ssh -fNL PORT:… <vm>` for the user.
  - Tunnel held for a bounded auth window (~180s), then torn down. No daemon.
- **Device / plain** — no loopback redirect (e.g. `github.com/login/device`).
  Open the URL on the laptop via the local-browser mechanism (`LOCAL_BROWSER_CMD`
  over SSH in path B; print for paste in path A). VM tool polls for the token.

Surfaces after the spike: `gh` → relay (via `$BROWSER`); GCM → tunnel (via
`xdg-open` shadow); **VS Code → no bridge needed** — Remote-SSH already handles
GitHub sign-in through its own `vscode://` URI handler + `asExternalUri`, so we
document it as handled rather than intercept it. The shim is stateless per
invocation; global config
(`LOCAL_HOST`, `VM_HOST`, `LOCAL_BROWSER_CMD`) tells it how to reach the laptop.
GCM picks a fresh random port each time — hence parsing the port from the URL
rather than assuming a fixed port.

## Command surface & TUI

### TUI

- Bare invocation → tabbed TUI (`Azure` | `GitHub`), tab switch keybind.
- Each tab = profile-list + action-pane. Per-provider action parity:

| Action | Azure | GitHub |
|---|---|---|
| Sign in | `az login` bridge | `gh auth login` (relay/bridge) |
| Use here | write `.azprofile` | write `.ghprofile` + `gh auth setup-git` |
| Capture session | record current `az` | record current `gh` auth |
| New profile | init + login | login + record |
| Edit / Rename / Remove | as today | same (label `*`-marker reused) |

### CLI

- GitHub is grouped: `<bin> gh login [--hostname H] [profile]`, `gh use`,
  `gh list`, `gh switch`, `gh rm`, `gh capture`, `gh status`. Hidden
  `<bin> __browser <url>` shim.
- **As shipped (deviation from the original plan):** Azure was **not** moved
  under an `az …` group — its verbs (`login`, `init`, `capture`, `use`, `rm`,
  `list`) stay **top-level** so the existing `azrl login` surface is preserved
  unchanged, and only GitHub is namespaced (`gh …`). This kept back-compat without
  a compatibility-shim layer; a symmetric `az` group can be added later if the
  asymmetry ever grates. (AWS/GCP in Phases 8–9 follow the *grouped* `gh` pattern:
  `<bin> aws …` / `<bin> gcp …`.)
- **Back-compat:** thin alias entrypoints — `azrl` (top-level Azure verbs, as
  today) and `ghrl` (preselects GitHub, `gh` verbs promoted to top level). Bare
  `azrl`/`ghrl` → tabbed TUI on the relevant tab.
- **Unified binary name:** deferred (keep `azrl` as primary vs a neutral name).

## Error handling & GHES gotcha

- **Local host unreachable** → path B fails → auto-fallback to path A (paste).
- **Unparseable/absent `redirect_uri`** → default to relay; if wrong, the tool
  times out → surface a clear "no callback detected" error rather than hang.
- **Missing `gh`/GCM** → detect up front, actionable install hint.
- **Assertion mismatch** → clear error + offer re-auth (mirrors `AssertAccount`).
- **Tunnel/auth timeout (~180s)** → tear down forward, report.
- **GHES gotcha:** the tunnel forwards the callback, but the laptop browser must
  reach the GHES **authorize page**. A self-hosted GHES only reachable on the
  VM's private network requires the laptop to be on the same VPN. github.com and
  `*.ghe.com` are public, so this bites only self-hosted GHES — documented, not
  engineered around.

## Phase 0 — Spike (gates implementation) — RESOLVED

Findings in `specs/github-remote-login.spike.md`. **Design holds** with the
mechanical revisions folded into the sections above: (1) `gh auth login
--insecure-storage`, (2) shim shadows `xdg-open` for GCM (not `$BROWSER`), (3)
`Use` sets per-repo `credential.https://<host>.username`, (4) VS Code documented
as handled (no active bridge). Items still requiring a real laptop+VM+GCM+VS Code
to close (see `specs/github-remote-login.manual-verify.md`): end-to-end GCM push
through shim+tunnel, two-account no-cross-push proof, gh isolation on a host with
a working keyring, VS Code Remote sign-in with zero tunnel.

Original verification checklist (now answered):

1. **`gh` token storage** — keyring (global) vs `hosts.yml` under
   `GH_CONFIG_DIR`; confirm per-profile isolation; decide whether to force file
   storage for clean isolation.
2. **GCM browser hook** — does GCM honor `$BROWSER` on Linux, or need explicit
   config? (Must fire the shim, or git-HTTPS bridging fails.)
3. **GCM authorize URL** — confirm it carries `redirect_uri=127.0.0.1:PORT` so
   the shim can parse the port.
4. **Two same-host accounts** — per-repo credential resolution picks the right
   token (a repo pinned to account A never pushes as B).
5. **VS Code** — honors `$BROWSER` or self-handles via its own port-forwarding →
   decide active support vs document-as-already-handled.

## Testing

- **Same pattern as azrl:** shim `gh`/`ssh`/`git` onto `PATH` via
  `t.Setenv("PATH", tmpDir)` with fake executables.
- **Pure logic unit-tested:** profile resolution, URL classification
  (device vs loopback), port parsing from `redirect_uri`.
- **Provider contract test:** one shared suite both `internal/azure` and
  `internal/github` satisfy (guarantees tab/CLI parity).
- **TUI:** model unit tests for `View()`, tab switching, message handling.
- **Bridge:** fake `ssh` asserting the right `-R`/`-fNL` invocation per path.
- **Manual E2E:** the tmux-hosted TUI capture flow.

## Packaging / release

- Repo goes from one binary to a provider-aware binary + thin alias entrypoints.
- `main.go` → `cmd/<name>/` entrypoints; goreleaser gains build targets for the
  unified binary and the `azrl`/`ghrl` aliases (all share one codebase, ship in
  one release/tag; Homebrew installs all).
- Version continues to be injected from the tag via ldflags.

## Deferred / open questions

- Unified binary name (keep `azrl` primary vs neutral name).
- Whether global config is shared once (e.g. `~/.remote-login/config`) or stays
  per-provider (`~/.azure-profiles/`, `~/.github-profiles/`). v1: per-provider.
- Shared TUI polish parity (angel-wing banner for the GitHub tab?) — cosmetic,
  later.

## Roadmap (phases)

0. **Spike** (gating) — verify the five items above.
1. **Refactor/extract** shared packages (`bridge`, `browsercapture`, parameterized
   `profile`) with azrl green throughout.
2. **Provider interface** + move Azure behind it (no behaviour change).
3. **`internal/github`** — profiles, login (relay), use, switch, assert.
4. **Smart shim** — classify + relay/tunnel; install as `$BROWSER` (gh) + shadow
   `xdg-open` (GCM). VS Code needs no bridge (Remote-SSH handles it).
5. **Tabbed TUI** — tab container + GitHub tab.
5.5. **Status dashboard** — `Provider.Status()`, `LastUsed`, default landing
   view, drill-through. Depends only on the Phase 5 tab container; independent of
   Phase 6, so it can land at any point after the GitHub work. See
   `specs/status-dashboard.md`.
6. **CLI namespacing** + `azrl`/`ghrl` alias entrypoints + goreleaser targets.
7. **Docs + release.**

Phases 1–7 shipped in #17. The provider-aware roadmap then continues (scoped in
separate spec files):

5.5. **Status dashboard** — `specs/status-dashboard.md`.
8. **AWS provider** (`internal/aws`) — opens with the bridge-generalization
   audit (confirm `internal/bridge` + `internal/browsercapture` carry no
   Azure-isms; AWS/GCP loopback classification tests), then `aws sso login` PKCE
   bridge, device-code fallback, `AWS_PROFILE`/`.envrc`, opt-in file isolation,
   `sts get-caller-identity` guardrail. See `specs/multi-cloud-providers.md`.
9. **GCP provider** (`internal/gcp`) — `gcloud auth login` bridge (replaces
   `--no-browser`), named configs/`.envrc`, opt-in `CLOUDSDK_CONFIG`, GKE
   warning, `auth list` guardrail. See `specs/multi-cloud-providers.md`.
