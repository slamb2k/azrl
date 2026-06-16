# azlogin — reusable Azure CLI login for the remote VM

**Date:** 2026-06-16
**Status:** Design approved, pending implementation
**Lives in:** `~/.local/bin/azlogin` (script) + `~/.azure-profiles/` (config). Cross-project; intentionally NOT inside any repo.

## Problem

`az login` on the remote VM (`vm-always`, accessed via plain SSH in wezterm + persistent tmux) is painful:

1. **Random callback port, no local browser.** MSAL's interactive auth-code flow binds a *random* `127.0.0.1:<port>` listener on the VM and there is no flag to pin it. On a headless box az silently *falls back to device code* — which the FIIG Conditional Access policy **blocks**. The browser lives on the local Windows machine and has no route to the VM's loopback.
2. **Wrong/stale user silently reused.** Cached tokens / a stale default account cause logins to land as the wrong identity.
3. **Multiple profiles.** Four isolated Azure identities (`digital-it-apps`, `fiig`, `nrg`, `velrada`) under `~/.azure-profiles/`, and the *right* one depends on the repo — sometimes two tenants for one repo.

## Constraints / environment (verified 2026-06-16)

- az CLI 2.86.0 on the VM. No `--redirect-uri`/`--port` flag exists; no stable port (azure-cli #26556).
- Device code (`--use-device-code`) is OUT (CA-blocked).
- `AZURE_CONFIG_DIR` gives **fully isolated** login state per directory (own `azureProfile.json` + `msal_token_cache.json`). Confirmed.
- Tailnet: VM is `vm-always` (100.95.104.30). Local is `velrada-pc-wsl` (100.106.30.55, WSL/zsh, **active**) and `velrada-pc` (Windows, active).
- **VM → `velrada-pc-wsl` SSH works passwordlessly (key auth); `wslview` present** (opens the Windows default browser from WSL). VM → `velrada-pc` (Windows sshd) key auth is NOT set up — unused.
- User SSHes in *from* `velrada-pc-wsl`, lives inside tmux on the VM.

## Architecture

**One script: `~/.local/bin/azlogin`, run inside tmux on the VM, in the target repo.** No local helper (the repo context lives on the VM, so the VM drives). The only thing the VM can't do alone is pop the local browser — handled by a tailnet callback (path B) with a paste-line fallback (path A).

### Flow

```
azlogin [profile] [--paste]
  1. Resolve profile:
       - explicit arg wins
       - else read `.azprofile` from the git root (walk up from $PWD)
       - else error with guidance
  2. export AZURE_CONFIG_DIR=~/.azure-profiles/<profile>
     source ~/.azure-profiles/<profile>.conf   # AZ_TENANT, optional AZ_DEFAULT_SUB, AZ_EXPECT_USER
  3. Clean slate (scoped to this profile dir only):
       az logout            # ignore errors
       az account clear     # clears azureProfile.json + token cache
       rm -f $AZURE_CONFIG_DIR/{msal_token_cache.json,service_principal_entries.json}
  4. Start `az login --tenant $AZ_TENANT` with BROWSER set to a capture script
       (prevents device-code fallback; records the auth URL). Parse PORT from
       redirect_uri=http://localhost:<PORT> in the captured URL.
  5. Bridge the browser:
       B (default): reverse tunnel + remote browser-open over the tailnet
                    ssh -fN -R $PORT:localhost:$PORT velrada-pc-wsl
                    ssh velrada-pc-wsl 'wslview "<URL>"'
                    (fall through to A if velrada-pc-wsl unreachable, or --paste given)
       A (fallback): print ONE paste-ready local line, e.g.
                    ssh -fNL $PORT:localhost:$PORT vm-always && wslview "<URL>"
  6. Wait for `az login` to complete; tear down the tunnel.
  7. Post-login verification:
       az account set --subscription $AZ_DEFAULT_SUB   # if set
       az account show -> assert tenant matches $AZ_TENANT (and user matches
       $AZ_EXPECT_USER if set). Loud red warning + nonzero exit on mismatch;
       green ✓ summary (user / tenant / sub) on success.
```

### The BROWSER-capture mechanism (key assumption — validate FIRST)

az/MSAL opens the browser via Python's `webbrowser`, which honours `$BROWSER`. We set
`BROWSER="$HOME/.local/bin/azlogin-capture %s"`, a tiny script that writes its `$1`
(the auth URL) to a temp file and exits 0. az believes a browser launched, so it keeps
the auth-code listener alive instead of falling back to device code, and we read the URL
(and its `redirect_uri` port) from the temp file.

**Validation before relying on it:** run `az login` under a throwaway `AZURE_CONFIG_DIR`
with the capture `BROWSER`, confirm (a) the capture script receives the URL and (b) no
device-code prompt appears. Ctrl-C after capture (no real auth needed). **Fallback if az
ignores `$BROWSER`:** `az login --debug 2>&1 | grep -i redirect_uri` to scrape the port,
and drive the browser from the scraped URL. Design is unchanged either way.

### The B callback chain (validate localhostForwarding link)

```
VM az listener (127.0.0.1:PORT)
  <- ssh -R tunnel ->  WSL 127.0.0.1:PORT (velrada-pc-wsl)
  <- WSL2 localhostForwarding ->  Windows localhost:PORT
  <- Windows default browser (launched by wslview)
```

WSL2 `localhostForwarding` is on by default, so a Windows-side `localhost:PORT` reaches
the WSL listener. This is the one link to confirm live; if it ever fails, A (paste) works
because the local `ssh -L` listener binds inside WSL where the user pastes it.

## Files & layout

```
~/.local/bin/azlogin            # main script (bash/zsh-compatible)
~/.local/bin/azlogin-capture    # BROWSER capture helper
~/.azure-profiles/
    azlogin.conf                # global: LOCAL_HOST=velrada-pc-wsl, LOCAL_BROWSER_CMD=wslview, VM_HOST=vm-always
    fiig.conf                   # AZ_TENANT=fiig.com.au, AZ_EXPECT_USER=..., AZ_DEFAULT_SUB=...
    digital-it-apps.conf
    nrg.conf
    velrada.conf
    fiig/ digital-it-apps/ nrg/ velrada/   # existing AZURE_CONFIG_DIR dirs (token caches)
<repo>/.azprofile               # one line: the profile name; UNCOMMITTED (global gitignore)
```

`.azprofile` is added to the global gitignore (`~/.config/git/ignore`) so it never
pollutes a repo. For the FIIG repo: `.azprofile` = `fiig`; run `azlogin digital-it-apps`
for the Dataverse/formrule work that targets the other tenant.

## Out of scope

- A separate local-machine helper (dropped: VM has the repo context; VM drives).
- Windows-native sshd callback (`velrada-pc`) — WSL path covers it.
- Service-principal / managed-identity login (this is interactive human login).
- Auto-refresh of expiring PIM/Contributor roles (separate concern).

## Open items to validate during implementation

1. **BROWSER-capture** actually prevents device-code fallback on az 2.86.0 (primary risk).
2. **WSL2 localhostForwarding** carries the tunnel to the Windows browser.
3. ~~Exact `az login` completion signal to key tunnel teardown off (process exit vs. token-cache write).~~ **RESOLVED:** the EXIT trap kills the tunnel, watchdog, and login pid; login completion is detected via `wait` on the login pid, bounded by a watchdog timeout (`AZL_LOGIN_TIMEOUT`, default 180s). Login failure/timeout now degrades to the path-A paste recovery hint instead of hanging.
4. Per-profile `.conf` values (tenants/subs/expected users) — fill in real values; `fiig` tenant is `fiig.com.au`.
```

## Spike result

BROWSER-capture confirmed on az 2.86.0 on 2026-06-16; captured port 45715.

## Guest/B2B tenants and identity assertion

For guest (B2B) logins, `az account show` may return `tenantId` only with a null `tenantDefaultDomain`, and the UPN domain differs from the target tenant (e.g. accessing fiig.com.au as Simon.Lamb@velrada.com). The domain therefore cannot anchor the identity check. Profiles set `AZ_TENANT_ID` (the tenant GUID) for the assertion while keeping `AZ_TENANT` (domain) for `az login --tenant`. The orchestrator asserts against `${AZ_TENANT_ID:-$AZ_TENANT}`.
