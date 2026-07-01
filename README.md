<p align="center">
  <img src="docs/banner.png" alt="azrl — Azure Remote Login" width="760">
</p>

# azrl — Azure Remote Login

**Juggle multiple Azure accounts without the `az logout` / `az login` treadmill.**

`azrl` gives every Azure identity — work tenant, personal, each client, every
guest/B2B invite — its own isolated, named profile. Switch between them by simply
`cd`-ing into a directory, always know exactly which account you're operating as,
and sign in through a real browser even on machines that don't have one (headless
VMs, WSL) or on tenants whose Conditional Access blocks device-code login.

---

## The problem

The Azure CLI keeps **one** session in `~/.azure`. The moment you work across more
than one account that single session becomes a bottleneck:

- **Switching means re-authenticating.** `az login` clobbers whoever was signed
  in. Bouncing between a work tenant and three client tenants is a day full of
  logout/login round-trips.
- **Guest / B2B accounts are the worst offenders.** Cross-tenant guest invites,
  personal Microsoft accounts, and Entra-ID-only tenants each need their own
  login, and several forbid the device-code flow via Conditional Access.
- **No headless browser.** On a remote SSH box or inside WSL, `az login` can't
  open the sign-in page — the browser lives somewhere else — so it silently
  falls back to device code, which may be blocked.
- **"Wait, which account am I?"** With one shared session it's easy to run a
  destructive command against the wrong subscription. There's no per-directory
  guardrail.

`azrl` fixes all of this by turning each identity into a **profile** with its own
`AZURE_CONFIG_DIR` token cache, wiring those profiles to directories, and bridging
the browser back to wherever you actually are.

## Where it helps

`azrl` is useful anywhere you touch more than one Azure account — it is **not**
just for remote servers:

| Environment | What azrl does for you |
|---|---|
| **Local workstation** | Keep work / personal / per-client accounts side by side, each in its own profile; switch by directory instead of re-logging-in. |
| **WSL (WSL2 on Windows)** | The browser is on Windows, `az` runs in Linux — azrl launches the sign-in page via `wslview` and captures the callback, no device code needed. |
| **Headless / remote VM (SSH)** | Pops the sign-in browser on your **local** machine over a reverse SSH tunnel and forwards the OAuth callback back to the VM. |
| **Conditional-Access tenants** | Keeps the interactive auth-code flow alive so tenants that block device code still work. |
| **Guest / B2B / multi-tenant** | Pins the tenant by GUID (needed where `az account show` returns a null default domain) and verifies you landed as the expected user. |

## What you get

- **Isolated, coexisting sessions.** Each profile stores its tokens under
  `~/.azure-profiles/<name>/`. Account A and Account B are logged in **at the same
  time** — no clobbering, no re-login when you switch.
- **Switch by `cd`.** A one-line, gitignored `.azprofile` names the profile for a
  repo. With [direnv](https://direnv.net), stepping into the directory points
  every `az` command at the right account automatically. `azrl` writes and
  `direnv allow`s the `.envrc` for you.
- **Auditability / guardrails.** After sign-in `azrl` asserts you got the tenant
  and user you expected. The TUI always shows *who this directory is* and warns
  when your shell's ambient `az` has drifted to a different (or no) account, so
  you don't fire a command as the wrong identity.
- **Browser bridging.** No local browser? `azrl` still completes an interactive
  login — reverse SSH tunnel (zero-paste), or a one-line command you paste
  locally, or `wslview` under WSL.
- **Works with subscription-less tenants.** Signs in with
  `--allow-no-subscription`, so Entra-ID-only / tenant-level accounts are fine.

## Quick start

```bash
# 1. Create a profile by signing in (tenant-less), recorded for this directory
cd ~/work/acme
azrl init acme                 # browser opens, you sign in, conf + .azprofile written
                               # → offers to write .envrc and run `direnv allow`

# 2. In another project, reuse or create another account
cd ~/personal/side-project
azrl init personal

# 3. Now each directory is its own account — no switching needed
cd ~/work/acme      && az account show   # → you@acme.com
cd ~/personal/side-project && az account show   # → you@outlook.com
```

Prefer a dashboard? Run **`azrl`** with no arguments for the TUI: pick a profile,
sign in, capture, link, or remove — and press `e` to pin the current directory to
its profile when it warns about drift.

## Usage

```bash
azrl                       # launch the TUI (manage / select / sign in to profiles)
azrl login [profile]       # sign in via the browser bridge (uses this dir's profile)
azrl init [name]           # tenant-less login, then record conf + .azprofile
azrl capture [name]        # record the CURRENT az session as conf + .azprofile
azrl use <name>            # link this dir to an existing profile
azrl rm <name> [-y]        # remove a profile (conf + token dir + matching .azprofile)
azrl list                  # list configured profiles and their tenants
azrl --help                # usage; azrl --version prints the version
```

`init`, `capture`, and `login` all **offer to write an `.envrc`** (and run
`direnv allow`) so plain `az` in that directory follows the profile from then on.

## Switching accounts by directory

`azrl` runs as a subprocess, so its per-profile isolation only covers the login
itself. To make **every** `az` in a tree use the right account, pin
`AZURE_CONFIG_DIR` per directory. `azrl` sets this up for you, or do it by hand
with [direnv](https://direnv.net):

```bash
# <repo>/.envrc   (gitignored alongside .azprofile)
export AZURE_CONFIG_DIR="$HOME/.azure-profiles/$(cat .azprofile)"
```

```bash
direnv allow
```

Add `.envrc` and `.azprofile` to your global gitignore (`~/.config/git/ignore`).
Now `cd`-ing between projects silently switches Azure accounts — `az account show`,
`az group list`, everything follows the directory. Without direnv, export the same
line in your shell rc.

## Saving and initializing profiles

- **`azrl init [name]`** — signs you in (tenant-less), then records the live
  session's tenant GUID, subscription, and user to `~/.azure-profiles/<name>.conf`
  plus a `.azprofile` in the current directory.
- **`azrl capture [name]`** — same recording step, but for a session you're
  **already** signed into (no new login).
- **`azrl use <name>`** — links the current directory to an **existing** profile
  (validated `echo <name> > .azprofile`); no login, no new conf.
- **`azrl rm <name> [-y]`** — deletes the profile's `<name>.conf`, its token dir,
  and `$PWD/.azprofile` when it names `<name>`. Prompts unless you pass `-y`.

Names default to the sanitized current directory when omitted.

## Install

```bash
./install.sh               # go build + install azrl into ~/.local/bin,
                           # ensure .azprofile is globally gitignored,
                           # bootstrap ~/.azure-profiles/azrl.conf from the template
```

## Configuration

| File | Purpose |
|---|---|
| `~/.azure-profiles/azrl.conf` | global: `LOCAL_HOST` (host running the browser, e.g. a tailnet name), `LOCAL_BROWSER_CMD` (e.g. `wslview`), `VM_HOST` (this machine's reachable name) |
| `~/.azure-profiles/<profile>.conf` | per-profile: `AZ_TENANT` (domain, for `az login --tenant`), `AZ_TENANT_ID` (tenant GUID — **required for guest/B2B** where `az account show` returns a null `tenantDefaultDomain`), `AZ_DEFAULT_SUB`, `AZ_EXPECT_USER` |
| `<repo>/.azprofile` | one line: the profile name for that repo (uncommitted; globally gitignored) |
| `<repo>/.envrc` | direnv stanza pinning `AZURE_CONFIG_DIR` to the profile (uncommitted; globally gitignored) |
| `~/.azure-profiles/<profile>/` | isolated per-profile token cache (`AZURE_CONFIG_DIR`) |

See `azrl.conf.example` and `profile.conf.example` for templates.

## Roadmap

The pattern at azrl's core — **named, isolated, directory-scoped credential
profiles + an interactive browser login that works from anywhere + automatic
per-directory switching** — isn't specific to Azure. Planned directions:

- **More login providers.** Bring the same "sign in from a headless box, switch by
  `cd`" experience to other tools that need an interactive browser login —
  **GitHub** (`gh auth login`), **AWS** (IAM Identity Center / `aws sso login`),
  and **Google Cloud** (`gcloud auth login`) are the leading candidates.
- **Unified profiles.** A single `.azprofile`-style pointer that can carry the
  right identity for *several* providers at once, so one `cd` lines up Azure, Git,
  and your cloud CLI together.
- **Richer auditability.** A quick "who am I, everywhere?" view and history of
  which account was active in which directory.

These are directions, not commitments — the Azure experience above is what ships
today.

## Requirements

Azure CLI, OpenSSH, `jq`, and (for the remote/WSL browser bridge) a machine that
can open the sign-in page — designed around **Tailscale** MagicDNS + **WSL2**
`wslview`/localhostForwarding, but the local host and browser command are
pluggable. [direnv](https://direnv.net) is optional but recommended for
switch-by-directory.

## Layout & development

```
main.go            # entrypoint
cmd/               # Cobra subcommands (+ hidden __browser-capture self-shim)
internal/config/   # azrl.conf + KEY=value parsing
internal/profile/  # pure profile logic (resolve, conf I/O, use, rm, .envrc) — unit-tested
internal/azure/    # az/ssh login lifecycle — unit + shimmed-integration tested
internal/ui/       # Bubble Tea TUI (winged banner, profile list, action pane)
install.sh         # go build + install + config bootstrap
```

```bash
go build ./...
go test ./...
gofmt -l .
```

Pure logic lives in `internal/profile` and is unit-tested. `internal/azure` shells
out to `az`/`ssh` and is tested by shimming them onto `PATH`. The single binary is
its own `$BROWSER` shim via the hidden `__browser-capture` subcommand. See
`HANDOVER.md` for full context.
