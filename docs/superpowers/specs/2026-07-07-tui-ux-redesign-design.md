# TUI UX Redesign — one action model, ephemeral shells, web consoles, mouse — Design

**Date:** 2026-07-07
**Status:** Approved (brainstormed interactively)

## Problem

azrl's job is keeping a developer's terminal identity and browser identity
pointed at the same credential with minimal ceremony. The TUI has grown into
that job unevenly:

- The Azure tab (`internal/ui/model.go`, ~1050 lines) and the shared provider
  view (`provider_view.go`) are **two implementations of the same screen**, and
  they have drifted: different keys for the same verb (`l` vs `s` Sign in,
  `i` vs `a` New profile), Remove confirms on Azure but **deletes instantly**
  on the other tabs, and Azure-only verbs (Capture/Edit/Rename) the other
  providers' backends support but their tabs never expose.
- Actions are **hidden** when a heuristic says they don't apply (`sessionLive`
  hides Sign in; a cwd link hides Use here). The heuristic can't distinguish
  "live" from "can't tell" (nil expiry ⇒ live forever), and a hidden verb
  reads as a missing feature — the incident that motivated this redesign: a
  user needed to sign in as a second account without disturbing the
  directory's association and concluded the TUI "wouldn't let" them.
- Even visible, no verb serves that scenario: **azrl has no ephemeral-use
  verb**. Sign-in fills the isolated config dir but nothing points a terminal
  at it except "Use here", which writes the association.
- Expiry countdowns render on every row for every provider, but for Azure/GCP
  the tracked value is the *access token*, which the native CLI refreshes
  silently on next use — the number is telemetry, not guidance.
- The five-tier scope icon overloads one column with two dimensions
  (relevance-to-this-dir and the profile's own linkage bookkeeping) and needs
  a five-entry legend to decode.
- Mouse support is zero (`run.go` enables no mouse mode).

## Design principles

1. **Dashboard is the command center; tabs are the detail view.** You land
   where the answers are and can act where you land.
2. **Never hide — disable with reason.** Every verb is always listed; an
   inapplicable one renders dim with its reason as the hint. Menus that never
   change shape are learnable.
3. **One keymap.** Same key ⇒ same verb on the dashboard, every tab, and help.
4. **CLI-first.** Every verb is a real `azrl` subcommand; the TUI execs it
   (the existing `runHandoff` pattern). Scripting parity is automatic.
5. **Display only what's actionable.** Absence of a marker means healthy /
   not in play. State that demands nothing displays nothing.
6. **Mirror, never actor** (existing invariant, PAT-002): azrl never refreshes
   tokens, never mutates native defaults; status stays disk-only.

## Terminology: "pin" → "link"

"Pin" reads as PIN-based login in an auth tool. All UI/docs language changes
to **link** ("this directory is linked to `work`"): action *Link here* (was
Use here), hints "link this dir — no login" / "sign in + link this dir" /
"links this dir", disabled reason "already linked here", DETAILS row
**Linked**, dashboard hint "no directories linked yet", "governing link".
Internal names (`.azprofile`, the mappings TSV, `Touch`) are unchanged; the
dashboard MAPPINGS section keeps its name (a link is one row in it).

## The shared action model

One verb set on all four tabs, one implementation (see Unification):

| Key | Verb | Behavior |
|---|---|---|
| `s` | Sign in | Always visible. Live session ⇒ still runnable (idempotent), hint "session live · re-auth anyway". |
| `t` | Shell as… | Ephemeral use (below). |
| `c` | Open console | Web console as this credential (below). |
| `u` | Link here | Writes the dir association, no login. Disabled on the already-linked selection ("already linked here"). |
| `n` | New profile | Inline name input → `login <name> --yes` (sign in + link, as today). |
| `b` | Browser profile | Unchanged discovery/picker flow. |
| `delete` | Remove | **Confirm dialog on every tab** (Azure's dialog becomes shared). |

`visibleActions` is replaced by `enabledActions`: the full list always
renders; each action carries `enabled bool` + `reason string`. Running a
disabled action (key, enter, or click) shows the reason in the status line.

**Capture becomes contextual** (it's an onboarding/discovery verb, not an
everyday one):

- **Empty-state, all four tabs** (today Azure-only): actions collapse to
  New profile + Capture. Capture gains the same inline name input New profile
  has, prefilled with `profile.DefaultName("", pwd)`, hint "adopt current CLI
  session · links this dir" — today the TUI runs `capture` with no name and
  silently auto-names + links cwd.
- **Dashboard adopt, extended**: today only the GitHub git-config detector
  produces an adoptable row. Unmanaged **AMBIENT** rows (any provider,
  `Profile == ""`, already computed) also get `[a]dopt`. Adopt prompts with a
  prefilled name instead of silently naming after the directory.

**Edit… and Rename… retire from the TUI** (CLI/`$EDITOR` cover them).

Container keys: `tab`/`shift+tab`/`[`/`]` tabs, `d` change dir, `o` options,
`r` **and** `f5` refresh, `?` **help overlay on every screen** (centered full
keymap; today `?` is an Azure-only footer toggle), `q`/`ctrl+c` quit, `esc`
back, `j`/`k` retained. `a` = adopt (contextual). The `e` write-.envrc hotkey
stays on the drift notice line.

## Shell as… (`azrl shell`)

The ephemeral-use verb — act as another profile in a terminal without
touching any directory link.

**CLI:** `azrl shell <name>`, `azrl gh|aws|gcp shell <name>` (ghrl promotes
the gh verb). Flow: resolve profile → if the session is dead, run the normal
login flow (bridge, browser mapping, everything) → exec `$SHELL` with the
profile's env map:

- Azure `AZURE_CONFIG_DIR=~/.azure-profiles/<name>` · GitHub `GH_CONFIG_DIR`
  · AWS `AWS_PROFILE` (or the isolate file vars when `AWS_ISOLATE`) · GCP
  `CLOUDSDK_ACTIVE_CONFIG_NAME` (or `CLOUDSDK_CONFIG` under isolate) — the
  same values the `.envrc` writers emit.
- `AZRL_BROWSER_CMD` from the profile's browser mapping, so `git push` / `az
  login` *inside* the subshell routes to the right browser profile
  (narrowing the documented GCM limitation).
- `AZRL_PROFILE=<provider>:<name>` — the marker powering all indication.

Exit status passes through. Nested shells warn and proceed (innermost wins —
it's just env).

**Indication** (a requirement): an entry banner (`azrl: shell as work (azure)
— 'exit' returns`); the `AZRL_PROFILE` variable with a documented one-line
prompt snippet for bash/zsh/starship; the TUI header shows an override chip
`⌁ shell: work` instead of misreporting drift; `azrl status` reports the
override. azrl does **not** rewrite arbitrary shells' prompts (compatibility
tarpit; the banner + variable is honest).

**TUI:** `t` suspends via `runHandoff`-style exec; exiting the subshell
returns to the TUI, which reloads.

Rejected alternatives (documented non-goals): one-shot `azrl run` wrapper and
eval-able `azrl env` printer (same core, add if demand shows); TTL sidecar
link override (`.azprofile.override`) for the "every new terminal" case —
revisit only if that case materializes.

## Open console (`azrl console`)

Opens the provider's web console **as the selected credential**, deep-linked
from data already in the profile conf:

- Azure: `https://portal.azure.com/#@<AZ_TENANT>` (tenant-scoped)
- AWS: the profile's `AWS_SSO_START_URL`
- GCP: `https://console.cloud.google.com/?project=<GCP_PROJECT>&authuser=<account>`
- GitHub: `https://<GH_HOST>`

Launch reuses the existing browser plumbing verbatim: profile
`*_BROWSER_CMD` override → global `BROWSER_CMD`; local mode (`IsLocal()`)
launches directly, remote goes over the SSH path the login bridge uses;
failure falls back to printing the URL. CLI verb + `c` in the TUI and on
dashboard rows. This closes the terminal+web loop: the browser-profile
mapping stops being login-redirect-only.

## Expiry: guidance, not telemetry

What azrl tracks differs in kind per provider, and the display now says so:

- **AWS** — the SSO session token (~8h); when it dies, `aws sso login` is
  genuinely required. Full three-state display: healthy ⇒ nothing; expiring
  soon (< 15 min) ⇒ small amber marker, only on rows in play (linked to a dir
  or the ambient default); expired ⇒ `expired` tag on in-play rows + the
  dashboard hint (`s` sign in).
- **Azure / GCP** — the tracked value is the *access token*, which `az` /
  `gcloud` refresh silently from their stored refresh tokens on next use.
  Rows show **nothing about it, ever**. The DETAILS pane tells the truth for
  a deliberate check: `token stale · refreshes on next use` (or the expiry
  timestamp while live). Caveat, documented: a Conditional-Access
  sign-in-frequency policy can kill the refresh chain; that state is
  invisible from disk and surfaces as a failed command — remedy `s`.
- **GitHub** — no expiry exists; nothing shown (as today).

Exact timestamps live in exactly one place: DETAILS. Tracking underneath is
unchanged (disk-only, powers the hint + amber threshold). azrl never
refreshes proactively — the CLIs already do it lazily, and doing it from a
status screen would break the disk-only invariant. The dashboard's
expired-governing-link hint (#79) effectively becomes an AWS-only signal.

## Scope marks: one column, one meaning

The icon slot means **relevance to this directory** and nothing else:

- ● green — linked to this directory
- ● orange — linked via a parent directory
- *(empty)* — everything else

The ambient default stops masquerading as a scope: 🌐 leaves the column and
becomes a **text tag** at the end of its row — `work@contoso  ⌁ default` —
same visual family as the `expired` tag. In a linked directory the pane now
reads: green row = what this dir intends; `default` row = what a bare shell
falls back to; if those differ with no `.envrc` bridging them, that is the
drift the header warns about — pane and warning tell one story.

The elsewhere/nowhere grey dots are **deleted**. "Where is this profile
used" moves to a new DETAILS row **Linked** — the directories from the
mappings index (`~/work/azrl + 2 more`) — answering "what breaks if I switch
this" on demand. The legend shrinks to two dots + one tag; bold still marks
the row effective here. Dashboard MAPPINGS keeps only the green
governing-row marker; AMBIENT rows carry the `default` tag natively.

## Dashboard as command center

- Cursor starts on the row governing the cwd (today: top row).
- **Every row accepts the verb keymap** (`t`/`c`/`s`/`u`/`b`, `a` where
  adoptable), acting on that row's profile. `enter` still drills into the tab.
- Sections, fsnotify liveness, and hint prioritization stay as shipped.

DETAILS pane rows: Name · Identity · Detail · Browser · **Linked** · Expiry
(per-provider semantics above) · Last used · Drift.

## Unification (the enabler)

The Azure `Model` folds into `providerTabView`; the azure tab becomes a thin
wrapper like `aws_view.go`. The shared view grows the hooks Azure's
remaining specialness needs: an optional notice line (drift warning + `e`
envrc hotkey) and the identity strip header. Azure's confirm dialog and
two-line profile rows are promoted to shared code. Expected deletion:
600–800 lines of `model.go`. This is what makes the
two-implementations bug class (the original drift) structurally dead — every
new verb, key, and mouse zone is written once.

The `azrl setup` wizard (#85–#89) is a separate Bubble Tea program and is
untouched by the unification; it adopts mouse zones in the mouse phase if
cheap, otherwise later.

## Mouse

`tea.WithMouseCellMotion` in `run.go`; hit-testing via **bubblezone** (new
dependency — justified: manual coordinate math across composed lipgloss
layouts, written once per redesign, is exactly the fragile duplication being
deleted). Semantics:

- Click selects (tab cell, profile row, action row, dashboard row, overlay
  row). Click-again or double-click runs. Wheel scrolls the list under the
  pointer. Click outside an overlay dismisses it. Clicking a disabled action
  shows its reason in the status line.
- Help documents that terminal text-copy needs Shift while azrl is open.

## Phasing (independently shippable PRs)

1. **Unify + action model** — fold Azure into `providerTabView`; shared
   confirm; `enabledActions`; new keymap; contextual capture (empty-state +
   dashboard adopt extension); link language; expiry semantics; scope-mark
   simplification; `?` overlay. (Split further at planning time if too big.)
2. **`azrl shell`** — CLI verb (all providers), env map, indication, TUI `t`.
3. **`azrl console`** — CLI verb, URLs, browser plumbing reuse, TUI `c`.
4. **Mouse** — bubblezone, zones, semantics, docs.
5. **Dashboard verbs on rows** — keymap dispatch from dashboard items (rides
   with 1 if small).

## Error handling

- Shell: dead session → login flow first; login failure aborts the shell
  (no half-authenticated subshell). Missing `$SHELL` → `/bin/sh` fallback.
- Console: no mapped browser and no global command, or launch failure →
  print the URL (never an error state).
- Adopt/capture name collisions: the existing "already exists — remove it
  first" error surfaces in the TUI status line; the name input lets the user
  pick another before running.
- Disabled actions never error — they explain.

## Testing

Existing patterns throughout: `View()` assertions and message-driven model
tests for the TUI (mouse via injected `tea.MouseMsg`); PATH-shimmed fake
`az`/`gh`/`aws`/`gcloud`/`ssh` and a fake `$SHELL` executable for shell/
console flows (assert env map contents from the fake's log); provider
contract suite unchanged (no `Provider` interface changes). Real-machine
items (actual subshell ergonomics, browser-profile console launches, WSL)
extend the manual-verify checklist.

## Out of scope

- `azrl run` / `azrl env` ephemeral variants; TTL sidecar link override.
- Proactive/background token refresh (mirror-not-actor).
- Prompt rewriting inside the subshell.
- Managing the native default identity (rejected — see
  `docs/ambient-identity-model.md`).
- GKE `gke-gcloud-auth-plugin` isolation gap (existing documented v1 warning).
