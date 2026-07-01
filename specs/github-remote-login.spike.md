# GitHub Remote Login — Phase 0 Spike Findings

- **Date:** 2026-07-01
- **Gates:** `specs/github-remote-login.md` §"Phase 0 — Spike"
- **Method:** local probing on this headless Linux VM + official docs/source review.
- **Labels:** `[VERIFIED-LOCAL]` confirmed on this VM · `[VERIFIED-DOCS]` confirmed via
  official docs/source (URL cited) · `[NEEDS-REAL-ENV]` only closable with a real
  laptop+VM+GCM+VS Code interactive login.

**VM environment (probed):** `gh` 2.95.0; `git` 2.55.0; `BROWSER=xdg-open`;
`/usr/bin/xdg-open` present; `secret-tool` present (linuxbrew) and a DBUS session
bus exists, but `gh`'s live token is stored **plaintext** in
`~/.config/gh/hosts.yml` (no functional keyring unlock in this headless session).
**GCM is NOT installed** here (`git-credential-manager*` absent; no
`credential.helper` set). VS Code full app is **not** present — only the
`.vscode-server` remote agent (Remote-SSH server side).

---

## 1. `gh` token storage & isolation

**Label:** `[VERIFIED-LOCAL]` + `[VERIFIED-DOCS]`

**Finding.** `gh` tries the OS system credential store (keyring) **first** and falls
back to a plaintext `hosts.yml` under the config dir when none is available. On this
headless VM it fell back to plaintext — `gh auth status` reports the token stored at
`~/.config/gh/hosts.yml`, and the file contains the `gho_…` token in cleartext.
`GH_CONFIG_DIR` isolates the config directory files (`config.yml`, `hosts.yml`,
`state.yml`), **but the keyring is global** — entries are keyed by
`gh:<hostname>` + username (`internal/config/config.go`), with nothing derived from
`GH_CONFIG_DIR`. So **when a functional keyring is present, per-profile
`GH_CONFIG_DIR` alone does NOT guarantee token isolation**: two profiles at the same
host+username read/write the same shared keyring entry and collide.

**Fix / design impact.** Force plaintext file storage per profile with
`gh auth login --insecure-storage` (documented flag: "Save authentication
credentials in plain text instead of credential store"). Combined with a per-profile
`GH_CONFIG_DIR`, each profile's token then lives in its own `hosts.yml` — true
isolation regardless of keyring presence or duplicate usernames. There is **no**
env-var/config-key equivalent; the flag must be passed at each `gh auth login`. The
azrl design's per-profile `GH_CONFIG_DIR` model **holds, with the added requirement
that `internal/github.Login` always pass `--insecure-storage`.** (Native gh 2.40+
multi-account — a `users:` map + `gh auth switch` — is an alternative but does not
match azrl's isolated-config-dir model; recommend `--insecure-storage`.)

**Evidence:**
- `gh auth login --help`: keyring-first + plaintext fallback; `--insecure-storage` flag.
- Live: token in `~/.config/gh/hosts.yml` cleartext on this VM.
- https://cli.github.com/manual/gh_auth_login
- https://cli.github.com/manual/gh_help_environment (`GH_CONFIG_DIR` scope)
- Keyring key = `gh:<host>`+username, not partitioned by config dir:
  https://github.com/cli/cli/blob/trunk/internal/config/config.go
- Multi-account model: https://github.com/cli/cli/blob/trunk/docs/multiple-accounts.md

---

## 2. GCM `$BROWSER` hook

**Label:** `[VERIFIED-DOCS]` (GCM not installable-verified locally — not present on VM)

**Finding.** **GCM does NOT honor `$BROWSER` on Linux.** Its
`LinuxSessionManager.OpenBrowser` → `TryGetShellExecuteHandler` searches PATH in a
fixed order — `xdg-open`, `gnome-open`, `kfmclient`, `wslview` — and execs the first
found. `$BROWSER` is only consulted for remote-session *detection*, never as the
launcher. There is no `GCM_*` env var to override the browser command.

**Fix / design impact.** The spec's assumption that setting `$BROWSER` to the shim
fires it for GCM is **wrong for GCM** — a targeted revision is required. Two viable
mechanisms:
- **(a) Shadow `xdg-open`**: put the `__browser` shim on GCM's PATH as an executable
  named `xdg-open`, ahead of `/usr/bin/xdg-open`. GCM then invokes the shim with the
  authorize URL, which the shim parses/tunnels exactly like the Azure path. (Cleanest
  fit with the existing capture pattern.)
- **(b) Force device-code flow**: set `GCM_GITHUB_AUTHMODES=device` (env) or
  `git config credential.gitHubAuthModes device`, so GCM never launches a browser and
  the shim only relays a device code — mirrors the `gh` relay path, no loopback tunnel.

Recommend implementing **(a)** as the general loopback bridge and documenting **(b)**
as the simpler headless default. Note: `$BROWSER=shim` still works for `gh` and for
tools that respect it; the shadow-`xdg-open` requirement is GCM-specific.

**Evidence:**
- https://github.com/git-ecosystem/git-credential-manager/blob/main/src/shared/Core/Interop/Linux/LinuxSessionManager.cs
- https://github.com/git-ecosystem/git-credential-manager/blob/main/docs/environment.md
- https://github.com/git-ecosystem/git-credential-manager/blob/main/docs/configuration.md

---

## 3. GCM authorize URL shape (loopback port)

**Label:** `[VERIFIED-DOCS]`

**Finding.** Confirmed. GCM's GitHub browser flow uses a localhost loopback redirect:
`GitHubConstants.OAuthRedirectUri = "http://127.0.0.1/"` (legacy GHES:
`http://localhost/`). At runtime `OAuth2SystemWebBrowser` binds
`new TcpListener(IPAddress.Loopback, 0)` to get an **OS-assigned ephemeral port**,
then builds `/login/oauth/authorize?…&redirect_uri=http://127.0.0.1:<port>/…`. The
code enforces "Only localhost is supported as a redirect URI." So the shim can parse
`<port>` from the `redirect_uri` query param on each invocation.

**Design impact.** Confirms the shim's port-parsing approach; the port is random per
invocation (parse it, don't assume a fixed one) — exactly as the spec states. Holds
as-is.

**Evidence:**
- https://github.com/git-ecosystem/git-credential-manager/blob/main/src/shared/GitHub/GitHubConstants.cs
- https://github.com/git-ecosystem/git-credential-manager/blob/main/src/shared/Core/Authentication/OAuth/OAuth2SystemWebBrowser.cs

---

## 4. Two same-host accounts (per-repo token selection)

**Label:** `[VERIFIED-DOCS]`

**Finding.** By default git shares ONE credential across all of `github.com` (path
ignored). To pin a repo to one account, GCM's own `docs/multiple-users.md` prescribes
binding an identity via `credential.<url>.username` plus an identity-bearing remote
URL — NOT `useHttpPath` and NOT `includeIf`. GCM uses that username to select/store
the matching token, so a pinned repo never authenticates as the other account.
Separately, `gh auth setup-git` registers `gh` as the credential helper and the token
it returns is scoped by `GH_CONFIG_DIR` — two repos pointing at two different
`GH_CONFIG_DIR`s resolve to two independent accounts.

**Concrete config that guarantees repo→account A (per-repo, robust):**
```
git config --local credential.https://github.com.username alice
git remote set-url origin https://alice@github.com/org/repo.git
```
Optional host-level partitioning of distinct tokens per `org/repo`:
```
git config --global credential.useHttpPath true
```

**Design impact.** The spec's `gh use` (writes `.ghprofile` + runs `gh auth
setup-git` scoped to the profile's `GH_CONFIG_DIR`) is a valid isolation path *for the
gh-as-helper route*. For the GCM route, `internal/github.Use` should additionally set
`credential.https://<host>.username <GH_USER>` in the repo's local config (and
optionally rewrite the remote to the identity-bearing form) to guarantee no
cross-account push. Holds with this concrete addition to `Use`.

**Evidence:**
- https://github.com/git-ecosystem/git-credential-manager/blob/main/docs/multiple-users.md
- https://git-scm.com/docs/gitcredentials · https://git-scm.com/docs/git-config
- https://cli.github.com/manual/gh_auth_setup-git · https://cli.github.com/manual/gh_help_environment

**Residual:** `[NEEDS-REAL-ENV]` — behavioural proof that a pinned push authenticates
as A (not B) needs two real accounts + GCM installed.

---

## 5. VS Code GitHub auth (Remote-SSH)

**Label:** `[VERIFIED-DOCS]` (full VS Code app not on this VM — only `.vscode-server`)

**Finding.** VS Code's built-in GitHub auth does **not** bind a raw `127.0.0.1:PORT`
listener on the VM and does **not** rely on `$BROWSER`. It uses a `vscode://`
protocol/URI handler: the OAuth redirect lands on a web page that bounces to
`vscode://vscode.github-authentication/…`, which the OS routes to the desktop VS Code
app on the laptop. Built-in auth providers run UI-side (local), so the browser
sign-in and callback complete on the laptop; the token is then surfaced to the remote
server. Remote Development "transparently handles passing the URI … regardless of
where it is actually running," and `vscode.env.asExternalUri` (1.40+) auto-forwards
any `localhost:PORT`-style callback. **No user action or external tunnel is needed.**

**Design impact.** **VS Code's built-in GitHub auth is already handled by VS Code
Remote — the tool should NOT actively bridge it; document it as handled.** (The
azrl-style SSH-tunnel bridge remains relevant only for third-party CLIs/extensions
that hardcode a raw loopback listener on the VM and skip `asExternalUri` — i.e. GCM,
not VS Code built-in auth.) This lets the spec drop "VS Code (tunnel; pending spike)"
to "VS Code — handled by Remote-SSH, documented."

**Evidence:**
- https://code.visualstudio.com/api/advanced-topics/remote-extensions
- https://code.visualstudio.com/docs/remote/ssh
- https://code.visualstudio.com/docs/sourcecontrol/github
- Failure mode for raw-loopback third-party tools: https://github.com/microsoft/vscode/issues/203869

---

## Design verdict

The design **HOLDS with two targeted revisions**, both mechanical:

1. **Item 1:** `internal/github.Login` must pass `gh auth login --insecure-storage`
   so per-profile `GH_CONFIG_DIR` gives real isolation even where a keyring exists.
2. **Item 2:** GCM ignores `$BROWSER` on Linux — the shim must be wired via
   **shadowing `xdg-open` on PATH** (or, simpler headless default, forcing
   `GCM_GITHUB_AUTHMODES=device`). The "set `$BROWSER` and GCM fires the shim"
   assumption is invalid for GCM specifically.

Item 3 (loopback port parse) and Item 5 (VS Code) confirm the design as written —
and Item 5 simplifies it (drop VS Code active-bridging; document as handled by
Remote-SSH). Item 4's `Use` should additionally set
`credential.https://<host>.username` per repo.

**Still requires the user's real laptop/VM/GCM/VS Code to close out:**
- End-to-end GCM git-HTTPS push through the shadow-`xdg-open` shim + SSH reverse
  tunnel (Items 2–4 behavioural proof) — GCM is not installed here.
- Behavioural proof that a repo pinned to account A never pushes as B with two live
  accounts (Item 4).
- Confirming `gh --insecure-storage` isolation on a VM that DOES have a functional
  unlocked keyring (this VM had none) (Item 1 edge).
- VS Code built-in GitHub sign-in over Remote-SSH observed working with zero tunnel
  (Item 5 confirmation) — full VS Code app not on this VM.
