# azrl `--init` / `--save` + tenant-less login — Design

Date: 2026-06-30

## Goal

Make it possible to walk into a fresh directory, run one command, sign in
(MFA captured via the existing browser shim), and have the tenant details
recorded under `~/.azure-profiles/` with the directory wired up for next
time — without seeding a `<profile>.conf` by hand first.

## Scope

1. Rename `--derive` → `--save` across the codebase.
2. Tenant-less login plumbing.
3. New `azrl --save | -s [name]`.
4. New `azrl --init | -i [name]`.
5. Bare `azrl` tenant-less fallback when no profile resolves.

## 1. Rename `--derive` → `--save`

- `azrl`: flag/mode `--derive`/`derive` → `--save`/`-s`/`save`.
- `azrl-lib.sh`: pure fn `azrl_derive_conf` → `azrl_save_conf`; usage text.
- `tests/azrl.bats`: test names + invocations.
- `README.md`: updated. `HANDOVER.md` left as historical record.

## 2. Tenant-less login plumbing

`azrl_login_capture` currently always passes `--tenant "$1"` to `az login`.
Change it so an **empty** tenant arg omits `--tenant` entirely. Single
testable function; unit test asserts the shimmed `az` is invoked *without*
`--tenant` when tenant is empty, and *with* it otherwise.

## 3. `azrl --save | -s [name]`

Records the current session (today's `--derive` + writing `.azprofile`).

- **name** = arg verbatim, else `azrl_sanitize_name "$(basename "$PWD")"`.
  No `.azprofile` walk-up — always names after the current dir (or arg).
- `AZURE_CONFIG_DIR=~/.azure-profiles/<name>`. If no session there →
  fail: `not logged in for <name> — run azrl --init first` (exit 1).
- Writes `~/.azure-profiles/<name>.conf` via `azrl_save_conf`.
  **Refuses to clobber** an existing conf (re-save = `rm` first).
- Writes `<name>` into `$PWD/.azprofile`.
- Does **not** require global `azrl.conf` (no login/bridge needed).

## 4. `azrl --init | -i [name]`

Zero-config bootstrap in a fresh directory.

- **name** = arg verbatim, else `azrl_sanitize_name "$(basename "$PWD")"`.
- Requires global `azrl.conf` (needs the bridge).
- `AZURE_CONFIG_DIR=~/.azure-profiles/<name>`; `azrl_clean_slate` →
  `azrl_login_capture ""` (tenant-less) → `azrl_bridge` →
  `azrl_wait_for_login`.
- On success: write conf (refuse-clobber) + `.azprofile`, same as `--save`.

## 5. Bare `azrl` fallback

When no profile resolves (no arg, no `.azprofile`): instead of erroring,
run a tenant-less login into the real default `~/.azure` (no
`AZURE_CONFIG_DIR` override, no conf/`.azprofile` written) — just
authenticate. Still requires global `azrl.conf` for the bridge.

## New helper: `azrl_sanitize_name`

Pure, testable. lowercase → replace each run of chars outside `[a-z0-9._-]`
with a single `-` → strip leading/trailing `-`.
e.g. `Contoso Migration` → `contoso-migration`. Explicit args are trusted
as-typed (not sanitized).

## Testing

TDD per existing patterns (PATH-shimmed `az`/`ssh`):

- `azrl_sanitize_name`: spaces, mixed case, leading/trailing junk.
- `azrl_login_capture`: omits `--tenant` when tenant empty; includes it
  otherwise.
- `azrl --save`: name from arg vs sanitized `basename $PWD`; not-logged-in
  failure; writes conf + `.azprofile`; refuses clobber.
- `azrl --init`: end-to-end with shims; writes conf + `.azprofile`.
- bare `azrl` fallback: tenant-less login, no files written.
- renamed `azrl_save_conf` tests (carried over from `azrl_derive_conf`).

## Decisions

- **Clobber:** both `--init` and `--save` refuse to overwrite an existing
  `<name>.conf` (preserves `--derive` behavior).
- **Name source for `--save`:** always `basename $PWD` (sanitized) or arg;
  no `.azprofile` walk-up.
