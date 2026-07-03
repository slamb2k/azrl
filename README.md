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
Every release artifact carries OIDC-signed build provenance; verify a download
with `gh attestation verify <file> -R slamb2k/azrl`.

**From source** (contributors):

```bash
./install.sh   # go build + install into ~/.local/bin, gitignore .azprofile,
               # bootstrap ~/.azure-profiles/azrl.conf from the template
```

## First-time setup

**Local workstation or laptop** — nothing to configure. azrl opens your
browser directly; skip straight to the Quick start.

**Headless VM or WSL** (the browser lives on another machine) — tell azrl
where your browser is, once, in `~/.azure-profiles/azrl.conf` (the installers
bootstrap it from [`azrl.conf.example`](azrl.conf.example)):

```ini
LOCAL_HOST=my-laptop        # SSH name of the machine with the browser (e.g. a tailnet name)
LOCAL_BROWSER_CMD=xdg-open  # what opens a URL there — wslview on WSL, open on macOS
VM_HOST=my-vm               # this machine's SSH-reachable name
```

Then `azrl login` does the rest: if `LOCAL_HOST` is SSH-reachable from here,
the sign-in page **pops on your local machine** through a reverse tunnel and
the OAuth callback is forwarded back — zero paste. If it isn't reachable, azrl
prints a one-line `ssh -fNL …` to paste in a local terminal instead. Under
WSL2 with `LOCAL_BROWSER_CMD=wslview`, the page opens straight in your Windows
browser.

[direnv](https://direnv.net) is optional but recommended — it's what makes
accounts switch automatically as you `cd` (azrl writes and allows the `.envrc`
for you).

## Quick start

```bash
# 1. Create a profile by signing in, recorded for this directory
cd ~/work/acme
azrl login acme                # browser opens, you sign in, conf + .azprofile written
                               # (Azure discovers the tenant on first login)
                               # → offers to write .envrc and run `direnv allow`

# 2. In another project, reuse or create another account
cd ~/personal/side-project
azrl login personal

# 3. Now each directory is its own account — no switching needed
cd ~/work/acme      && az account show   # → you@acme.com
cd ~/personal/side-project && az account show   # → you@outlook.com
```

On a **headless VM** the same command is the whole story — after
[First-time setup](#first-time-setup), `azrl login acme` pops the sign-in page
on your laptop and the VM receives the session. The same model covers GitHub
(`ghrl login`), AWS (`azrl aws login`), and Google Cloud (`azrl gcp login`) —
one repo can pin all four at once, and the dashboard shows them together.

Prefer a dashboard? Run **`azrl`** with no arguments — see
[The TUI at a glance](#the-tui-at-a-glance).

## Usage

```bash
azrl                       # launch the TUI (manage / select / sign in to profiles)
azrl login [profile]       # sign in via the browser bridge (uses this dir's profile)
                           # login <name> also creates the profile on first use
                           # (Azure discovers the tenant); pass --yes to skip the prompt
azrl capture [name]        # record the CURRENT az session as conf + .azprofile
azrl use <name>            # link this dir to an existing profile
azrl rm <name> [-y]        # remove a profile (conf + token dir + matching .azprofile)
azrl list                  # list configured profiles and their tenants
azrl status [--json]       # "who am I, everywhere?" — mappings / ambient / unmapped (disk-only)
azrl --help                # usage; azrl --version prints the version
```

`capture` and `login` both **offer to write an `.envrc`** (and run
`direnv allow`) so plain `az` in that directory follows the profile from then on.

Bare `azrl` opens the tabbed TUI (below). `azrl status` prints the same
three-section overview on the CLI; `--json` emits
`{"mappings":[…],"ambient":[…],"unmapped":[…]}`.

## The TUI at a glance

Bare `azrl` lands on a live cross-provider dashboard — "who am I,
everywhere?" — followed by an **Azure · GitHub · AWS · Google Cloud** tab per
provider (every tab shares one profiles/actions layout; `ghrl` opens on the
GitHub tab):

```text
 Dashboard │ Azure │ GitHub
 MAPPINGS
  ● ~/work/acme     → azure:acme       .azprofile
  ● ~/oss/tool      → github:personal  (git)          ⚠ drift
 AMBIENT — defaults in effect
  🌐 GitHub   you@github.com   hosts.yml   managed
 UNMAPPED PROFILES
  ● azure:velrada   you@velrada.com   expired
```

It's **live**: besides polling, it watches each provider's token cache *and*
native config dir via fsnotify, so it updates the moment you sign in — or
`gh auth switch` — with any CLI outside azrl.

**What the marks mean:**

| Mark | Meaning |
|---|---|
| ● green | pinned to the current directory — effective here |
| ● orange | inherited from a parent directory's pin |
| 🌐 | the provider's global default (its ambient identity matches this profile) |
| ● dark-white | mapped to some other directory — real, just not relevant here |
| ● grey | mapped nowhere |
| **bold** name | the profile that would be used right now (closest scope wins) |
| *italic* name | renamed — display label differs from the profile slug |
| `⚠ drift` | your shell's ambient session differs from this directory's pin |
| `managed` / `unmanaged` | the ambient identity is / isn't held by any saved profile |

**Keys:**

| Key | Action |
|---|---|
| `⇥`/`⇧⇥` (or `[`/`]`) | switch tabs · `↑` at the top of a list focuses the tab bar (`←`/`→` to pick, `↓` to return) |
| `→`/`←` | focus the DETAILS pane / back to profiles |
| `↑`/`↓` | select a profile or action |
| `↵` | open the detail pane · run the selected action |
| `esc` | back to the profile list |
| `d` | change directory (fuzzy finder) — the whole app follows |
| `o` | options — choose which provider tabs to show (saved to `azrl.conf`; default Azure + GitHub) |
| `Delete` / `F5` | remove profile / refresh |
| `a` | adopt an unmanaged identity into a new profile (dashboard) |
| `w` | recheck drift (dashboard) |
| `e` | write `.envrc` to pin the shell (Azure tab) |
| `q` / `ctrl+c` | quit |

Sign in and New profile run the full interactive flow — browser bridge
included — directly from the TUI; the tab picks up the result when the flow
finishes.

## GitHub accounts (`gh`)

The same "sign in from a headless box, switch by `cd`" model now covers GitHub.
Each account gets an isolated `GH_CONFIG_DIR` under `~/.github-profiles/<name>/`,
signed in with the browser on your **local** machine.

```bash
azrl gh login [name] [--hostname H]  # sign in (github.com, *.ghe.com, or a GHES host)
azrl gh list                         # list GitHub profiles and their hosts
azrl gh use <name>                   # pin this repo (.ghprofile) + wire git-HTTPS creds
azrl gh capture <name> [--hostname H]# record the currently signed-in gh session
azrl gh status                       # show the ambient and repo-pinned accounts
azrl gh rm <name>                    # remove a GitHub profile and its config dir
```

The **`ghrl`** alias promotes these to the top level (`ghrl login`, `ghrl use`, …)
and opens the TUI on the GitHub tab.

> **`gh switch` was removed in v0.7.0.** The default account is whatever `gh`
> itself is signed into — use `gh auth switch` to change it, or map a directory
> with `ghrl use <name>`. azrl shows the ambient account but never mutates it.

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

## Google Cloud accounts (`gcp`)

The same model extends to **Google Cloud**, built on native `gcloud` **named
configurations**. Each account is a profile under `~/.gcp-profiles/<name>.conf`,
pinned to a repo with a gitignored `.gcpprofile`.

```bash
azrl gcp login [name]   # sign in via `gcloud auth login` over the browser bridge
azrl gcp list           # list GCP profiles and their projects
azrl gcp use <name>     # pin this dir (.gcpprofile) + write an .envrc
azrl gcp capture <name> # record the current gcloud session as a profile
azrl gcp status         # disk-only "who am I?" from the gcloud config dir
azrl gcp rm <name>      # remove a GCP profile
```

`gcloud auth login` reuses the SSH browser bridge unchanged — by default it binds
a `localhost` loopback callback that is forwarded back to your local browser.
`azrl gcp status` is disk-only: it reads the gcloud config dir
(`configurations/config_<name>` for the signed-in account, `active_config` for
drift), no network — token expiry isn't surfaced in v1. On login `azrl` runs
`gcloud auth list --filter=status:ACTIVE` to assert you landed on the expected
account. `gcp use`/`login` idempotently create the named configuration
(`gcloud config configurations create --no-activate`) and set its project/region.
By default the `.envrc` exports `CLOUDSDK_ACTIVE_CONFIG_NAME`; pass `--isolate` to
instead scope the whole `CLOUDSDK_CONFIG` dir to the profile (note:
`gke-gcloud-auth-plugin` ignores `CLOUDSDK_CONFIG`, so `azrl` warns when GKE is
detected under isolation). Real-tenant browser-interception details are still
being validated (see `specs/multi-cloud-providers.manual-verify.md`).

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

The pointer walk-up runs all the way to the filesystem root, so pinning your
**home directory** makes a profile the effective identity everywhere while
deeper pins still win — see
[coexistence patterns](#when-not-to-capture--coexistence-patterns).

## Saving and initializing profiles

- **`azrl login <name>`** — for an unknown `<name>`, signs you in (Azure
  discovers the tenant on first login), then records the live session's tenant
  GUID, subscription, and user to `~/.azure-profiles/<name>.conf` plus a
  `.azprofile` in the current directory. Pass `--yes`/`-y` to create without the
  confirmation prompt. (This replaces the removed `azrl init` command.)
- **`azrl capture [name]`** — same recording step, but for a session you're
  **already** signed into (no new login).
- **`azrl use <name>`** — links the current directory to an **existing** profile
  (validated `echo <name> > .azprofile`); no login, no new conf.
- **`azrl rm <name> [-y]`** — deletes the profile's `<name>.conf`, its token dir,
  and `$PWD/.azprofile` when it names `<name>`. Prompts unless you pass `-y`.

Names default to the sanitized current directory when omitted.

### When not to capture — coexistence patterns

You do **not** have to capture your primary account. The identity your CLIs
use by default is controlled outside azrl; azrl just mirrors it (the AMBIENT
rows), and `unmanaged` there is a legitimate steady state, not a problem.
Capture an identity only when you want what profiles buy: directory pinning,
guardrail assertions, expiry tracking, or bridge re-login. Capture records
metadata only — no tokens are copied, and the profile holds no session until
you `login` it.

When you *do* want your default under azrl's tracking, pin it at home —
`cd ~ && azrl use <name>` (accept the offered `.envrc`) — and every directory
without a more specific pin resolves to it. On AWS/GCP you can go one step
further with zero duplication, because azrl profiles **are** native entries:
`export AWS_PROFILE=<name>` in your shell rc, or
`gcloud config configurations activate <name>` (azrl mirrors but can't track
shell-rc/native state). GitHub is the exception: `gh` has no env-based
selection — its default is `gh auth switch`, and azrl's per-repo wiring via
`ghrl use` covers the rest.

The full pattern language (and what was deliberately *not* built) is in
[docs/ambient-identity-model.md](docs/ambient-identity-model.md).

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
| `~/.gcp-profiles/<profile>.conf` | per-profile GCP: `GCP_CONFIG_NAME` (named gcloud configuration; defaults to the profile name), `GCP_PROJECT` (**required**), `GCP_REGION`, `GCP_EXPECT_ACCOUNT`, `GCP_LABEL`, `GCP_ISOLATE` |
| `<repo>/.gcpprofile` | one line: the GCP profile for that repo (uncommitted; globally gitignored) |
| `~/.gcp-profiles/<profile>/` | isolated `CLOUDSDK_CONFIG` dir (only under `--isolate`) |

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
  manual-verify. **Google Cloud** (`gcloud auth login`) now ships too (see
  [Google Cloud accounts](#google-cloud-accounts-gcp);
  `specs/multi-cloud-providers.md`, Phase 9), built on native `gcloud` named
  configurations and reusing the same bridge (default loopback callback) — its
  real-tenant interception is likewise still in manual-verify.
- **Richer auditability — "who am I, everywhere?"** *(shipped — Phase 5.5,
  `specs/status-dashboard.md`; restructured in v0.7.0,
  `specs/resolution-strategies.md`)*. A cross-provider overview is the default
  landing view of the TUI (and `azrl status [--json]` on the CLI): directory →
  profile mappings with drift, each CLI's ambient identity read from its own
  config on disk, and unmapped profiles with their expiry — refreshed from local
  cache only (no network). A per-directory *history* beyond a single last-used
  timestamp remains a later direction.
- **Unified profiles** *(direction, not yet scoped)*. A single `.azprofile`-style
  pointer that can carry the right identity for *several* providers at once, so
  one `cd` lines up Azure, Git, and your cloud CLI together.

Azure, GitHub, AWS, and GCP are what work today, along with the cross-provider
dashboard. "Unified profiles" remains a direction, not a commitment.

## Requirements

Azure CLI, OpenSSH, `jq`, and (for the remote/WSL browser bridge) a machine that
can open the sign-in page — designed around **Tailscale** MagicDNS + **WSL2**
`wslview`/localhostForwarding, but the local host and browser command are
pluggable. [direnv](https://direnv.net) is optional but recommended for
switch-by-directory.

## Layout & development

```
main.go               # azrl entrypoint
cmd/                  # Cobra tree: azure top-level + `gh`/`aws`/`gcp` groups; hidden __browser shims
cmd/ghrl/             # ghrl alias entrypoint (GitHub subcommands promoted to top level)
internal/config/      # azrl.conf + KEY=value parsing; profile-dir roots
internal/profile/     # pure profile logic + parameterized Scheme (Azure/GitHub/AWS/GCP) — unit-tested
internal/provider/    # Provider interface + shared contract suite (providertest)
internal/azure/       # az/ssh login lifecycle; Azure Provider — shimmed-integration tested
internal/github/      # gh/git login lifecycle; GitHub Provider — shimmed-integration tested
internal/aws/         # aws/sts SSO lifecycle; AWS Provider — shimmed-integration tested
internal/gcp/         # gcloud named-config lifecycle; GCP Provider — shimmed-integration tested
internal/bridge/      # SSH reverse-tunnel / paste-line browser bridge (shared)
internal/browsercapture/ # smart __browser shim: classify + relay/tunnel; xdg-open shadow
internal/ui/          # tabbed Bubble Tea TUI (dashboard | Azure | AWS | GCP | GitHub) — model unit tests
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
`docs/HANDOVER-origin.md` for full context.
