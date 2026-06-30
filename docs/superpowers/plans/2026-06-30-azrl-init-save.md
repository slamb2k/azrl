# azrl `--init` / `--save` + tenant-less login — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `azrl --init` (tenant-less bootstrap that logs in, records the tenant, and wires the directory) and `azrl --save` (record the current session), rename `--derive`→`--save`, and make bare `azrl` fall back to a tenant-less sign-in when no profile resolves.

**Architecture:** New logic lives as pure/testable functions in `azrl-lib.sh` (`azrl_sanitize_name`, `azrl_default_name`, `azrl_write_profile`, plus an optional-tenant tweak to `azrl_login_capture`); the `azrl` orchestrator gains `--init`/`--save` dispatch and a bare-fallback branch. TDD with PATH-shimmed `az`/`ssh` per the existing suite.

**Tech Stack:** Bash (`set -euo pipefail`), bats, jq, shellcheck.

## Global Constraints

- All scripts use `set -euo pipefail`.
- `azrl-lib.sh` must have **no side effects on source**.
- New behaviour goes in `azrl-lib.sh` as testable functions, called from `azrl`.
- Cross-file: lib functions set `AZRL_*` globals; long-lived background PIDs must be exported as `AZRL_*` for the cleanup trap.
- Lint clean: `shellcheck azrl azrl-lib.sh azrl-capture`.
- Conventional commits with scope, e.g. `feat(azrl): ...`.
- Run `bats tests/azrl.bats` after every task; it must stay green.

---

### Task 1: Pure name helpers (`azrl_sanitize_name`, `azrl_default_name`)

**Files:**
- Modify: `azrl-lib.sh`
- Test: `tests/azrl.bats`

**Interfaces:**
- Produces:
  - `azrl_sanitize_name "<raw>"` → prints sanitized name (lowercase; runs of chars outside `[a-z0-9._-]` → single `-`; leading/trailing `-` stripped).
  - `azrl_default_name "<arg>" "<dir>"` → prints `<arg>` verbatim if non-empty, else `azrl_sanitize_name "$(basename "<dir>")"`.

- [ ] **Step 1: Write the failing tests**

Add to `tests/azrl.bats`:

```bash
@test "azrl_sanitize_name: lowercases and dashes spaces" {
  run azrl_sanitize_name "Contoso Migration"
  [ "$status" -eq 0 ]
  [ "$output" = "contoso-migration" ]
}

@test "azrl_sanitize_name: collapses junk runs and trims edges" {
  run azrl_sanitize_name "  --Foo__Bar!!  "
  [ "$output" = "foo__bar" ]
}

@test "azrl_default_name: explicit arg used verbatim" {
  run azrl_default_name "My Profile" "/home/x/whatever"
  [ "$output" = "My Profile" ]
}

@test "azrl_default_name: empty arg falls back to sanitized basename" {
  run azrl_default_name "" "/home/x/Contoso Migration"
  [ "$output" = "contoso-migration" ]
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `bats tests/azrl.bats -f "azrl_sanitize_name|azrl_default_name"`
Expected: FAIL — `azrl_sanitize_name: command not found`.

- [ ] **Step 3: Implement the helpers**

Add to `azrl-lib.sh` (after `azrl_resolve_profile`):

```bash
azrl_sanitize_name() {
  # $1=raw. lowercase; non [a-z0-9._-] runs -> '-'; strip leading/trailing '-'.
  local s="${1,,}"
  s="$(printf '%s' "$s" | sed -E 's/[^a-z0-9._-]+/-/g; s/^-+//; s/-+$//')"
  printf '%s\n' "$s"
}

azrl_default_name() {
  # $1=arg $2=dir. Explicit arg verbatim; else sanitized basename of dir.
  local arg="$1" dir="${2:-$PWD}"
  if [[ -n "$arg" ]]; then printf '%s\n' "$arg"; return 0; fi
  azrl_sanitize_name "$(basename "$dir")"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `bats tests/azrl.bats -f "azrl_sanitize_name|azrl_default_name"`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add azrl-lib.sh tests/azrl.bats
git commit -m "feat(azrl): add azrl_sanitize_name and azrl_default_name helpers"
```

---

### Task 2: Rename `azrl_derive_conf` → `azrl_save_conf`

**Files:**
- Modify: `azrl-lib.sh` (function def + `azrl_usage` text)
- Modify: `azrl:38` (call site)
- Test: `tests/azrl.bats` (rename the two `azrl_derive_conf` unit tests)

**Interfaces:**
- Produces: `azrl_save_conf "<account_json>" "<domains_json>"` — identical behaviour to the old `azrl_derive_conf` (emits a `<profile>.conf` body to stdout).

- [ ] **Step 1: Rename the function definition**

In `azrl-lib.sh`, change the definition header only:

```bash
azrl_save_conf() {
```

(Leave the body of the function exactly as-is.)

- [ ] **Step 2: Update the orchestrator call site**

In `azrl`, line ~38, change:

```bash
  azrl_save_conf "$ACCT" "$DOMS" > "$OUT"
```

- [ ] **Step 3: Update the two unit tests**

In `tests/azrl.bats`, rename both test titles and invocations:
- `@test "azrl_derive_conf: builds conf ..."` → `@test "azrl_save_conf: builds conf ..."`, and `run azrl_derive_conf` → `run azrl_save_conf`.
- `@test "azrl_derive_conf: falls back ..."` → `@test "azrl_save_conf: falls back ..."`, and `run azrl_derive_conf` → `run azrl_save_conf`.

- [ ] **Step 4: Run the renamed tests**

Run: `bats tests/azrl.bats -f "azrl_save_conf"`
Expected: PASS (2 tests).

- [ ] **Step 5: Run the full suite (the `azrl --derive` integration tests still call the flag, which still exists)**

Run: `bats tests/azrl.bats`
Expected: PASS (all). The `--derive` flag is removed in Task 5.

- [ ] **Step 6: Commit**

```bash
git add azrl azrl-lib.sh tests/azrl.bats
git commit -m "refactor(azrl): rename azrl_derive_conf to azrl_save_conf"
```

---

### Task 3: Optional tenant in `azrl_login_capture`

**Files:**
- Modify: `azrl-lib.sh` (`azrl_login_capture`)
- Test: `tests/azrl.bats`

**Interfaces:**
- Consumes/Produces: `azrl_login_capture "<tenant>"` — when `<tenant>` is empty, `az login` is invoked **without** `--tenant`; otherwise unchanged. Still sets `AZRL_CAPFILE`, `AZRL_URL`, `AZRL_PORT`, `AZRL_LOGIN_PID`.

- [ ] **Step 1: Write the failing test**

Add to `tests/azrl.bats`:

```bash
@test "azrl_login_capture: omits --tenant when tenant is empty" {
  shimdir="$(mktemp -d)"; log="$shimdir/az.log"
  cat > "$shimdir/az" <<EOF
#!/usr/bin/env bash
echo "\$*" >> "$log"
url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F&state=z'
cmd="\${BROWSER/\\%s/\$url}"
eval "\$cmd"
sleep 2
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export AZRL_CAPTURE='${BATS_TEST_DIRNAME}/../azrl-capture'
    PATH='$shimdir':\$PATH azrl_login_capture ''
    echo \"PORT=\$AZRL_PORT\"
    kill \$AZRL_LOGIN_PID 2>/dev/null || true
  "
  [ "$status" -eq 0 ]
  [[ "$output" == *"PORT=40404"* ]]
  grep -q 'login' "$log"
  ! grep -q -- '--tenant' "$log"
  rm -rf "$shimdir"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats tests/azrl.bats -f "omits --tenant"`
Expected: FAIL — `az login` is still called with `--tenant ` (empty), so `--tenant` appears in the log.

- [ ] **Step 3: Implement optional tenant**

In `azrl-lib.sh`, inside `azrl_login_capture`, replace the `az login` invocation block. Find:

```bash
  AZRL_CAPFILE="$AZRL_CAPFILE" BROWSER="$capture %s" \
    az login --tenant "$tenant" --only-show-errors >/dev/null 2>&1 &
```

Replace with:

```bash
  local -a tenant_args=()
  [[ -n "$tenant" ]] && tenant_args=(--tenant "$tenant")
  AZRL_CAPFILE="$AZRL_CAPFILE" BROWSER="$capture %s" \
    az login ${tenant_args[@]+"${tenant_args[@]}"} --only-show-errors >/dev/null 2>&1 &
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `bats tests/azrl.bats -f "azrl_login_capture"`
Expected: PASS (both the existing with-tenant test and the new tenant-less test).

- [ ] **Step 5: Lint**

Run: `shellcheck azrl-lib.sh`
Expected: no output (clean).

- [ ] **Step 6: Commit**

```bash
git add azrl-lib.sh tests/azrl.bats
git commit -m "feat(azrl): omit --tenant in azrl_login_capture when tenant is empty"
```

---

### Task 4: `azrl_write_profile` (record session → conf + .azprofile)

**Files:**
- Modify: `azrl-lib.sh`
- Test: `tests/azrl.bats`

**Interfaces:**
- Consumes: `azrl_save_conf` (Task 2); reads `AZURE_CONFIG_DIR` set by the caller.
- Produces: `azrl_write_profile "<profile>" "<target_dir>"`:
  - Refuses to clobber an existing `$HOME/.azure-profiles/<profile>.conf` → message + return 1.
  - Else reads `az account show`; if not logged in → message + return 1.
  - Writes the conf via `azrl_save_conf`, writes `<profile>` into `<target_dir>/.azprofile`, returns 0.

- [ ] **Step 1: Write the failing tests**

Add to `tests/azrl.bats`:

```bash
@test "azrl_write_profile: writes conf and .azprofile from session" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/acme"
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *"account show"*)   echo '{"tenantId":"guid-9","id":"sub-1","name":"Sub","user":{"name":"u@acme.onmicrosoft.com"}}' ;;
  *"rest"*"domains"*) echo '{"value":[{"id":"acme.onmicrosoft.com","isDefault":true}]}' ;;
  *)                  echo '{}' ;;
esac
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export HOME='$home' AZURE_CONFIG_DIR='$home/.azure-profiles/acme'
    PATH='$shimdir':\$PATH azrl_write_profile acme '$work'
  "
  [ "$status" -eq 0 ]
  grep -q 'AZ_TENANT=acme.onmicrosoft.com' "$home/.azure-profiles/acme.conf"
  grep -q 'AZ_TENANT_ID=guid-9' "$home/.azure-profiles/acme.conf"
  [ "$(cat "$work/.azprofile")" = "acme" ]
  rm -rf "$home" "$shimdir" "$work"
}

@test "azrl_write_profile: fails clearly when not logged in" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export HOME='$home' AZURE_CONFIG_DIR='$home/.azure-profiles/acme'
    PATH='$shimdir':\$PATH azrl_write_profile acme '$work'
  "
  [ "$status" -ne 0 ]
  [[ "$output" == *"not logged in"* ]]
  [ ! -e "$home/.azure-profiles/acme.conf" ]
  [ ! -e "$work/.azprofile" ]
  rm -rf "$home" "$shimdir" "$work"
}

@test "azrl_write_profile: refuses to clobber existing conf" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles"
  printf 'AZ_TENANT=keep.me\n' > "$home/.azure-profiles/acme.conf"
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
echo '{}'
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export HOME='$home' AZURE_CONFIG_DIR='$home/.azure-profiles/acme'
    PATH='$shimdir':\$PATH azrl_write_profile acme '$work'
  "
  [ "$status" -ne 0 ]
  grep -q 'AZ_TENANT=keep.me' "$home/.azure-profiles/acme.conf"
  [ ! -e "$work/.azprofile" ]
  rm -rf "$home" "$shimdir" "$work"
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `bats tests/azrl.bats -f "azrl_write_profile"`
Expected: FAIL — `azrl_write_profile: command not found`.

- [ ] **Step 3: Implement `azrl_write_profile`**

Add to `azrl-lib.sh` (after `azrl_save_conf`):

```bash
azrl_write_profile() {
  # $1=profile $2=target_dir. Uses AZURE_CONFIG_DIR (set by caller).
  # Writes ~/.azure-profiles/<profile>.conf from the current session (refusing to
  # clobber) and <target_dir>/.azprofile. Returns 1 on existing conf or no session.
  local profile="$1" dir="$2"
  local out="$HOME/.azure-profiles/$profile.conf"
  [[ -e "$out" ]] && { printf 'azrl: %s already exists — remove it first to re-save\n' "$out" >&2; return 1; }
  local acct doms
  acct="$(az account show -o json 2>/dev/null)" \
    || { printf 'azrl: not logged in for %q — run azrl --init first\n' "$profile" >&2; return 1; }
  doms="$(az rest --url 'https://graph.microsoft.com/v1.0/domains' -o json 2>/dev/null || printf '{}')"
  azrl_save_conf "$acct" "$doms" > "$out"
  printf '%s\n' "$profile" > "$dir/.azprofile"
  printf 'azrl: wrote %s and %s/.azprofile\n' "$out" "$dir"
  return 0
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `bats tests/azrl.bats -f "azrl_write_profile"`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add azrl-lib.sh tests/azrl.bats
git commit -m "feat(azrl): add azrl_write_profile to record session as conf + .azprofile"
```

---

### Task 5: Orchestrator `--save | -s` mode (replaces `--derive`)

**Files:**
- Modify: `azrl` (arg parse + replace derive block with save block)
- Modify: `azrl-lib.sh` (`azrl_usage` text)
- Test: `tests/azrl.bats` (convert the two `azrl --derive` integration tests to `--save`)

**Interfaces:**
- Consumes: `azrl_default_name` (Task 1), `azrl_write_profile` (Task 4).
- Produces: `azrl --save [name]` / `azrl -s [name]` — resolves name via `azrl_default_name`, sets `AZURE_CONFIG_DIR`, calls `azrl_write_profile`; needs no global `azrl.conf`.

- [ ] **Step 1: Update arg parsing and replace the derive block**

In `azrl`, in the `for a in "$@"` loop, replace the `--derive` case:

```bash
    --save|-s) AZRL_MODE=save ;;
```

Then replace the entire `if [[ "$AZRL_MODE" == "derive" ]]; then ... fi` block (lines ~26-41) with:

```bash
# --save: record the current session as <name>.conf and wire $PWD/.azprofile.
# name = arg, else sanitized basename of $PWD. Refuses to clobber. No login.
if [[ "$AZRL_MODE" == "save" ]]; then
  PROFILE="$(azrl_default_name "$PROFILE_ARG" "$PWD")"
  export AZURE_CONFIG_DIR="$HOME/.azure-profiles/$PROFILE"
  azrl_write_profile "$PROFILE" "$PWD" || exit 1
  exit 0
fi
```

- [ ] **Step 2: Update the usage text**

In `azrl-lib.sh` `azrl_usage`, replace the `azrl --derive [profile]` synopsis line and the `--derive` option block with:

```
  azrl --save [name]
```

and under Options:

```
  --save, -s       Record the current session as <name>.conf (name defaults to
                   the sanitized current directory) and write .azprofile in $PWD.
```

- [ ] **Step 3: Convert the two `--derive` integration tests to `--save`**

In `tests/azrl.bats`:
- `@test "azrl --derive: writes a profile conf from the logged-in session"` → title `azrl --save: writes a profile conf and .azprofile`; change the invocation `--derive nrg` → `--save nrg`; and add an assertion that `.azprofile` is written. The test runs from the repo dir, so assert against a temp `work` dir by `cd`-ing. Replace the invocation line with:

```bash
  work="$(mktemp -d)"
  HOME="$home" PATH="$shimdir:$PATH" run bash -c "cd '$work' && '${BATS_TEST_DIRNAME}/../azrl' --save nrg"
  [ "$status" -eq 0 ]
  [ -f "$home/.azure-profiles/nrg.conf" ]
  grep -q 'AZ_TENANT=onenrg.onmicrosoft.com' "$home/.azure-profiles/nrg.conf"
  grep -q 'AZ_TENANT_ID=guid-1' "$home/.azure-profiles/nrg.conf"
  grep -q 'AZ_EXPECT_USER=u@onenrg.onmicrosoft.com' "$home/.azure-profiles/nrg.conf"
  [ "$(cat "$work/.azprofile")" = "nrg" ]
  rm -rf "$home" "$shimdir" "$work"
```

- `@test "azrl --derive: refuses to clobber an existing conf"` → title `azrl --save: refuses to clobber an existing conf`; change `--derive nrg` → `--save nrg`. Keep the rest.

- [ ] **Step 4: Run the suite**

Run: `bats tests/azrl.bats`
Expected: PASS (all). No remaining references to `--derive`.

- [ ] **Step 5: Lint**

Run: `shellcheck azrl azrl-lib.sh`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add azrl azrl-lib.sh tests/azrl.bats
git commit -m "feat(azrl): replace --derive with --save (records conf + .azprofile)"
```

---

### Task 6: Bare `azrl` tenant-less fallback (restructure main login path)

**Files:**
- Modify: `azrl` (hoist `cleanup` trap; branch profile vs no-profile)
- Test: `tests/azrl.bats`

**Interfaces:**
- Consumes: `azrl_resolve_profile` (returns 1 when nothing resolves), `azrl_login_capture ""` (Task 3).
- Produces: bare `azrl` with no arg and no `.azprofile` → tenant-less sign-in into the default `~/.azure` (no `AZURE_CONFIG_DIR` override, no files written), still using the bridge (needs global `azrl.conf`).

- [ ] **Step 1: Write the failing test**

Add to `tests/azrl.bats`:

```bash
@test "azrl: no profile -> tenant-less sign-in into default config" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"; log="$shimdir/az.log"
  mkdir -p "$home/.azure-profiles"
  cat > "$home/.azure-profiles/azrl.conf" <<'EOF'
LOCAL_HOST=localhost
LOCAL_BROWSER_CMD=true
VM_HOST=vm
EOF
  cat > "$shimdir/az" <<EOF
#!/usr/bin/env bash
echo "\$*" >> "$log"
case "\$*" in
  *"login"*)
    url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F'
    cmd="\${BROWSER/\\%s/\$url}"; eval "\$cmd"; exit 0 ;;
  *"account show"*) echo '{"tenantId":"g","tenantDefaultDomain":"d","name":"s","user":{"name":"u@x"}}' ;;
  *) echo '{}' ;;
esac
EOF
  chmod +x "$shimdir/az"
  # ssh shim: reachability + reverse tunnel both succeed quickly.
  cat > "$shimdir/ssh" <<'EOF'
#!/usr/bin/env bash
for a in "$@"; do [[ "$a" == "-R" ]] && { sleep 1; exit 0; }; done
exit 0
EOF
  chmod +x "$shimdir/ssh"
  HOME="$home" PATH="$shimdir:$PATH" AZRL_CAPTURE="${BATS_TEST_DIRNAME}/../azrl-capture" \
    run bash -c "cd '$work' && '${BATS_TEST_DIRNAME}/../azrl'"
  [ "$status" -eq 0 ]
  grep -q 'login' "$log"
  ! grep -q -- '--tenant' "$log"
  [ ! -e "$work/.azprofile" ]
  rm -rf "$home" "$shimdir" "$work"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats tests/azrl.bats -f "tenant-less sign-in into default"`
Expected: FAIL — bare `azrl` currently errors with "no profile arg and no .azprofile found".

- [ ] **Step 3: Hoist the cleanup trap**

In `azrl`, remove the existing `cleanup()` definition + `trap cleanup EXIT` (currently lines ~62-66). Add this single definition immediately **after** the `--save` block (before the global-config load):

```bash
cleanup() {
  kill "${AZRL_TUNNEL_PID:-}" "${AZRL_WATCHDOG_PID:-}" "${AZRL_LOGIN_PID:-}" 2>/dev/null || true
  rm -f "${AZRL_CAPFILE:-}"
}
trap cleanup EXIT
```

- [ ] **Step 4: Restructure the main login path**

In `azrl`, replace the block that currently runs from the `PROFILE="$(azrl_resolve_profile ...)"` line through the final identity-assertion `fi` with:

```bash
if PROFILE="$(azrl_resolve_profile "$PROFILE_ARG" "$PWD" 2>/dev/null)"; then
  azrl_load_profile_conf "$PROFILE"
  export AZURE_CONFIG_DIR="$HOME/.azure-profiles/$PROFILE"
  mkdir -p "$AZURE_CONFIG_DIR"
  TENANT="$AZ_TENANT"
  printf 'azrl: profile=%s tenant=%s\n' "$PROFILE" "$AZ_TENANT"
  azrl_clean_slate
else
  PROFILE=""
  TENANT=""
  printf 'azrl: no profile resolved — tenant-less sign-in into default ~/.azure\n'
fi

azrl_login_capture "$TENANT"
printf 'azrl: callback port %s\n' "$AZRL_PORT"
azrl_bridge "$AZRL_PORT" "$AZRL_URL"

printf 'azrl: waiting for sign-in to complete...\n'
login_rc=0
azrl_wait_for_login "$AZRL_LOGIN_PID" "$AZRL_LOGIN_TIMEOUT" \
  "$AZRL_PORT" "$VM_HOST" "$LOCAL_BROWSER_CMD" "$AZRL_URL" || login_rc=$?
(( login_rc == 0 )) || exit "$login_rc"

if [[ -z "$PROFILE" ]]; then
  ACCT="$(az account show -o json)"
  printf '✓ azrl: signed in as %s (tenant %s, sub %s)\n' \
    "$(jq -r '.user.name' <<<"$ACCT")" \
    "$(jq -r '.tenantDefaultDomain // .tenantId' <<<"$ACCT")" \
    "$(jq -r '.name' <<<"$ACCT")"
  exit 0
fi

if [[ -n "${AZ_DEFAULT_SUB:-}" ]]; then
  az account set --subscription "$AZ_DEFAULT_SUB" \
    || { printf '✗ azrl: could not select subscription %q\n' "$AZ_DEFAULT_SUB" >&2; exit 1; }
fi
ACCT="$(az account show -o json)"
if azrl_assert_account "$ACCT" "${AZ_TENANT_ID:-$AZ_TENANT}" "${AZ_EXPECT_USER:-}"; then
  printf '✓ azrl: signed in as %s (tenant %s, sub %s)\n' \
    "$(jq -r '.user.name' <<<"$ACCT")" \
    "$(jq -r '.tenantDefaultDomain // .tenantId' <<<"$ACCT")" \
    "$(jq -r '.name' <<<"$ACCT")"
else
  printf '✗ azrl: signed in but identity does NOT match profile %s — review above.\n' "$PROFILE" >&2
  exit 1
fi
```

- [ ] **Step 5: Run the suite**

Run: `bats tests/azrl.bats`
Expected: PASS (all, including the new bare-fallback test and the existing missing-`<profile>.conf` test).

- [ ] **Step 6: Lint**

Run: `shellcheck azrl`
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add azrl tests/azrl.bats
git commit -m "feat(azrl): bare azrl falls back to tenant-less sign-in when no profile resolves"
```

---

### Task 7: Orchestrator `--init | -i` mode

**Files:**
- Modify: `azrl` (arg parse + init block)
- Modify: `azrl-lib.sh` (`azrl_usage` text)
- Test: `tests/azrl.bats`

**Interfaces:**
- Consumes: `azrl_default_name`, `azrl_clean_slate`, `azrl_login_capture ""`, `azrl_bridge`, `azrl_wait_for_login`, `azrl_write_profile`, hoisted `cleanup` trap.
- Produces: `azrl --init [name]` / `azrl -i [name]` — tenant-less login into the profile's isolated `AZURE_CONFIG_DIR`, then writes conf + `.azprofile`.

- [ ] **Step 1: Write the failing test**

Add to `tests/azrl.bats`:

```bash
@test "azrl --init: tenant-less login then writes conf and .azprofile" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"; log="$shimdir/az.log"
  mkdir -p "$home/.azure-profiles"
  cat > "$home/.azure-profiles/azrl.conf" <<'EOF'
LOCAL_HOST=localhost
LOCAL_BROWSER_CMD=true
VM_HOST=vm
EOF
  cat > "$shimdir/az" <<EOF
#!/usr/bin/env bash
echo "\$*" >> "$log"
case "\$*" in
  *"login"*)
    url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F'
    cmd="\${BROWSER/\\%s/\$url}"; eval "\$cmd"; exit 0 ;;
  *"account show"*)   echo '{"tenantId":"guid-7","id":"sub-7","name":"Sub","user":{"name":"u@boot.onmicrosoft.com"}}' ;;
  *"rest"*"domains"*) echo '{"value":[{"id":"boot.onmicrosoft.com","isDefault":true}]}' ;;
  *) echo '{}' ;;
esac
EOF
  chmod +x "$shimdir/az"
  cat > "$shimdir/ssh" <<'EOF'
#!/usr/bin/env bash
for a in "$@"; do [[ "$a" == "-R" ]] && { sleep 1; exit 0; }; done
exit 0
EOF
  chmod +x "$shimdir/ssh"
  HOME="$home" PATH="$shimdir:$PATH" AZRL_CAPTURE="${BATS_TEST_DIRNAME}/../azrl-capture" \
    run bash -c "cd '$work' && '${BATS_TEST_DIRNAME}/../azrl' --init boot"
  [ "$status" -eq 0 ]
  ! grep -q -- '--tenant' "$log"
  grep -q 'AZ_TENANT=boot.onmicrosoft.com' "$home/.azure-profiles/boot.conf"
  grep -q 'AZ_TENANT_ID=guid-7' "$home/.azure-profiles/boot.conf"
  [ "$(cat "$work/.azprofile")" = "boot" ]
  rm -rf "$home" "$shimdir" "$work"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bats tests/azrl.bats -f "azrl --init"`
Expected: FAIL — `--init` is an unknown flag (exit 2).

- [ ] **Step 3: Add the `--init` arg case**

In `azrl`, in the `for a in "$@"` loop, add alongside the `--save` case:

```bash
    --init|-i) AZRL_MODE=init ;;
```

- [ ] **Step 4: Add the init block**

In `azrl`, immediately after the `--save` block (and before the hoisted `cleanup`/`trap`), add:

```bash
# --init: tenant-less sign-in into the profile's isolated config, then record it.
if [[ "$AZRL_MODE" == "init" ]]; then
  GLOBAL_CONF="$HOME/.azure-profiles/azrl.conf"
  [[ -f "$GLOBAL_CONF" ]] || { printf 'azrl: missing %s (run install.sh)\n' "$GLOBAL_CONF" >&2; exit 1; }
  # shellcheck source=/dev/null
  source "$GLOBAL_CONF"
  : "${LOCAL_HOST:?set in azrl.conf}" "${LOCAL_BROWSER_CMD:?}" "${VM_HOST:?}"
  AZRL_LOGIN_TIMEOUT="${AZRL_LOGIN_TIMEOUT:-180}"

  PROFILE="$(azrl_default_name "$PROFILE_ARG" "$PWD")"
  OUT="$HOME/.azure-profiles/$PROFILE.conf"
  [[ -e "$OUT" ]] && { printf 'azrl: %s already exists — remove it first to re-init\n' "$OUT" >&2; exit 1; }
  export AZURE_CONFIG_DIR="$HOME/.azure-profiles/$PROFILE"
  mkdir -p "$AZURE_CONFIG_DIR"
  printf 'azrl: init profile=%s (tenant-less sign-in)\n' "$PROFILE"
  azrl_clean_slate

  azrl_login_capture ""
  printf 'azrl: callback port %s\n' "$AZRL_PORT"
  azrl_bridge "$AZRL_PORT" "$AZRL_URL"
  printf 'azrl: waiting for sign-in to complete...\n'
  login_rc=0
  azrl_wait_for_login "$AZRL_LOGIN_PID" "$AZRL_LOGIN_TIMEOUT" \
    "$AZRL_PORT" "$VM_HOST" "$LOCAL_BROWSER_CMD" "$AZRL_URL" || login_rc=$?
  (( login_rc == 0 )) || exit "$login_rc"

  azrl_write_profile "$PROFILE" "$PWD" || exit 1
  exit 0
fi
```

Note: this block relies on the `cleanup`/`trap` that follows it in the file. Since Task 6 hoisted `cleanup`/`trap` to *after* the `--save` block, move the `--init` block to sit **between** the `--save` block and the `cleanup` definition so the trap is armed before `azrl_login_capture` spawns the background login. (If ordering makes that awkward, duplicate the four-line `cleanup` + `trap` at the top of the init block instead — both are acceptable; prefer the shared one.)

- [ ] **Step 5: Update the usage text**

In `azrl-lib.sh` `azrl_usage`, add to the synopsis:

```
  azrl --init [name]
```

and under Options:

```
  --init, -i       Tenant-less `az login`, then record the session as
                   <name>.conf (default: sanitized current directory) and
                   write .azprofile in $PWD.
```

- [ ] **Step 6: Run the suite**

Run: `bats tests/azrl.bats`
Expected: PASS (all).

- [ ] **Step 7: Lint**

Run: `shellcheck azrl azrl-lib.sh`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add azrl azrl-lib.sh tests/azrl.bats
git commit -m "feat(azrl): add --init for zero-config tenant-less bootstrap"
```

---

### Task 8: Update README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace `--derive` references and document the new flow**

In `README.md`:
- Replace the synopsis line `azrl --derive [profile]    # generate <profile>.conf from a logged-in session` with:

```
azrl --init [name]         # tenant-less login, then record conf + .azprofile
azrl --save [name]         # record current session as conf + .azprofile
```

- Replace the `azrl --derive <profile>    # writes ~/.azure-profiles/<profile>.conf` example and the paragraph beginning `` `--derive` reads the live session's... `` with:

```
azrl --save <name>         # writes ~/.azure-profiles/<name>.conf

`--save` reads the live session's tenant GUID, subscription, and user, and
writes them to `<name>.conf` plus a `.azprofile` in the current directory.
`--init` does the same but signs you in first (tenant-less). The name
defaults to the sanitized current directory when omitted.
```

- [ ] **Step 2: Verify no stale references remain**

Run: `grep -rn "derive" README.md`
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs(azrl): document --init and --save, drop --derive"
```

---

## Self-Review

**Spec coverage:**
- Rename `--derive`→`--save` everywhere → Tasks 2 (fn + call site + unit tests), 5 (flag + integration tests + usage), 8 (README). ✅
- Tenant-less login plumbing → Task 3. ✅
- `azrl --save [name]` (name resolution, fail-when-not-logged-in, refuse clobber, writes conf + .azprofile) → Tasks 4 + 5. ✅
- `azrl --init [name]` (tenant-less login → conf + .azprofile, refuse clobber) → Task 7. ✅
- Bare `azrl` tenant-less fallback → Task 6. ✅
- `azrl_sanitize_name`, default = sanitized `basename $PWD`, explicit arg verbatim → Task 1. ✅

**Placeholder scan:** No TBD/TODO/"handle edge cases"; every code step shows complete code. ✅

**Type/name consistency:** `azrl_save_conf`, `azrl_sanitize_name`, `azrl_default_name`, `azrl_write_profile`, `azrl_login_capture ""`, `AZRL_MODE` values `save`/`init`, flags `--save|-s`/`--init|-i` used consistently across tasks. ✅

**Ordering note:** Task 5 removes the `--derive` flag, so the `--derive` integration tests are converted in the same task (Step 3) to keep the suite green. Task 6 hoists `cleanup` before Task 7 places the `--init` block adjacent to it.
