# azrl — Session Handover / Init Prompt

> Paste-ready context for a fresh Claude Code session picking up `azrl`
> development. Captures the full origin story, every design decision, what's
> validated, and what's left. Current as of **2026-06-16**.

---

## 1. What this is

`azrl` (Azure Remote Login) is a Bash CLI that makes interactive `az login`
work cleanly from a **headless remote Linux VM** when the browser is on a
**local machine** and the tenant's Conditional Access **blocks device-code flow**.
It also fixes "logged into the wrong profile/user" by isolating per-repo profiles
and asserting identity after login.

It began as a personal fix for the FIIG Document Reviewer project's auth pain and
was extracted into this standalone repo for ongoing development.

## 2. The core problem (and why the obvious fixes don't work)

- `az login`'s interactive (MSAL auth-code) flow binds a **random ephemeral
  `127.0.0.1:<port>`** listener on the VM. **No CLI flag pins the port**
  (confirmed on az 2.86.0; azure-cli issue #26556 — never implemented).
- On a headless box, az **auto-falls back to device-code**, which the FIIG CA
  policy **rejects**. So we must prevent that fallback.
- The browser is on the local machine, which can't reach the VM's loopback.
- `AZURE_CONFIG_DIR` gives **fully isolated** login state per directory (its own
  `azureProfile.json` + `msal_token_cache.json`) — this is the multi-profile
  mechanism.

## 3. How azrl solves it (architecture)

One orchestrator (`azrl`) + a pure-function library (`azrl-lib.sh`, fully
unit-tested) + a `$BROWSER` capture helper (`azrl-capture`).

Flow of `azrl [profile] [--paste]`:
1. **Resolve profile** — explicit arg, else `.azprofile` walking up to the git
   root, else error. (`azrl_resolve_profile`)
2. **Isolate** — `export AZURE_CONFIG_DIR=~/.azure-profiles/<profile>`; source
   `~/.azure-profiles/<profile>.conf`. (`azrl_load_profile_conf`)
3. **Clean slate** (scoped to that profile only) — `az logout` / `az account
   clear` / rm the token-cache files. Never touches other profiles.
   (`azrl_clean_slate`)
4. **Capture trick** — run `az login --tenant $AZ_TENANT` with
   `BROWSER="azrl-capture %s"`. The helper records the auth URL and exits 0, so
   MSAL thinks a browser launched (no device-code fallback) and keeps its
   listener alive. We parse the random port from the captured
   `redirect_uri=...localhost:<port>`. (`azrl_login_capture` + `azrl_extract_port`)
5. **Bridge the browser:**
   - **Path B (default, zero-paste):** if the local host is reachable, open a
     reverse tunnel `ssh -N -R <port>:localhost:<port> $LOCAL_HOST`, verify the
     tunnel process is alive, then `ssh $LOCAL_HOST "$LOCAL_BROWSER_CMD '<url>'"`
     to pop the local browser. The callback returns: local browser →
     `localhost:<port>` → (WSL2 localhostForwarding) → reverse tunnel → VM
     listener. (`azrl_bridge`)
   - **Path A (fallback):** if local unreachable, `--paste`, or the tunnel dies,
     print a one-line `ssh -fNL <port>:localhost:<port> <vm> && <browser> "<url>"`
     to paste locally. (`azrl_paste_line`)
6. **Wait with a watchdog** — `wait` on the background `az login`, bounded by
   `AZRL_LOGIN_TIMEOUT` (default 180s). On failure/cancel/timeout, print the
   path-A recovery hint and exit nonzero.
7. **Verify identity** — `az account set` to `$AZ_DEFAULT_SUB` if set (hard-fail
   on error — no silent wrong-sub), then assert tenant + user match the profile;
   loud red mismatch + nonzero exit, else green ✓. (`azrl_assert_account`)
8. **Cleanup trap** tears down the reverse tunnel, watchdog, background az, and
   the capfile on every exit path.

## 4. Key design decisions (and the reasoning)

- **VM drives, not local.** `az` must run on the VM (that's where repos/scripts/
  token cache live). The repo→profile context lives on the VM, so a local-only
  helper would have to interrogate the VM's tmux for the cwd — backwards. So the
  command runs on the VM and reaches *out* to the local browser. (We explicitly
  dropped an earlier "local helper" idea for this reason.)
- **Ephemeral tunnel, not SSH ControlMaster.** Windows' native OpenSSH lacks
  `ControlMaster`/`-O forward`; an ephemeral `ssh -R`/`-L` is portable across
  WSL bash and Windows pwsh and needs no special SSH config.
- **`.azprofile` is uncommitted** — added to the global gitignore
  (`~/.config/git/ignore`) so it never pollutes any repo.
- **Assert tenant by GUID for guests.** See §6 — the FIIG login is a guest/B2B
  account, so the domain can't anchor the check.
- **Repo lives outside any project**, originally `~/.azure-profiles/azlogin/`,
  now `~/work/azrl/`. Real `*.conf` are gitignored; only `*.conf.example` ship.
- **TDD throughout.** Pure functions in `azrl-lib.sh` are bats-tested; risky
  integration assumptions were validated with live spikes before building on them.

## 5. What's been validated LIVE (not just assumed)

- ✅ **BROWSER-capture works** on az 2.86.0 — capture helper received the real
  authorize URL, **no device-code prompt**, az kept waiting for the callback.
- ✅ **Full path-B chain works end-to-end** — a real `azrl` run popped the
  Windows browser via the reverse tunnel + `wslview`, MFA completed, and az
  obtained a valid token. This proves the **WSL2 localhostForwarding** link (the
  one piece the design flagged as "confirm live") actually carries the callback.
- ✅ **19/19 bats tests pass; shellcheck clean.**

## 6. Environment specifics (the live setup it was built against)

- **VM:** tailnet name `vm-always` (100.95.104.30), Linux, zsh, tmux, az 2.86.0.
- **Local:** `velrada-pc-wsl` (WSL2, zsh, has `wslview`) on the same tailnet;
  the VM can already SSH to it passwordlessly. Windows-native sshd
  (`velrada-pc`) is NOT set up for passwordless from the VM — unused.
- **Tailscale** provides MagicDNS + reachability; **WSL2 localhostForwarding**
  (on by default) carries `localhost:<port>` from Windows to the WSL listener.
- **Global config** `~/.azure-profiles/azrl.conf`: `LOCAL_HOST=velrada-pc-wsl`,
  `LOCAL_BROWSER_CMD=wslview`, `VM_HOST=vm-always`.
- **FIIG profile (`fiig`)** is a **GUEST/B2B** login: `az account show` returns
  `tenantId=96e360c3-4483-43a9-9025-195a431eba14`, **null `tenantDefaultDomain`**,
  user `Simon.Lamb@velrada.com` (a velrada.com identity), default sub
  `7f2e5dbc-…` ("Pay-As-You-Go"). Because the domain `fiig.com.au` appears
  nowhere in the session, `fiig.conf` sets `AZ_TENANT_ID` (the GUID) for the
  assertion while keeping `AZ_TENANT=fiig.com.au` for `az login --tenant`.
- Other profiles present under `~/.azure-profiles/`: `nrg`, `velrada`,
  `digital-it-apps` (no `.conf` yet — create one to use them).

## 7. Current state

- Repo: `~/work/azrl`, fresh `git init`, GitHub remote `slamb2k/azrl` (private).
- Installed: `~/.local/bin/azrl` + `azrl-capture` symlinks on PATH.
- `fiig.conf` populated and validated; the `fiig` token is currently live.
- The old pre-rename copy may still exist at `~/.azure-profiles/azlogin/` as an
  inert backup (no symlink points at it).

## 8. Known limitations / candidate next steps

- **Watchdog/timeout/recovery block is not bats-covered** (E2E-only orchestration
  logic). Consider extracting it into a testable function with shims.
- **Capture timeout is fixed-ish** (`AZRL_CAPTURE_POLL`, ~20s) — fine, but a cold
  az can be slow; revisit if it ever trips.
- **Only `wslview` browser-open is exercised.** `LOCAL_BROWSER_CMD` is pluggable;
  a pure-Windows (pwsh, `Start-Process`) or macOS (`open`) path is untested.
- **No `--help`/usage text or version flag.** Worth adding.
- **No CI.** A GitHub Actions job running bats + shellcheck would be cheap.
- **Multi-profile rollout:** create `<profile>.conf` for `nrg`/`velrada`/etc. and
  drop `.azprofile` files in their repos.
- Possible: package as a Homebrew formula / make `install.sh` idempotent-verified.

## 9. How to work on it

```bash
cd ~/work/azrl
bats tests/azrl.bats                          # unit tests
shellcheck azrl azrl-lib.sh azrl-capture      # lint
bash -n azrl                                  # syntax
# DO NOT run `azrl` casually — it clean-slates the profile and triggers a real login.
# To test pure logic without logging in:  source azrl-lib.sh; azrl_resolve_profile "" "$PWD"
```

Pure functions: `azrl_extract_port`, `azrl_resolve_profile`, `azrl_load_profile_conf`,
`azrl_paste_line`, `azrl_assert_account`, `azrl_clean_slate`, `azrl_login_capture`,
`azrl_bridge`. Globals are `AZRL_*`; the capture env var is `AZRL_CAPFILE`.

## 10. Provenance

Built in a single session via the superpowers brainstorming → writing-plans →
subagent-driven-development workflow. Full design rationale in `docs/design.md`
and the step-by-step TDD build in `docs/build-plan.md` (both **historical** —
written when the tool was `azlogin` at `~/.azure-profiles/azlogin/`; names/paths
there are stale). 14 granular commits from the original build were intentionally
collapsed into this repo's fresh history; this document is the narrative record.
