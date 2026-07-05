# Browser-Profile Mapping — Design

**Date:** 2026-07-05
**Status:** Approved (brainstormed interactively)
**Ships as:** two branches — (1) the per-profile browser-command override substrate,
(2) the discovery + picker UX on top of it.

## Problem

The user's local machine has multiple browser profiles (Edge primarily, Chrome too),
each isolated to a specific work account. azrl launches the sign-in browser with one
global `LOCAL_BROWSER_CMD`, so every login opens the default browser profile — often
the wrong one for the credential being used. Users need each azrl profile to open its
matching browser profile, and a friendly way to set that mapping up without knowing
Chromium's internal `--profile-directory` names.

## Part 1 — Override substrate (ships first)

New optional per-profile conf key, provider-prefixed per convention:
`AZ_BROWSER_CMD`, `GH_BROWSER_CMD`, `AWS_BROWSER_CMD`, `GCP_BROWSER_CMD` — a local
browser command that overrides the global `LOCAL_BROWSER_CMD` for that profile's
logins. Purely additive: unset key = today's behaviour, byte-identical.

Mechanics (verified against the code):

- **Env hook in `config.LoadGlobal`** (internal/config/config.go, after validation):
  `AZRL_BROWSER_CMD` env var, when set, overrides `g.LocalBrowserCmd`. This single
  hook covers AWS/GCP (whose `Login` funcs load `Global` internally), the GitHub
  `__browser` shim (a child process of `gh`), and doubles as a universal user escape
  hatch. `internal/bridge`, `internal/browsercapture`, and `cmd/browser.go` need no
  edits — the browser command always executes on the laptop (path B
  `ssh <LocalHost> "<cmd> '<url>'"`, path A paste line), and every launch path
  already threads a `config.Global`.
- **Conf structs**: `BrowserCmd` field + LoadConf read + Write emit in
  `internal/profile/conf.go`, `internal/aws/conf.go`, `internal/gcp/conf.go`,
  `internal/github/conf.go`. Empty optional keys already serialize as `KEY=`
  (AZ_LABEL style); create paths need no changes.
- **Wiring**: Azure overrides `g.LocalBrowserCmd` directly in `cmd/login.go`
  (covers `bridge.Bridge` and the failure-reprint `PasteLine`, same `g` by value);
  `cmd/aws.go` / `cmd/gcp.go` `os.Setenv("AZRL_BROWSER_CMD", ...)` before their
  `Login`; `internal/github/login.go` appends `AZRL_BROWSER_CMD=` to the `gh`
  process env, which propagates to the `azrl __browser` shim.
- TUI "Sign in" needs nothing: `runHandoff` execs `azrl <provider> login` as a
  fresh subprocess.

**Known limitation (documented):** GCM auth prompts during plain `git push` run
outside any login, so the xdg-open shim falls back to the global command unless the
user exports `AZRL_BROWSER_CMD`. Possible follow-up: cwd-based `.ghprofile`
resolution in the shim. Out of scope.

## Part 2 — Discovery + picker UX

### Shared core: `internal/browserpick`

- `Discover(g config.Global) ([]BrowserProfile, error)` — sshes to `LOCAL_HOST`
  and reads each browser's Chromium `Local State` file from a candidate-path list:
  Edge and Chrome × Linux (`~/.config/microsoft-edge`, `~/.config/google-chrome`),
  macOS (`~/Library/Application Support/Microsoft Edge`, `.../Google/Chrome`),
  WSL (`/mnt/c/Users/*/AppData/Local/Microsoft/Edge/User Data`, `.../Google/Chrome/User Data`),
  native Windows (`%LOCALAPPDATA%` equivalents). One ssh invocation runs a small
  POSIX loop (`for f in ...; do echo "===$f"; cat "$f"; done`) covering
  Linux/macOS/WSL; if that yields nothing, a second `cmd`/`type`-based attempt
  covers native-Windows OpenSSH. Read-only, best-effort, no state on the laptop.
- Parsing `profile.info_cache` yields
  `BrowserProfile{Browser, DirName, DisplayName, Email, OS}` — e.g.
  `{edge, "Profile 2", "Work", "simon@contoso.com", linux}`. Edge and Chrome share
  the format, so both browsers are one parser.
- `Command(bp) string` renders the OS-appropriate launch string:
  - Linux: `microsoft-edge --profile-directory="Profile 2"` (Chrome:
    `google-chrome`)
  - macOS: `open -na "Microsoft Edge" --args --profile-directory="Profile 2"`
  - WSL/Windows: full quoted `msedge.exe` / `chrome.exe` path with the flag.
- No new dependencies: `encoding/json` + `exec`, testable with the repo's
  fake-ssh-on-PATH pattern.

### Storage

Picking writes two conf keys via an order-preserving `Scheme`-level helper (same
`readOrderedKV` → mutate → `writeAtomic` mechanics as `SetLabel`; added alongside
it, **not** a new `Provider` interface method):

- `<PREFIX>_BROWSER_CMD` — the launch command (the Part 1 key; this feature is a
  friendly front-end to it).
- `<PREFIX>_BROWSER_LABEL` — human display, e.g. `Edge — Work` (display-only; a
  hand-edited `_BROWSER_CMD` without a label still works everywhere).

Clearing the mapping removes both keys.

### CLI (console form + fallback)

New `browser` verb on each group: `azrl browser <profile>`,
`azrl gh browser <profile>`, `azrl aws browser <profile>`,
`azrl gcp browser <profile>`. Runs `Discover`, prints a numbered list with
identity matches sorted first — a browser profile whose signed-in email equals the
azrl profile's expected identity (`AZ_EXPECT_USER`, `GCP_EXPECT_ACCOUNT`,
`GH_USER`; AWS confs carry no email, so AWS lists are unsorted) — plus
`m) enter command manually` and `0) clear mapping`. Selection writes the keys and
confirms. Laptop unreachable / nothing found → drops straight to the manual prompt.

Naming note: the visible `browser` verb coexists with the hidden `__browser` /
`__browser-capture` self-shims; the underscore prefix already marks those as
internal, and none of them collide in Cobra.

### TUI (picker overlay)

- Each provider tab's ACTIONS radio gains **`b` Browser profile…** (hint reflects
  state: "map to a local browser profile" / current label when set).
- Trigger → busy spinner + `Discover` in an async `tea.Cmd` closure returning a
  result msg (never `tea.ExecProcess` — no screen blanking, event loop never
  blocks; precedent: `runUse`/`runRelabel` closures).
- Result → centered overlay picker: dirpicker's fuzzy-filter shell (input +
  candidates + `fuzzyScore` + `(model, result, closed)` update contract) rendered
  through `overlayCenter` like the options popup. Rows like
  `Edge — Work  simon@contoso.com`, identity matches on top, fuzzy-filterable.
  Enter writes both keys and closes with a status-line confirmation; esc cancels.
- Discovery failure or empty list → inline text-input fallback (New profile's
  naming-state pattern) for a raw command.
- DETAILS sheet gains a `browser` row showing the label (or the raw command when
  only `_BROWSER_CMD` is set by hand).

### Error handling

Discovery is best-effort and read-only. ssh failure, missing files, malformed
JSON, and unknown OS all degrade to manual entry — never a blocking error in the
TUI. Every failure mode leaves confs untouched.

## Testing

TDD-first, existing patterns:

- **Parser**: fixture `Local State` JSONs (Edge + Chrome, several profiles, one
  without an email) → parsed `BrowserProfile` sets; malformed JSON → error.
- **Discover**: fake `ssh` on PATH echoing fixture output; assert the probe
  command shape and the parsed result; ssh exit-1 → error.
- **Command generation**: table test per OS/browser.
- **Conf round-trips**: `BrowserCmd` (+ label key) per provider conf.
- **Substrate wiring** (Part 1): `LoadGlobal` env-hook test; per-provider login
  tests asserting the profile's command (not the fixture's `wslview`) reaches the
  fake-ssh log / paste line; github fake-`gh` env assertion; `__browser` shim
  env-override test.
- **CLI**: `browser` verb driven with scripted stdin over fake ssh; assert conf
  writes and the clear/manual paths.
- **TUI**: model tests asserting the action renders, the overlay `View()` shows
  discovered rows, selection writes both keys, esc/failure paths.

## Out of scope

- GCM push-time browser routing (documented limitation, possible `.ghprofile`
  follow-up).
- Firefox (different profile store; add if ever requested).
- OAuth `login_hint` URL injection (separate idea from the same discussion —
  complements this, not required by it).
- Auto-suggesting a mapping at `login`/profile-creation time.
