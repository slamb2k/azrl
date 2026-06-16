# azlogin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A single remote-VM script `azlogin` that logs the Azure CLI into the correct per-repo profile, from a clean slate, popping the browser on the local Windows machine over Tailscale with zero paste (fallback: one paste line).

**Architecture:** One orchestrator (`azlogin`) sources a library of pure functions (`azlogin-lib.sh`) and a BROWSER-capture helper (`azlogin-capture`). It resolves the profile from `.azprofile`/arg, isolates state via `AZURE_CONFIG_DIR`, clean-slates, runs `az login` while capturing the random callback port via `$BROWSER`, bridges that port to the local browser via a reverse SSH tunnel + `wslview` (path B) or a printed paste line (path A), then asserts the resulting identity.

**Tech Stack:** bash, Azure CLI 2.86.0, OpenSSH (reverse tunnels), Tailscale (MagicDNS), WSL2 `wslview` + localhostForwarding, jq. Tests: bats-core. Lint: shellcheck/shfmt.

**Source layout (git repo at `~/.azure-profiles/azlogin/`):**
```
~/.azure-profiles/azlogin/            # git repo (source of truth)
    azlogin                           # orchestrator (symlinked to ~/.local/bin/azlogin)
    azlogin-capture                   # BROWSER capture helper (symlinked)
    azlogin-lib.sh                    # pure, sourceable functions (unit-tested)
    azlogin.conf.example              # global config template
    profile.conf.example              # per-profile config template
    install.sh                        # symlink + gitignore + conf bootstrap
    tests/azlogin.bats                # bats unit tests
    azlogin-design.md                 # the approved spec (moved here in Task 1)
    azlogin-plan.md                   # this plan (moved here in Task 1)
~/.local/bin/azlogin -> ../../.azure-profiles/azlogin/azlogin          # symlink, on PATH
~/.local/bin/azlogin-capture -> ../../.azure-profiles/azlogin/azlogin-capture
~/.azure-profiles/azlogin.conf        # REAL global config (not in repo)
~/.azure-profiles/<profile>.conf      # REAL per-profile config (not in repo)
~/.azure-profiles/<profile>/          # existing token-cache dirs (secrets, never in repo)
```

---

## Task 1: Scaffold the source repo

**Files:**
- Create: `~/.azure-profiles/azlogin/` (git repo)
- Create: `~/.azure-profiles/azlogin/.gitignore`
- Move: `~/.azure-profiles/azlogin-design.md` → `~/.azure-profiles/azlogin/azlogin-design.md`
- Move: `~/.azure-profiles/azlogin-plan.md` → `~/.azure-profiles/azlogin/azlogin-plan.md`

- [ ] **Step 1: Create and init the repo**

```bash
mkdir -p ~/.azure-profiles/azlogin/tests
cd ~/.azure-profiles/azlogin
git init
```

- [ ] **Step 2: Add a .gitignore (defensive — keep any stray secrets out)**

Create `~/.azure-profiles/azlogin/.gitignore`:

```
*.conf
!*.conf.example
msal_token_cache.json
service_principal_entries.json
*.log
```

- [ ] **Step 3: Move the design + plan docs into the repo**

```bash
mv ~/.azure-profiles/azlogin-design.md ~/.azure-profiles/azlogin/azlogin-design.md
mv ~/.azure-profiles/azlogin-plan.md   ~/.azure-profiles/azlogin/azlogin-plan.md
```

- [ ] **Step 4: Commit**

```bash
cd ~/.azure-profiles/azlogin
git add -A
git commit -m "chore(azlogin): scaffold source repo with spec and plan"
```

---

## Task 2: SPIKE — validate the BROWSER-capture mechanism (primary risk)

**Goal:** Prove that setting `$BROWSER` to a capture script (a) receives the auth URL and (b) stops `az login` from falling back to device code. This gates the whole design. **Do this before building anything else.**

**Files:**
- Create: `~/.azure-profiles/azlogin/azlogin-capture`

- [ ] **Step 1: Write the capture helper**

Create `~/.azure-profiles/azlogin/azlogin-capture`:

```bash
#!/usr/bin/env bash
# Invoked by Python's webbrowser (via $BROWSER) as: azlogin-capture <url>
# Records the URL so the caller can read the random callback port, then exits 0
# so az believes a browser launched (preventing device-code fallback).
set -euo pipefail
printf '%s' "${1:-}" > "${AZLOGIN_CAPFILE:?AZLOGIN_CAPFILE not set}"
exit 0
```

```bash
chmod +x ~/.azure-profiles/azlogin/azlogin-capture
```

- [ ] **Step 2: Run the spike against a throwaway config dir**

Run (interactively; you do NOT need to complete sign-in):

```bash
export AZLOGIN_CAPFILE="$(mktemp)"
export AZURE_CONFIG_DIR="$(mktemp -d)"
export BROWSER="$HOME/.azure-profiles/azlogin/azlogin-capture %s"
timeout 20 az login --tenant fiig.com.au --only-show-errors; echo "exit=$?"
echo "--- captured ---"; cat "$AZLOGIN_CAPFILE"; echo
```

Expected: **NO "To sign in, use a web browser ... enter the code" device-code prompt.** The command blocks waiting for the browser callback (the `timeout 20` ends it). `$AZLOGIN_CAPFILE` contains a URL with `redirect_uri=...localhost%3A<port>` (or `localhost:<port>`).

- [ ] **Step 3: Record the outcome**

- If capture works and no device code: append a `## Spike result` note to `azlogin-design.md` ("BROWSER-capture confirmed on az 2.86.0, <date>") and proceed.
- If a device-code prompt appears instead: the `$BROWSER` path failed. Switch the plan's capture mechanism (Task 9) to scrape the port from `az login --debug 2>&1 | grep -i redirect_uri` and drive the browser from that URL. The rest of the plan is unchanged.

- [ ] **Step 4: Clean up the spike**

```bash
rm -rf "$AZURE_CONFIG_DIR" "$AZLOGIN_CAPFILE"
unset AZURE_CONFIG_DIR AZLOGIN_CAPFILE BROWSER
```

- [ ] **Step 5: Commit the helper + spike note**

```bash
cd ~/.azure-profiles/azlogin
git add azlogin-capture azlogin-design.md
git commit -m "feat(azlogin): add BROWSER-capture helper; validate against az 2.86.0"
```

---

## Task 3: Port extraction (TDD)

**Files:**
- Create: `~/.azure-profiles/azlogin/azlogin-lib.sh`
- Test: `~/.azure-profiles/azlogin/tests/azlogin.bats`

- [ ] **Step 1: Write the failing test**

Create `~/.azure-profiles/azlogin/tests/azlogin.bats`:

```bash
#!/usr/bin/env bats

setup() {
  load_lib() { source "${BATS_TEST_DIRNAME}/../azlogin-lib.sh"; }
  load_lib
}

@test "azl_extract_port: url-encoded redirect_uri" {
  url='https://login.microsoftonline.com/x?redirect_uri=http%3A%2F%2Flocalhost%3A38149%2F&state=y'
  run azl_extract_port "$url"
  [ "$status" -eq 0 ]
  [ "$output" = "38149" ]
}

@test "azl_extract_port: plain redirect_uri" {
  url='https://login.microsoftonline.com/x?redirect_uri=http://localhost:55322/&state=y'
  run azl_extract_port "$url"
  [ "$output" = "55322" ]
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: FAIL (`azlogin-lib.sh` does not exist / function not found).

- [ ] **Step 3: Write minimal implementation**

Create `~/.azure-profiles/azlogin/azlogin-lib.sh`:

```bash
#!/usr/bin/env bash
# Pure, sourceable helpers for azlogin. No side effects on source.

azl_extract_port() {
  local url="$1" decoded
  decoded="${url//%3A/:}"; decoded="${decoded//%2F//}"
  printf '%s' "$decoded" | grep -oE 'localhost:[0-9]+' | head -1 | cut -d: -f2
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
cd ~/.azure-profiles/azlogin
git add azlogin-lib.sh tests/azlogin.bats
git commit -m "feat(azlogin): extract callback port from auth URL"
```

---

## Task 4: Profile resolution (TDD)

**Files:**
- Modify: `~/.azure-profiles/azlogin/azlogin-lib.sh`
- Test: `~/.azure-profiles/azlogin/tests/azlogin.bats`

- [ ] **Step 1: Write the failing test**

Append to `tests/azlogin.bats`:

```bash
@test "azl_resolve_profile: explicit arg wins" {
  run azl_resolve_profile "fiig" "/tmp"
  [ "$output" = "fiig" ]
}

@test "azl_resolve_profile: reads .azprofile walking up" {
  tmp="$(mktemp -d)"; mkdir -p "$tmp/a/b"
  printf 'digital-it-apps\n' > "$tmp/.azprofile"
  run azl_resolve_profile "" "$tmp/a/b"
  [ "$status" -eq 0 ]
  [ "$output" = "digital-it-apps" ]
  rm -rf "$tmp"
}

@test "azl_resolve_profile: no arg, no file -> error" {
  tmp="$(mktemp -d)"
  run azl_resolve_profile "" "$tmp"
  [ "$status" -ne 0 ]
  rm -rf "$tmp"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: FAIL (`azl_resolve_profile` not found) for the 3 new tests.

- [ ] **Step 3: Write minimal implementation**

Append to `azlogin-lib.sh`:

```bash
azl_resolve_profile() {
  local arg="$1" dir="${2:-$PWD}"
  if [[ -n "$arg" ]]; then printf '%s\n' "$arg"; return 0; fi
  local d="$dir"
  while [[ -n "$d" && "$d" != "/" ]]; do
    if [[ -f "$d/.azprofile" ]]; then
      tr -d '[:space:]' < "$d/.azprofile"; printf '\n'; return 0
    fi
    d="$(dirname "$d")"
  done
  printf 'azlogin: no profile arg and no .azprofile found from %s\n' "$dir" >&2
  return 1
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: PASS (all tests).

- [ ] **Step 5: Commit**

```bash
cd ~/.azure-profiles/azlogin
git add azlogin-lib.sh tests/azlogin.bats
git commit -m "feat(azlogin): resolve profile from arg or .azprofile"
```

---

## Task 5: Profile config loading + validation (TDD)

**Files:**
- Modify: `~/.azure-profiles/azlogin/azlogin-lib.sh`
- Test: `~/.azure-profiles/azlogin/tests/azlogin.bats`

- [ ] **Step 1: Write the failing test**

Append to `tests/azlogin.bats`:

```bash
@test "azl_load_profile_conf: sources tenant and returns 0" {
  tmp="$(mktemp -d)"
  printf 'AZ_TENANT=fiig.com.au\nAZ_DEFAULT_SUB=sub-123\n' > "$tmp/fiig.conf"
  run bash -c "source '${BATS_TEST_DIRNAME}/../azlogin-lib.sh'; azl_load_profile_conf fiig '$tmp'; echo \"\$AZ_TENANT\""
  [ "$status" -eq 0 ]
  [[ "$output" == *"fiig.com.au"* ]]
  rm -rf "$tmp"
}

@test "azl_load_profile_conf: missing file -> error" {
  tmp="$(mktemp -d)"
  run azl_load_profile_conf nope "$tmp"
  [ "$status" -ne 0 ]
  rm -rf "$tmp"
}

@test "azl_load_profile_conf: missing AZ_TENANT -> error" {
  tmp="$(mktemp -d)"
  printf 'AZ_DEFAULT_SUB=x\n' > "$tmp/bad.conf"
  run azl_load_profile_conf bad "$tmp"
  [ "$status" -ne 0 ]
  rm -rf "$tmp"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: FAIL (`azl_load_profile_conf` not found).

- [ ] **Step 3: Write minimal implementation**

Append to `azlogin-lib.sh`:

```bash
azl_load_profile_conf() {
  local profile="$1" confdir="${2:-$HOME/.azure-profiles}"
  local f="$confdir/$profile.conf"
  [[ -f "$f" ]] || { printf 'azlogin: missing config %s\n' "$f" >&2; return 1; }
  # shellcheck disable=SC1090
  source "$f"
  [[ -n "${AZ_TENANT:-}" ]] || { printf 'azlogin: AZ_TENANT not set in %s\n' "$f" >&2; return 1; }
  return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: PASS (all tests).

- [ ] **Step 5: Commit**

```bash
cd ~/.azure-profiles/azlogin
git add azlogin-lib.sh tests/azlogin.bats
git commit -m "feat(azlogin): load and validate per-profile config"
```

---

## Task 6: Paste-line builder — path A (TDD)

**Files:**
- Modify: `~/.azure-profiles/azlogin/azlogin-lib.sh`
- Test: `~/.azure-profiles/azlogin/tests/azlogin.bats`

- [ ] **Step 1: Write the failing test**

Append to `tests/azlogin.bats`:

```bash
@test "azl_paste_line: builds local forward+open command" {
  run azl_paste_line 38149 vm-always wslview 'https://login/x?y=z'
  [ "$status" -eq 0 ]
  [ "$output" = 'ssh -fNL 38149:localhost:38149 vm-always && wslview "https://login/x?y=z"' ]
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: FAIL (`azl_paste_line` not found).

- [ ] **Step 3: Write minimal implementation**

Append to `azlogin-lib.sh`:

```bash
azl_paste_line() {
  # $1=port $2=vm_host $3=browser_cmd $4=url
  printf 'ssh -fNL %s:localhost:%s %s && %s "%s"\n' "$1" "$1" "$2" "$3" "$4"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd ~/.azure-profiles/azlogin
git add azlogin-lib.sh tests/azlogin.bats
git commit -m "feat(azlogin): build path-A paste line"
```

---

## Task 7: Account assertion (TDD)

**Files:**
- Modify: `~/.azure-profiles/azlogin/azlogin-lib.sh`
- Test: `~/.azure-profiles/azlogin/tests/azlogin.bats`

- [ ] **Step 1: Write the failing test**

Append to `tests/azlogin.bats`:

```bash
@test "azl_assert_account: matches tenant domain and user" {
  json='{"tenantId":"guid-1","tenantDefaultDomain":"fiig.com.au","user":{"name":"simon@fiig.com.au"},"name":"sub"}'
  run azl_assert_account "$json" "fiig.com.au" "simon@fiig.com.au"
  [ "$status" -eq 0 ]
}

@test "azl_assert_account: matches tenant by GUID" {
  json='{"tenantId":"guid-1","tenantDefaultDomain":"fiig.com.au","user":{"name":"x"}}'
  run azl_assert_account "$json" "guid-1" ""
  [ "$status" -eq 0 ]
}

@test "azl_assert_account: tenant mismatch -> error" {
  json='{"tenantId":"guid-1","tenantDefaultDomain":"other.com","user":{"name":"x"}}'
  run azl_assert_account "$json" "fiig.com.au" ""
  [ "$status" -ne 0 ]
}

@test "azl_assert_account: user mismatch -> error" {
  json='{"tenantId":"g","tenantDefaultDomain":"fiig.com.au","user":{"name":"wrong@x"}}'
  run azl_assert_account "$json" "fiig.com.au" "right@x"
  [ "$status" -ne 0 ]
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: FAIL (`azl_assert_account` not found).

- [ ] **Step 3: Write minimal implementation**

Append to `azlogin-lib.sh`:

```bash
azl_assert_account() {
  # $1=account_json $2=expected_tenant (GUID or domain) $3=expected_user (optional)
  local json="$1" exp_tenant="$2" exp_user="$3"
  local got_tenant got_domain got_user
  got_tenant="$(printf '%s' "$json" | jq -r '.tenantId // empty')"
  got_domain="$(printf '%s' "$json" | jq -r '.tenantDefaultDomain // empty')"
  got_user="$(printf '%s' "$json" | jq -r '.user.name // empty')"
  if [[ "$exp_tenant" != "$got_tenant" && "$exp_tenant" != "$got_domain" ]]; then
    printf 'azlogin: TENANT MISMATCH — expected %q, got tenantId=%q domain=%q\n' \
      "$exp_tenant" "$got_tenant" "$got_domain" >&2
    return 1
  fi
  if [[ -n "$exp_user" && "$exp_user" != "$got_user" ]]; then
    printf 'azlogin: USER MISMATCH — expected %q, got %q\n' "$exp_user" "$got_user" >&2
    return 1
  fi
  return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd ~/.azure-profiles/azlogin
git add azlogin-lib.sh tests/azlogin.bats
git commit -m "feat(azlogin): assert logged-in tenant/user matches profile"
```

---

## Task 8: Clean-slate function (TDD with az shim)

**Files:**
- Modify: `~/.azure-profiles/azlogin/azlogin-lib.sh`
- Test: `~/.azure-profiles/azlogin/tests/azlogin.bats`

- [ ] **Step 1: Write the failing test**

Append to `tests/azlogin.bats`:

```bash
@test "azl_clean_slate: calls az logout+clear and removes only scoped caches" {
  shimdir="$(mktemp -d)"; cfg="$(mktemp -d)"
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
echo "az $*" >> "$AZ_SHIM_LOG"
EOF
  chmod +x "$shimdir/az"
  export AZ_SHIM_LOG="$cfg/shim.log"
  export AZURE_CONFIG_DIR="$cfg"
  : > "$cfg/msal_token_cache.json"
  : > "$cfg/service_principal_entries.json"
  PATH="$shimdir:$PATH" run azl_clean_slate
  [ "$status" -eq 0 ]
  grep -q "az logout" "$AZ_SHIM_LOG"
  grep -q "az account clear" "$AZ_SHIM_LOG"
  [ ! -f "$cfg/msal_token_cache.json" ]
  [ ! -f "$cfg/service_principal_entries.json" ]
  rm -rf "$shimdir" "$cfg"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: FAIL (`azl_clean_slate` not found).

- [ ] **Step 3: Write minimal implementation**

Append to `azlogin-lib.sh`:

```bash
azl_clean_slate() {
  # Operates only within $AZURE_CONFIG_DIR (isolated profile).
  az logout >/dev/null 2>&1 || true
  az account clear >/dev/null 2>&1 || true
  rm -f "${AZURE_CONFIG_DIR:?}/msal_token_cache.json" \
        "${AZURE_CONFIG_DIR:?}/service_principal_entries.json"
  return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd ~/.azure-profiles/azlogin
git add azlogin-lib.sh tests/azlogin.bats
git commit -m "feat(azlogin): clean-slate scoped to profile config dir"
```

---

## Task 9: Login driver with port capture (TDD with az shim + live check)

**Files:**
- Modify: `~/.azure-profiles/azlogin/azlogin-lib.sh`
- Test: `~/.azure-profiles/azlogin/tests/azlogin.bats`

> If Task 2's spike forced the `--debug` fallback, implement `azl_login_capture` to scrape `az login --debug 2>&1 | grep -i redirect_uri` instead of using `$BROWSER`; keep the same outputs (`AZL_PORT`, `AZL_URL`, `AZL_LOGIN_PID`, `AZL_CAPFILE`).

- [ ] **Step 1: Write the failing test (shimmed az writes the capture file like the real BROWSER path would)**

Append to `tests/azlogin.bats`:

```bash
@test "azl_login_capture: sets AZL_PORT and AZL_URL from captured browser URL" {
  shimdir="$(mktemp -d)"
  # Fake az: emulate webbrowser by invoking $BROWSER with a URL, then block briefly.
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F&state=z'
# $BROWSER is like "/path/azlogin-capture %s"
cmd="${BROWSER/\%s/$url}"
eval "$cmd"
sleep 2
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azlogin-lib.sh'
    export AZLOGIN_CAPTURE='${BATS_TEST_DIRNAME}/../azlogin-capture'
    PATH='$shimdir:\$PATH' azl_login_capture fiig.com.au
    echo \"PORT=\$AZL_PORT URL=\$AZL_URL\"
    kill \$AZL_LOGIN_PID 2>/dev/null || true
  "
  [ "$status" -eq 0 ]
  [[ "$output" == *"PORT=40404"* ]]
  rm -rf "$shimdir"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: FAIL (`azl_login_capture` not found).

- [ ] **Step 3: Write minimal implementation**

Append to `azlogin-lib.sh`:

```bash
azl_login_capture() {
  # $1 = tenant. Sets globals: AZL_CAPFILE, AZL_URL, AZL_PORT, AZL_LOGIN_PID.
  local tenant="$1"
  AZL_CAPFILE="$(mktemp)"; : > "$AZL_CAPFILE"
  local capture="${AZLOGIN_CAPTURE:-$HOME/.local/bin/azlogin-capture}"
  AZLOGIN_CAPFILE="$AZL_CAPFILE" BROWSER="$capture %s" \
    az login --tenant "$tenant" --only-show-errors >/dev/null 2>&1 &
  AZL_LOGIN_PID=$!
  local i
  for i in $(seq 1 200); do
    [[ -s "$AZL_CAPFILE" ]] && break
    kill -0 "$AZL_LOGIN_PID" 2>/dev/null || break
    sleep 0.1
  done
  [[ -s "$AZL_CAPFILE" ]] || { printf 'azlogin: failed to capture auth URL\n' >&2; return 1; }
  AZL_URL="$(cat "$AZL_CAPFILE")"
  AZL_PORT="$(azl_extract_port "$AZL_URL")"
  [[ -n "$AZL_PORT" ]] || { printf 'azlogin: could not parse callback port\n' >&2; return 1; }
  return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd ~/.azure-profiles/azlogin
git add azlogin-lib.sh tests/azlogin.bats
git commit -m "feat(azlogin): drive az login and capture callback port"
```

---

## Task 10: Browser bridge — path B with path-A fallback (TDD with ssh shim)

**Files:**
- Modify: `~/.azure-profiles/azlogin/azlogin-lib.sh`
- Test: `~/.azure-profiles/azlogin/tests/azlogin.bats`

- [ ] **Step 1: Write the failing test**

Append to `tests/azlogin.bats`:

```bash
@test "azl_bridge: B path when local reachable (uses reverse tunnel + browser cmd)" {
  shimdir="$(mktemp -d)"; log="$shimdir/ssh.log"
  cat > "$shimdir/ssh" <<EOF
#!/usr/bin/env bash
echo "ssh \$*" >> "$log"
# reachability probe ("ssh ... <host> true") and browser cmd should succeed
exit 0
EOF
  chmod +x "$shimdir/ssh"
  export LOCAL_HOST=velrada-pc-wsl LOCAL_BROWSER_CMD=wslview VM_HOST=vm-always AZL_FORCE_PASTE=0
  PATH="$shimdir:$PATH" run azl_bridge 40404 'https://login/x'
  [ "$status" -eq 0 ]
  grep -q -- "-R 40404:localhost:40404 velrada-pc-wsl" "$log"
  grep -q "wslview" "$log"
  rm -rf "$shimdir"
}

@test "azl_bridge: A path when forced to paste" {
  export LOCAL_HOST=velrada-pc-wsl LOCAL_BROWSER_CMD=wslview VM_HOST=vm-always AZL_FORCE_PASTE=1
  run azl_bridge 40404 'https://login/x'
  [ "$status" -eq 0 ]
  [[ "$output" == *"ssh -fNL 40404:localhost:40404 vm-always && wslview"* ]]
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: FAIL (`azl_bridge` not found).

- [ ] **Step 3: Write minimal implementation**

Append to `azlogin-lib.sh`:

```bash
azl_bridge() {
  # $1=port $2=url. Uses LOCAL_HOST, LOCAL_BROWSER_CMD, VM_HOST, AZL_FORCE_PASTE.
  # Sets AZL_TUNNEL_PID when a reverse tunnel is started (for teardown).
  local port="$1" url="$2"
  if [[ "${AZL_FORCE_PASTE:-0}" != "1" ]] \
     && ssh -o BatchMode=yes -o ConnectTimeout=5 "$LOCAL_HOST" true 2>/dev/null; then
    ssh -N -R "$port:localhost:$port" "$LOCAL_HOST" 2>/dev/null &
    AZL_TUNNEL_PID=$!
    ssh "$LOCAL_HOST" "$LOCAL_BROWSER_CMD '$url'" >/dev/null 2>&1 || true
    printf 'azlogin: browser opened on %s (zero-paste path B)\n' "$LOCAL_HOST"
  else
    printf 'azlogin: local callback unavailable — paste this on your LOCAL machine:\n\n' >&2
    azl_paste_line "$port" "$VM_HOST" "$LOCAL_BROWSER_CMD" "$url" >&2
    printf '\n' >&2
  fi
  return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd ~/.azure-profiles/azlogin
git add azlogin-lib.sh tests/azlogin.bats
git commit -m "feat(azlogin): bridge browser via reverse tunnel with paste fallback"
```

---

## Task 11: Orchestrator script

**Files:**
- Create: `~/.azure-profiles/azlogin/azlogin`

- [ ] **Step 1: Write the orchestrator**

Create `~/.azure-profiles/azlogin/azlogin`:

```bash
#!/usr/bin/env bash
set -euo pipefail

SELF="$(readlink -f "$0")"
LIB="$(dirname "$SELF")/azlogin-lib.sh"
# shellcheck source=/dev/null
source "$LIB"

PROFILE_ARG=""
export AZL_FORCE_PASTE=0
for a in "$@"; do
  case "$a" in
    --paste) AZL_FORCE_PASTE=1 ;;
    -*) printf 'azlogin: unknown flag %s\n' "$a" >&2; exit 2 ;;
    *)  PROFILE_ARG="$a" ;;
  esac
done

# Global config (LOCAL_HOST, LOCAL_BROWSER_CMD, VM_HOST)
GLOBAL_CONF="$HOME/.azure-profiles/azlogin.conf"
[[ -f "$GLOBAL_CONF" ]] || { printf 'azlogin: missing %s (run install.sh)\n' "$GLOBAL_CONF" >&2; exit 1; }
# shellcheck source=/dev/null
source "$GLOBAL_CONF"
: "${LOCAL_HOST:?set in azlogin.conf}" "${LOCAL_BROWSER_CMD:?}" "${VM_HOST:?}"

PROFILE="$(azl_resolve_profile "$PROFILE_ARG" "$PWD")"
export AZURE_CONFIG_DIR="$HOME/.azure-profiles/$PROFILE"
mkdir -p "$AZURE_CONFIG_DIR"
azl_load_profile_conf "$PROFILE"

printf 'azlogin: profile=%s tenant=%s\n' "$PROFILE" "$AZ_TENANT"
azl_clean_slate

cleanup() { kill "${AZL_TUNNEL_PID:-}" 2>/dev/null || true; rm -f "${AZL_CAPFILE:-}"; }
trap cleanup EXIT

azl_login_capture "$AZ_TENANT"
printf 'azlogin: callback port %s\n' "$AZL_PORT"
azl_bridge "$AZL_PORT" "$AZL_URL"

printf 'azlogin: waiting for sign-in to complete...\n'
wait "$AZL_LOGIN_PID"

[[ -n "${AZ_DEFAULT_SUB:-}" ]] && az account set --subscription "$AZ_DEFAULT_SUB" || true
ACCT="$(az account show -o json)"
if azl_assert_account "$ACCT" "$AZ_TENANT" "${AZ_EXPECT_USER:-}"; then
  printf '✓ azlogin: signed in as %s (tenant %s, sub %s)\n' \
    "$(jq -r '.user.name' <<<"$ACCT")" \
    "$(jq -r '.tenantDefaultDomain // .tenantId' <<<"$ACCT")" \
    "$(jq -r '.name' <<<"$ACCT")"
else
  printf '✗ azlogin: signed in but identity does NOT match profile %s — review above.\n' "$PROFILE" >&2
  exit 1
fi
```

```bash
chmod +x ~/.azure-profiles/azlogin/azlogin
```

- [ ] **Step 2: Lint the whole tool**

Run: `shellcheck ~/.azure-profiles/azlogin/azlogin ~/.azure-profiles/azlogin/azlogin-lib.sh ~/.azure-profiles/azlogin/azlogin-capture`
Expected: no errors (warnings acceptable if intentional; fix real issues).

- [ ] **Step 3: Re-run the full unit suite (ensure no regressions)**

Run: `bats ~/.azure-profiles/azlogin/tests/azlogin.bats`
Expected: PASS (all tests).

- [ ] **Step 4: Commit**

```bash
cd ~/.azure-profiles/azlogin
git add azlogin
git commit -m "feat(azlogin): orchestrator wiring profile/clean-slate/login/bridge/assert"
```

---

## Task 12: Install — symlinks, gitignore, config templates and real configs

**Files:**
- Create: `~/.azure-profiles/azlogin/install.sh`
- Create: `~/.azure-profiles/azlogin/azlogin.conf.example`
- Create: `~/.azure-profiles/azlogin/profile.conf.example`
- Modify: `~/.config/git/ignore`
- Create (real, outside repo): `~/.azure-profiles/azlogin.conf`, `~/.azure-profiles/fiig.conf`
- Create: `~/work/fiig-document-reviewer/.azprofile`

- [ ] **Step 1: Write config templates**

Create `~/.azure-profiles/azlogin/azlogin.conf.example`:

```bash
# Global azlogin config. Copy to ~/.azure-profiles/azlogin.conf
LOCAL_HOST=velrada-pc-wsl          # tailnet host that runs the browser (WSL)
LOCAL_BROWSER_CMD=wslview          # opens the Windows default browser from WSL
VM_HOST=vm-always                  # this VM's tailnet name (for the path-A paste line)
```

Create `~/.azure-profiles/azlogin/profile.conf.example`:

```bash
# Per-profile config. Copy to ~/.azure-profiles/<profile>.conf
AZ_TENANT=example.onmicrosoft.com  # tenant domain or GUID (required)
AZ_DEFAULT_SUB=                    # optional: subscription name or id to select after login
AZ_EXPECT_USER=                    # optional: expected signed-in UPN, asserted post-login
```

- [ ] **Step 2: Write install.sh**

Create `~/.azure-profiles/azlogin/install.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
SRC="$(dirname "$(readlink -f "$0")")"
BIN="$HOME/.local/bin"
mkdir -p "$BIN"
ln -sf "$SRC/azlogin"         "$BIN/azlogin"
ln -sf "$SRC/azlogin-capture" "$BIN/azlogin-capture"
echo "linked azlogin + azlogin-capture into $BIN"

# Ensure .azprofile is globally ignored
IGN="${XDG_CONFIG_HOME:-$HOME/.config}/git/ignore"
mkdir -p "$(dirname "$IGN")"
grep -qxF '.azprofile' "$IGN" 2>/dev/null || echo '.azprofile' >> "$IGN"
echo "ensured .azprofile in $IGN"

# Bootstrap global config if absent
[[ -f "$HOME/.azure-profiles/azlogin.conf" ]] || {
  cp "$SRC/azlogin.conf.example" "$HOME/.azure-profiles/azlogin.conf"
  echo "created ~/.azure-profiles/azlogin.conf (review values)"
}
```

```bash
chmod +x ~/.azure-profiles/azlogin/install.sh
```

- [ ] **Step 3: Run the installer**

Run: `~/.azure-profiles/azlogin/install.sh`
Expected output: lines confirming symlinks, the gitignore entry, and the global conf.

- [ ] **Step 4: Verify symlinks resolve and command is found**

Run: `command -v azlogin && readlink -f "$(command -v azlogin)" && ls -l ~/.local/bin/azlogin`
Expected: resolves to `~/.azure-profiles/azlogin/azlogin`.

- [ ] **Step 5: Create the real global + fiig configs and the fiig .azprofile**

```bash
cat > ~/.azure-profiles/azlogin.conf <<'EOF'
LOCAL_HOST=velrada-pc-wsl
LOCAL_BROWSER_CMD=wslview
VM_HOST=vm-always
EOF

cat > ~/.azure-profiles/fiig.conf <<'EOF'
AZ_TENANT=fiig.com.au
AZ_DEFAULT_SUB=
AZ_EXPECT_USER=
EOF

printf 'fiig\n' > ~/work/fiig-document-reviewer/.azprofile
```

(Fill `AZ_DEFAULT_SUB` / `AZ_EXPECT_USER` for fiig once known — see Task 13.)

- [ ] **Step 6: Verify .azprofile is ignored by git in the fiig repo**

Run: `cd ~/work/fiig-document-reviewer && git check-ignore -v .azprofile`
Expected: a line showing it's matched by the global ignore file.

- [ ] **Step 7: Commit (templates + installer only; real confs are gitignored)**

```bash
cd ~/.azure-profiles/azlogin
git add install.sh azlogin.conf.example profile.conf.example
git commit -m "feat(azlogin): installer, config templates, global gitignore for .azprofile"
```

---

## Task 13: End-to-end live login (acceptance)

**Goal:** Prove the whole flow including the live B chain (reverse tunnel → WSL → localhostForwarding → Windows browser).

- [ ] **Step 1: Run azlogin in the fiig repo**

Run:
```bash
cd ~/work/fiig-document-reviewer
azlogin
```
Expected: prints `profile=fiig tenant=fiig.com.au`, a callback port, "browser opened on velrada-pc-wsl (zero-paste path B)", then waits.

- [ ] **Step 2: Complete sign-in**

The Windows default browser should pop to the Microsoft sign-in page. Complete MFA. The page should land on a "you may now close this window" success page.

Expected: back in the terminal, `✓ azlogin: signed in as <you> (tenant fiig.com.au, sub <...>)`.

- [ ] **Step 3: Confirm the session independently**

Run: `AZURE_CONFIG_DIR=~/.azure-profiles/fiig az account show -o table`
Expected: shows the fiig tenant and your account.

- [ ] **Step 4: If B did not pop the browser (localhostForwarding link failed), verify path A**

Run: `cd ~/work/fiig-document-reviewer && azlogin --paste`
Paste the printed line into a local WSL pane; complete sign-in.
Expected: same `✓` success. (If only A works, note in `azlogin-design.md` that B's localhostForwarding link needs investigation — but A is a fully functional fallback.)

- [ ] **Step 5: Backfill fiig.conf and commit docs**

Set `AZ_DEFAULT_SUB` and `AZ_EXPECT_USER` in `~/.azure-profiles/fiig.conf` using values from Step 2/3. Then:

```bash
cd ~/.azure-profiles/azlogin
# Append a "## Live validation" note to azlogin-design.md recording B vs A outcome + WSL2 localhostForwarding result.
git add azlogin-design.md
git commit -m "docs(azlogin): record live E2E validation result"
```

---

## Self-Review

**Spec coverage:**
- Problem 1 (random port / device-code) → Tasks 2, 9. ✓
- Problem 2 (wrong/stale user) → Task 8 (clean slate) + Task 7/11 (assertion). ✓
- Problem 3 (multi-profile) → Tasks 4, 5, 12 (.azprofile + per-profile conf + override arg). ✓
- BROWSER-capture mechanism → Tasks 2, 9. ✓
- Clean slate scoped to profile → Task 8. ✓
- Browser bridge B + A fallback → Task 10; live → Task 13. ✓
- Post-login verification → Tasks 7, 11. ✓
- Files & layout (script, capture, lib, confs, .azprofile, global gitignore) → Tasks 1, 11, 12. ✓
- Out-of-scope items respected (no local helper, no SP/MI, no Windows-native sshd). ✓
- Open items: BROWSER (Task 2), localhostForwarding (Task 13), teardown signal (Task 10/11 `wait`+trap), per-profile values (Tasks 12/13). ✓

**Type/name consistency:** Globals `AZL_PORT/AZL_URL/AZL_LOGIN_PID/AZL_CAPFILE/AZL_TUNNEL_PID`, config vars `AZ_TENANT/AZ_DEFAULT_SUB/AZ_EXPECT_USER`, global conf `LOCAL_HOST/LOCAL_BROWSER_CMD/VM_HOST`, and functions `azl_extract_port/azl_resolve_profile/azl_load_profile_conf/azl_paste_line/azl_assert_account/azl_clean_slate/azl_login_capture/azl_bridge` are used identically across Tasks 3–13. ✓

**Placeholder scan:** No TBD/TODO in steps; every code step shows complete code; the only deferred *values* (fiig sub/user) are explicitly backfilled in Task 13. ✓
