<p align="center">
  <img src="docs/banner.png" alt="azrl — Azure Remote Login" width="760">
</p>

# azrl — No device codes. Just divine intervention.

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
azrl status [--json]       # "who am I, everywhere?" — cross-provider table (disk-only)
azrl --help                # usage; azrl --version prints the version
```

`init`, `capture`, and `login` all **offer to write an `.envrc`** (and run
`direnv allow`) so plain `az` in that directory follows the profile from then on.

Bare `azrl` opens a **tabbed TUI** — landing on a cross-provider **status
dashboard** ("who am I, everywhere?"), plus **Azure**, **GitHub**, and **AWS**
tabs; switch between them with `[` and `]`.

## GitHub accounts (`gh`)

The same "sign in from a headless box, switch by `cd`" model now covers GitHub.
Each account gets an isolated `GH_CONFIG_DIR` under `~/.github-profiles/<name>/`,
signed in with the browser on your **local** machine.

```bash
azrl gh login [name] [--hostname H]  # sign in (github.com, *.ghe.com, or a GHES host)
azrl gh list                         # list GitHub profiles and their hosts
azrl gh use <name>                   # pin this repo (.ghprofile) + wire git-HTTPS creds
azrl gh switch <name>                # set the active account (default when a repo has no pin)
azrl gh capture <name> [--hostname H]# record the currently signed-in gh session
azrl gh status                       # show the active and repo-pinned accounts
azrl gh rm <name>                    # remove a GitHub profile and its config dir
```

The **`ghrl`** alias promotes these to the top level (`ghrl login`, `ghrl use`, …)
and opens the TUI on the GitHub tab.

How the browser reaches your laptop:

- **`gh` sign-in** uses GitHub's device flow — no localhost callback and no
  Conditional-Access kill switch. `azrl` sets `$BROWSER` to its shim, which
  **relays** the activation page to your local browser; `gh` polls for the token.
  Sign-in is forced into the per-profile `hosts.yml` with `--insecure-storage`
  (the OS keyring is global and would otherwise collide across accounts).
- **git-HTTPS via Git Credential Manager** *does* use a random `127.0.0.1:PORT`
  callback. GCM ignores `$BROWSER` on Linux and execs `xdg-open`, so `azrl`
  shadows `xdg-open` on `PATH` with a wrapper that forwards to the same shim; the
  shim parses the callback port and opens a reverse SSH tunnel (or prints a paste
  line). `gh use` also sets the repo-local
  `credential.https://<host>.username` so two accounts on one host never
  cross-push.
- **VS Code** needs no bridge — Remote-SSH already handles GitHub sign-in through
  its own URI handler.

## AWS accounts (`aws`)

The same model extends to **AWS IAM Identity Center (SSO)**. Each account is a
named profile under `~/.aws-profiles/<name>.conf`, pinned to a repo with a
gitignored `.awsprofile`.

```bash
azrl aws login [name]   # sign in via `aws sso login` over the browser bridge
azrl aws list           # list AWS SSO profiles and their start URLs
azrl aws use <name>     # pin this dir (.awsprofile) + write an .envrc
azrl aws capture <name> # record the current SSO session as a profile
azrl aws status         # disk-only "who am I?" from the SSO token cache
azrl aws rm <name>      # remove an AWS profile
```

`aws sso login` reuses the SSH browser bridge unchanged — the PKCE loopback
`127.0.0.1` callback is forwarded back to your local browser (with
`--use-device-code` as a fallback). `azrl aws status` is disk-only: it reads
`~/.aws/sso/cache/*.json` for the signed-in account/role and expiry, no network.
On login `azrl` runs `aws sts get-caller-identity` to assert you landed on the
expected account and role boundary. Pass `--isolate` to scope
`AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE` to the profile; otherwise the
`.envrc` just exports `AWS_PROFILE`. Real-tenant browser-interception details are
still being validated (see `specs/multi-cloud-providers.manual-verify.md`).

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

**Quick install** (Linux/macOS — pulls the latest release binary):

```bash
curl -fsSL https://raw.githubusercontent.com/slamb2k/azrl/main/scripts/install.sh | sh
```

**Homebrew** (macOS/Linux):

```bash
brew install slamb2k/tap/azrl
```

**Go** (any platform with a recent Go toolchain):

```bash
go install github.com/slamb2k/azrl@latest
```

**Binaries & packages** — download a `.tar.gz`, `.deb`, or `.rpm` for your
platform from the [latest release](https://github.com/slamb2k/azrl/releases/latest).

**From source** (contributors):

```bash
./install.sh   # go build + install into ~/.local/bin, gitignore .azprofile,
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
| `~/.github-profiles/<profile>.conf` | per-profile GitHub: `GH_HOST` (github.com / `*.ghe.com` / GHES host), `GH_USER` (expected login), `GH_LABEL` (optional display name), `GH_PROTOCOL` (`https`) |
| `<repo>/.ghprofile` | one line: the GitHub profile for that repo (uncommitted; globally gitignored) |
| `~/.github-profiles/<profile>/` | isolated per-profile `GH_CONFIG_DIR` (its own `hosts.yml`/token) |
| `~/.aws-profiles/<profile>.conf` | per-profile AWS SSO: `AWS_SSO_START_URL`, `AWS_SSO_REGION`, `AWS_ACCOUNT_ID`, `AWS_ROLE_NAME`, `AWS_EXPECT_ACCOUNT`, `AWS_EXPECT_ARN`, `AWS_LABEL`, `AWS_ISOLATE` |
| `<repo>/.awsprofile` | one line: the AWS profile for that repo (uncommitted; globally gitignored) |
| `~/.aws-profiles/<profile>/` | isolated `AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE` (only under `--isolate`) |

See `azrl.conf.example` and `profile.conf.example` for templates.

## Roadmap

The pattern at azrl's core — **named, isolated, directory-scoped credential
profiles + an interactive browser login that works from anywhere + automatic
per-directory switching** — isn't specific to Azure. It's now a
**provider-aware binary**, and the next providers + dashboard are scoped,
numbered phases behind the shared `Provider` interface (see `specs/`):

- **More login providers**. **GitHub** ships (see
  [GitHub accounts](#github-accounts-gh); `specs/github-remote-login.md`,
  Phases 1–7). **AWS** (IAM Identity Center / `aws sso login`) now ships too (see
  [AWS accounts](#aws-accounts-aws); `specs/multi-cloud-providers.md`, Phase 8),
  reusing the SSH browser-bridge unchanged — real-tenant interception is still in
  manual-verify. **Google Cloud** (`gcloud auth login`) is Phase 9 *(scoped)*,
  where the bridge replaces `gcloud --no-browser` outright.
- **Richer auditability — "who am I, everywhere?"** *(shipped — Phase 5.5,
  `specs/status-dashboard.md`)*. A cross-provider status dashboard is now the
  default landing view of the TUI (and `azrl status [--json]` on the CLI): every
  profile's identity, bound directory, expiry, and ambient-drift in one
  glanceable table, refreshed from local cache only (no network). A
  per-directory *history* beyond a single last-used timestamp remains a later
  direction.
- **Unified profiles** *(direction, not yet scoped)*. A single `.azprofile`-style
  pointer that can carry the right identity for *several* providers at once, so
  one `cd` lines up Azure, Git, and your cloud CLI together.

Azure, GitHub, and AWS are what work today, along with the cross-provider
dashboard; GCP is the next committed, numbered phase (see `specs/`). "Unified
profiles" remains a direction, not a commitment.

## Requirements

Azure CLI, OpenSSH, `jq`, and (for the remote/WSL browser bridge) a machine that
can open the sign-in page — designed around **Tailscale** MagicDNS + **WSL2**
`wslview`/localhostForwarding, but the local host and browser command are
pluggable. [direnv](https://direnv.net) is optional but recommended for
switch-by-directory.

## Layout & development

```
main.go               # azrl entrypoint
cmd/                  # Cobra tree: azure top-level + `gh`/`aws` groups; hidden __browser shims
cmd/ghrl/             # ghrl alias entrypoint (GitHub subcommands promoted to top level)
internal/config/      # azrl.conf + KEY=value parsing; profile-dir roots
internal/profile/     # pure profile logic + parameterized Scheme (Azure/GitHub/AWS) — unit-tested
internal/provider/    # Provider interface + shared contract suite (providertest)
internal/azure/       # az/ssh login lifecycle; Azure Provider — shimmed-integration tested
internal/github/      # gh/git login lifecycle; GitHub Provider — shimmed-integration tested
internal/aws/         # aws/sts SSO lifecycle; AWS Provider — shimmed-integration tested
internal/bridge/      # SSH reverse-tunnel / paste-line browser bridge (shared)
internal/browsercapture/ # smart __browser shim: classify + relay/tunnel; xdg-open shadow
internal/ui/          # tabbed Bubble Tea TUI (dashboard | Azure | GitHub | AWS) — model unit tests
install.sh            # go build + install + config bootstrap
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
