# Design: azrl Go CLI/TUI rewrite

Date: 2026-06-30
Status: Approved (brainstorm)

## Goal

Rewrite the `azrl` (Azure Remote Login) Bash CLI as a single self-contained Go
binary with a first-class terminal UI. Migrate all current commands, preserve the
config/layout so existing profiles keep working, and add a visually engaging TUI
(launched by bare `azrl`) for managing and selecting profiles.

## Locked decisions (from brainstorm)

1. **Replace**, don't coexist. The Go binary becomes the one true `azrl`; the Bash
   `azrl` / `azrl-lib.sh` / `azrl-capture` and `tests/*.bats` are removed (kept in
   git history). The single binary also serves as its own `$BROWSER` capture shim.
2. **Cobra subcommands**, fully scriptable. Bare `azrl` (no args) launches the TUI.
   **No legacy flag aliases** — the old `--init/--capture/--use/--rm/--list/--paste`
   and deprecated `--save` spellings are NOT carried over. Clean subcommand surface.
3. **Native Go orchestration** of the login lifecycle (shell out to `az`/`ssh` via
   `os/exec`); the `$BROWSER` trick uses the binary itself as the shim
   (`azrl __browser-capture %s`). JSON parsed with `encoding/json` (no `jq`).
4. **Visuals**: Azure-blue + gold palette; a detailed multi-line angel ASCII art
   plus a FIGlet-style block "AZRL" banner.
5. **Go-native tests** mirroring current coverage (unit + PATH-shimmed integration
   + TUI model tests).
6. Config formats (`azrl.conf`, `<profile>.conf`) and `~/.azure-profiles/<name>/`
   isolation are **unchanged**.

## Runtime dependencies

- `az` (Azure CLI) and `ssh` (OpenSSH) required at runtime (as today).
- `jq` no longer required (JSON parsed natively).
- Go 1.24 toolchain to build.

## Module / package layout

Module: `github.com/slamb2k/azrl`

```
main.go                      entrypoint -> cmd.Execute()
cmd/
  root.go                    cobra root; bare invocation -> ui.Run()
  login.go                   azrl login [profile] [--paste]
  init.go                    azrl init [name]
  capture.go                 azrl capture [name]
  use.go                     azrl use <name>
  rm.go                      azrl rm <name> [-y]
  list.go                    azrl list
  browsercapture.go          hidden __browser-capture <url> (self-shim)
internal/
  config/                    global azrl.conf (LOCAL_HOST, LOCAL_BROWSER_CMD,
                             VM_HOST), path helpers (~/.azure-profiles), constants
  profile/                   pure logic + tests:
                               Resolve(arg, dir)         walk-up .azprofile
                               LoadConf(name, dir)       parse <name>.conf (AZ_TENANT req)
                               WriteConf(...)/SaveConf    build+write conf (atomic)
                               SanitizeName / DefaultName
                               List(confdir)             name + tenant, excl. azrl.conf
                               Use(name, confdir, pwd)   validated .azprofile write
                               Remove(name, confdir, pwd, yes)
                               ExtractPort(url)
  azure/                     orchestration + tests (shell out to az/ssh):
                               CleanSlate(cfgDir)
                               LoginCapture(tenant, capExe) -> url, port, *exec.Cmd
                               Bridge(port, url, cfg, forcePaste)
                               WaitForLogin(cmd, timeout, ...) recovery hint
                               AssertAccount(acctJSON, expTenant, expUser)
                               AccountShow() / SetSubscription()
  ui/                        bubbletea models + lipgloss styles:
                               app.go      root model, screen routing
                               list.go     profile list + context panel
                               forms.go    name input, confirm modal
                               run.go      long-op runner (spinner + status stream)
                               banner.go   go-figure AZRL banner (gradient)
                               angel.go    embedded angel art
                               styles.go   palette + styled components
install.sh                   go build -o ~/.local/bin/azrl . ; gitignore .azprofile ;
                             bootstrap azrl.conf
```

## Config parsing

`azrl.conf` and `<profile>.conf` are simple `KEY=value` shell-sourced files with
plain (space-free where it matters) values. A small parser reads `KEY=value` lines,
trims whitespace, ignores blanks/`#` comments. Writing keeps the exact current
format so files remain inspectable and backward compatible:

```
AZ_TENANT=<domain or guid>
AZ_TENANT_ID=<guid>
AZ_DEFAULT_SUB=<sub id>
AZ_EXPECT_USER=<upn>
```

Global: `LOCAL_HOST`, `LOCAL_BROWSER_CMD`, `VM_HOST`. Conf writes are atomic
(temp file + rename), as in the current Bash.

## Login lifecycle (ported faithfully)

1. `CleanSlate(cfgDir)` — `az logout`, `az account clear`, remove
   `msal_token_cache.json` and `service_principal_entries.json` within the scoped
   `AZURE_CONFIG_DIR` only.
2. `LoginCapture(tenant)` — temp capfile; run
   `az login [--tenant <t>] --allow-no-subscription --only-show-errors` with
   `AZRL_CAPFILE` set and `BROWSER="<selfpath> __browser-capture %s"`; poll the
   capfile for the URL; `ExtractPort` the random callback port. `--allow-no-subscription`
   is always passed (parity with current behavior).
3. `Bridge(port, url)` — if not `--paste` and `ssh -o BatchMode=yes -o
   ConnectTimeout=5 <LOCAL_HOST> true` succeeds: start `ssh -N -R
   port:localhost:port <LOCAL_HOST>`, verify it stays up, then
   `ssh <LOCAL_HOST> "<LOCAL_BROWSER_CMD> '<url>'"` (path B, zero-paste). Otherwise
   print the one-line `ssh -fNL ... && <browser> "<url>"` paste line (path A).
4. `WaitForLogin` — context deadline = `AZRL_LOGIN_TIMEOUT` (default 180s); kill the
   login process on timeout; on nonzero exit print the path-A recovery hint.
5. `AssertAccount` — parse `az account show -o json`; verify tenant by
   `tenantDefaultDomain` OR `tenantId` (GUID, for guest/B2B), and optional
   `AZ_EXPECT_USER`. Select `AZ_DEFAULT_SUB` first if set.

The self-shim subcommand `azrl __browser-capture <url>` writes `<url>` to
`$AZRL_CAPFILE` and exits 0, so MSAL believes a real browser launched (no
device-code fallback).

## CLI command behavior

- `azrl login [profile] [--paste]` — full login flow. Resolves profile from arg or
  `.azprofile`; if none, tenant-less sign-in into default `~/.azure` (current
  behavior) plus a one-line hint to run `azrl init <name>`. (Rich interactive
  create/link lives in the TUI, not the CLI.)
- `azrl init [name]` — tenant-less login, then write `<name>.conf` + `.azprofile`
  (refuses to clobber). Name defaults to sanitized `$PWD` basename.
- `azrl capture [name]` — record the current session as `<name>.conf` + `.azprofile`
  (no login; refuses to clobber).
- `azrl use <name>` — validated `.azprofile` write linking `$PWD` to an existing
  profile.
- `azrl rm <name> [-y]` — remove `<name>.conf`, `~/.azure-profiles/<name>/`, and
  `$PWD/.azprofile` if it names `<name>`; `[y/N]` confirm unless `-y`. Guards empty
  name, `/`, and the reserved `azrl`.
- `azrl list` — name + tenant per profile, excluding `azrl.conf`.

## TUI (bare `azrl`)

**Home layout:** banner (AZRL block letters, gradient) + angel art at top; a
profile **list** (`bubbles/list`) of `name · tenant`, with a marker on the profile
the current dir's `.azprofile` resolves to; a **context panel** showing the current
directory's status.

**No-profile flow** (the saved bare-azrl ideas):
- `.azprofile` resolves -> "This dir -> `<name>`", offer **Login**.
- no `.azprofile` but `<basename>.conf` exists -> **"Link this dir to `<name>`?"**
  (the `use` action).
- no `.azprofile`, no match -> **"Create profile for this dir"** (init), with an
  **editable name field** (default = sanitized basename). Footgun guard: name is
  always shown/editable so running in `~` or `tmp/` never silently creates junk.

**Keys:** `enter` use-here · `l` login · `i` init · `c` capture · `u` use ·
`d` delete (confirm modal) · `r` refresh · `?` help · `q` quit.

**Long operations** (login/init/capture): a spinner + streamed status lines
(clean slate -> got URL -> bridging -> waiting -> ✓/✗), driven by Bubble Tea
messages. Bridge path-A fallback renders the paste line in a panel. Delete uses a
yes/no confirm modal.

## Visual identity

- Banner: `go-figure` block "AZRL" with an Azure-blue gradient (Lip Gloss), tagline
  "Azure Remote Login".
- Angel: embedded multi-line ASCII (gold halo / white wings), shown beside/above
  the banner on the home screen.
- Palette: Azure blues (primary), gold/amber (accents/halo), green/red
  (success/failure), adaptive to light/dark terminals. Styled panels, borders, help
  bar.

## Testing

- **Unit (internal/profile):** table-driven — Resolve (walk-up), ExtractPort,
  LoadConf/SaveConf roundtrip, SanitizeName, DefaultName, Use, Remove,
  AssertAccount.
- **Integration (internal/azure):** shim `az`/`ssh` by writing temp scripts and
  prepending to `PATH` (same technique as the bats suite) to drive CleanSlate,
  LoginCapture, Bridge, WaitForLogin.
- **TUI (internal/ui):** model-level tests of `Update`/`View` transitions; a
  `teatest` golden check for the banner/home render.

## Install / migration

- `install.sh`: `go build -o ~/.local/bin/azrl .`; keep `.azprofile` global-gitignore
  and `azrl.conf` bootstrap from the example; drop script-symlink logic.
- Remove `azrl`, `azrl-lib.sh`, `azrl-capture`, `tests/*.bats` (in git history).
- Update `README.md` and `CLAUDE.md` for the Go architecture.

## Scope (YAGNI)

In: all current commands + TUI management + full login flow + visuals + tests.
Out (for now): in-place profile editing, multi-account dashboards, telemetry,
self-update, CLI-side interactive auto-init (TUI covers interactive create/link).
