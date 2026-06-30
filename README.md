# azrl — Azure Remote Login

Interactive `az login` from a **headless/remote Linux VM**, with the sign-in
browser opening on your **local machine** and the OAuth callback forwarded back —
even when Conditional Access **blocks device-code flow**. Picks the right Azure
profile per repo, always starts from a clean slate, and verifies you ended up as
the identity you expected.

## Why this exists

`az login`'s interactive flow binds a **random** `127.0.0.1:<port>` listener on
the VM (no flag pins it). On a headless box az silently **falls back to device
code** — which some tenants' CA policies forbid. And the browser lives on your
local machine, which has no route to the VM's loopback. `azrl` solves all three:

1. **Keeps the auth-code flow alive** — points `$BROWSER` at a capture helper so
   MSAL believes a browser launched (no device-code fallback) and records the
   random callback URL/port.
2. **Bridges the browser** — opens a reverse SSH tunnel to your local machine and
   launches the browser there (path **B**, zero-paste); falls back to printing a
   one-line `ssh -L …` you paste locally (path **A**).
3. **Right profile, clean slate, verified identity** — per-repo profile via an
   uncommitted `.azprofile`, isolated `AZURE_CONFIG_DIR`, a hard logout/clear
   before login, and a post-login assertion of tenant (by GUID, for guest/B2B)
   and user.

## Usage

```bash
cd <repo> && azrl          # auto-detect profile from .azprofile, pop local browser
azrl <profile>             # override the profile explicitly
azrl --paste               # force the manual paste-line path (A)
azrl --list                # list configured profiles and their tenants
azrl --init [name]         # tenant-less login, then record conf + .azprofile
azrl --save [name]         # record current session as conf + .azprofile
azrl --help                # usage; azrl --version prints the version
```

azrl always signs in with `--allow-no-subscription`, so it works with tenants
that have no Azure subscription (Entra-ID-only / tenant-level accounts).

## Using the profile after login

`azrl` runs as a subprocess, so its `AZURE_CONFIG_DIR` isolation only covers the
login itself — once it exits, plain `az` in your shell falls back to `~/.azure`
unless you export `AZURE_CONFIG_DIR` yourself. Pin it **per repo** so every `az`
in that tree uses the right profile. With [direnv](https://direnv.net):

```bash
# <repo>/.envrc  (gitignored alongside .azprofile)
export AZURE_CONFIG_DIR="$HOME/.azure-profiles/$(cat .azprofile)"
```

Add `.envrc` to your global gitignore (`~/.config/git/ignore`) next to
`.azprofile`, then `direnv allow`. Without direnv, export the same line in your
shell rc or before running `az`.

## Saving and initializing profile configs

```bash
azrl --save <name>         # writes ~/.azure-profiles/<name>.conf
```

`--save` reads the live session's tenant GUID, subscription, and user, and
writes them to `<name>.conf` plus a `.azprofile` in the current directory.
`--init` does the same but signs you in first (tenant-less). The name
defaults to the sanitized current directory when omitted.

## Install

```bash
./install.sh               # symlinks azrl + azrl-capture into ~/.local/bin,
                           # ensures .azprofile is globally gitignored,
                           # bootstraps ~/.azure-profiles/azrl.conf from the template
```

## Configuration

| File | Purpose |
|---|---|
| `~/.azure-profiles/azrl.conf` | global: `LOCAL_HOST` (tailnet host running the browser), `LOCAL_BROWSER_CMD` (e.g. `wslview`), `VM_HOST` (this VM's tailnet name) |
| `~/.azure-profiles/<profile>.conf` | per-profile: `AZ_TENANT` (domain, for `az login --tenant`), `AZ_TENANT_ID` (tenant GUID — **required for guest/B2B** where `az account show` returns a null `tenantDefaultDomain`), `AZ_DEFAULT_SUB`, `AZ_EXPECT_USER` |
| `<repo>/.azprofile` | one line: the profile name for that repo (uncommitted; globally gitignored) |
| `~/.azure-profiles/<profile>/` | isolated per-profile token cache (`AZURE_CONFIG_DIR`) |

See `azrl.conf.example` and `profile.conf.example` for templates.

## Layout

```
azrl            # orchestrator (symlinked onto PATH)
azrl-lib.sh     # pure, sourceable functions (unit-tested)
azrl-capture    # $BROWSER capture helper
install.sh      # symlink + gitignore + config bootstrap
tests/azrl.bats # bats unit tests
docs/           # design.md + build-plan.md (historical, pre-rename)
```

## Development

```bash
bats tests/azrl.bats
shellcheck azrl azrl-lib.sh azrl-capture
```

Pure logic lives in `azrl-lib.sh` (sourceable, fully unit-tested). Integration
paths (`az login` lifecycle, the reverse-tunnel bridge, the watchdog/timeout) are
exercised end-to-end. Built TDD-first. See `HANDOVER.md` for full context.

## Requirements

Azure CLI, OpenSSH, `jq`, and a local machine reachable from the VM (designed
around **Tailscale** MagicDNS + **WSL2** `wslview`/localhostForwarding, but the
config makes the local host/browser command pluggable).
