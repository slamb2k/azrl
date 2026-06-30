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
azrl                       # launch the TUI (manage/select/login profiles)
azrl login [profile]       # sign in via the remote-browser bridge
azrl init [name]           # tenant-less login, then record conf + .azprofile
azrl capture [name]        # record the current session as conf + .azprofile
azrl use <name>            # link this dir to an existing profile
azrl rm <name> [-y]        # remove a profile (conf + token dir + matching .azprofile)
azrl list                  # list configured profiles and their tenants
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
azrl capture [name]        # writes ~/.azure-profiles/<name>.conf
```

`capture` reads the live session's tenant GUID, subscription, and user, and
writes them to `<name>.conf` plus a `.azprofile` in the current directory.
`init` does the same but signs you in first (tenant-less). The name
defaults to the sanitized current directory when omitted.

`use <name>` links the current directory to an **existing** profile by writing
its name to `.azprofile` (after checking `<name>.conf` exists). Use it to point a
new repo at a profile you already created — unlike `capture`/`init`, it does not
log in or create a conf. Equivalent to `echo <name> > .azprofile`, but validated.

`rm <name>` deletes the profile's `<name>.conf`, its token dir
`~/.azure-profiles/<name>/`, and `$PWD/.azprofile` when it names `<name>`. It
prompts for confirmation unless you pass `-y`.

## Install

```bash
./install.sh               # go build + installs azrl into ~/.local/bin,
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
main.go            # entrypoint
cmd/               # Cobra subcommands (+ hidden __browser-capture self-shim)
internal/config/   # azrl.conf + KEY=value parsing
internal/profile/  # pure profile logic (resolve, conf I/O, use, rm) — unit-tested
internal/azure/    # az/ssh login lifecycle — unit + shimmed-integration tested
internal/ui/       # Bubble Tea TUI (banner, angel, list, actions)
install.sh         # go build + install + config bootstrap
```

## Development

```bash
go build ./...
go test ./...
gofmt -l .
```

Pure logic lives in `internal/profile` and is unit-tested. `internal/azure` shells
out to `az`/`ssh` and is tested by shimming them onto `PATH`. Bare `azrl` launches
the `internal/ui` TUI. The single binary is its own `$BROWSER` shim via the hidden
`__browser-capture` subcommand. See `HANDOVER.md` for full context.

## Requirements

Azure CLI, OpenSSH, `jq`, and a local machine reachable from the VM (designed
around **Tailscale** MagicDNS + **WSL2** `wslview`/localhostForwarding, but the
config makes the local host/browser command pluggable).
