# GitHub Remote Login — Manual Verification

- **Date:** 2026-07-01
- **Purpose:** Items implemented and unit-tested with fakes here, but whose
  end-to-end behaviour can only be *closed out* on a real laptop + remote VM +
  Git Credential Manager (GCM) + VS Code. Nothing below is claimed "done" — each
  is a scripted repro for the user to run in the real environment.
- **Related:** `specs/github-remote-login.md`, `specs/github-remote-login.spike.md`.

## What IS verified in this headless environment

- Unit + shimmed-integration tests green across all packages (`go test ./...`),
  `gofmt -l .` clean, `go vet` clean, at every committed phase.
- Both `internal/azure` and `internal/github` pass the shared
  `providertest.RunContract` suite.
- `internal/github` login/use/assert/capture logic driven against **fake**
  `gh`/`git` shims on `PATH`: asserts `gh auth login --insecure-storage
  --hostname <host> --git-protocol https --web` with `GH_CONFIG_DIR` scoped
  per-profile and `BROWSER` set to the shim; `gh auth setup-git` +
  `git config --local credential.https://<host>.username <user>`; `gh api user`
  for assert/capture.
- Smart shim: URL classification (device vs loopback), `redirect_uri` port
  parsing (127.0.0.1/localhost, %3A/%2F-encoded), `xdg-open` wrapper forwarding,
  and all four `Run` paths (device A/B, loopback A/B) via a fake `ssh`.
- Tabbed TUI renders and switches tabs — proven by a **tmux capture**: the Azure
  banner + panes on the default tab, `]` switches to the GitHub tab listing
  profiles (`internal/ui` container tests assert the same via `View()`).

## Items requiring the real laptop + VM + GCM + VS Code

### 1. End-to-end `gh` device-flow sign-in with browser relay

**Why manual:** needs a real GitHub account + an interactive `gh` on a TTY +
a reachable local browser.

**Repro (on the VM):**
```bash
# global config present at ~/.azure-profiles/azrl.conf (LOCAL_HOST/VM_HOST/LOCAL_BROWSER_CMD)
ghrl login work --hostname github.com      # or: azrl gh login work
# Expect: the gh device activation page opens on your LOCAL machine (via the
# shim relay over SSH); enter the one-time code; gh completes on the VM.
ghrl status
cat ~/.github-profiles/work/hosts.yml       # token stored here (insecure-storage), NOT the keyring
```
**Pass:** `gh api user` under `GH_CONFIG_DIR=~/.github-profiles/work` returns the
expected login; no browser ran on the VM.

### 2. End-to-end git-HTTPS push through the shadow-`xdg-open` shim + SSH tunnel

**Why manual:** GCM is **not installed** in this environment; needs a real GCM
loopback callback + a laptop browser.

**Repro:**
```bash
# Install GCM and ensure the azrl xdg-open shim is ahead of /usr/bin/xdg-open on PATH.
# (Generate it from browsercapture.XdgOpenShimScript(<azrl-bin>) into a dir on PATH.)
cd <repo pinned with `ghrl use work`>
git push        # GCM opens 127.0.0.1:PORT authorize URL via xdg-open -> shim
# Expect: shim parses PORT, opens a reverse SSH tunnel, laptop browser authorizes,
# callback tunnels back to the VM; push succeeds.
```
**Pass:** push authenticates as `work`'s account with no manual port juggling.

### 3. Two same-host accounts never cross-push

**Why manual:** needs two real github.com accounts + GCM.

**Repro:**
```bash
ghrl login alice --hostname github.com
ghrl login bob   --hostname github.com
cd repoA && ghrl use alice    # sets credential.https://github.com.username alice
cd repoB && ghrl use bob
cd repoA && git push          # must authenticate as alice
cd repoB && git push          # must authenticate as bob
```
**Pass:** each repo pushes as its pinned account; neither uses the other's token.

### 4. `gh` isolation on a host with a working keyring

**Why manual:** this VM had no functional unlocked keyring, so plaintext fallback
was automatic. Confirm `--insecure-storage` still isolates when a keyring exists.

**Repro:** on a desktop with an unlocked Secret Service keyring, run `ghrl login`
for two profiles at the same host+user and confirm each token lands in its own
`~/.github-profiles/<name>/hosts.yml` (not a shared keyring entry).

### 5. VS Code Remote-SSH GitHub sign-in needs zero tunnel

**Why manual:** the full VS Code app is not on this VM (only `.vscode-server`).

**Repro:** open the VM folder via VS Code Remote-SSH on your laptop; sign in to
GitHub from the Accounts menu. **Pass:** sign-in completes with **no** `azrl`
bridge/tunnel involved (VS Code's `vscode://` URI handler + `asExternalUri` do
it). This is documented as handled, not bridged.

## GHES-on-private-network caveat (documented, not engineered)

For a self-hosted GHES reachable only on the VM's private network, the laptop
browser must also reach the GHES **authorize** page — put the laptop on the same
VPN. github.com and `*.ghe.com` are public, so this only bites self-hosted GHES.
