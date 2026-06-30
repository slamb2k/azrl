# azrl `--allow-no-subscription` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `azrl` sign in to tenants with no Azure subscription by always passing `--allow-no-subscription` to `az login`.

**Architecture:** One unconditional flag added to the `az login` invocation in `azrl_login_capture` (`azrl-lib.sh`); no orchestrator changes (sub-select is already gated, assertion checks tenant/user only). TDD with PATH-shimmed `az` per the existing suite.

**Tech Stack:** Bash (`set -euo pipefail`), bats, shellcheck.

## Global Constraints

- All scripts use `set -euo pipefail`.
- `azrl-lib.sh` must have **no side effects on source**.
- Lint clean: `shellcheck azrl azrl-lib.sh azrl-capture`.
- Conventional commits with scope, e.g. `feat(azrl): ...`.
- Run `bats tests/azrl.bats` before committing; it must stay green.
- bats test rule: a bare `! grep`/`! cmd` assertion is INERT (bash exempts `!` from `set -e`; usually not the last line) — it asserts nothing. Use `run grep ...; [ "$status" -eq 0 ]` (present) or `[ "$status" -ne 0 ]` (absent). Never a bare `! cmd`.

---

### Task 1: Always pass `--allow-no-subscription` to `az login`

**Files:**
- Modify: `azrl-lib.sh` (the `az login` invocation in `azrl_login_capture`, currently the line `az login ${tenant_args[@]+"${tenant_args[@]}"} --only-show-errors >/dev/null 2>&1 &`)
- Modify: `tests/azrl.bats` (extend the tenant-less test; add a with-tenant argv test)
- Modify: `README.md` (one-line note)

**Interfaces:**
- Consumes: existing `azrl_login_capture "<tenant>"` (empty tenant ⇒ no `--tenant`).
- Produces: every `az login` azrl spawns includes `--allow-no-subscription`, on both the with-tenant and tenant-less paths.

- [ ] **Step 1: Write the failing tests**

In `tests/azrl.bats`, in the existing test `@test "azrl_login_capture: omits --tenant when tenant is empty"`, add an allow-no-subscription assertion immediately before the final `rm -rf "$shimdir"` line. The block currently ends:

```bash
  run grep -q -- '--tenant' "$log"
  [ "$status" -ne 0 ]
  rm -rf "$shimdir"
}
```

Change it to:

```bash
  run grep -q -- '--tenant' "$log"
  [ "$status" -ne 0 ]
  run grep -q -- '--allow-no-subscription' "$log"
  [ "$status" -eq 0 ]
  rm -rf "$shimdir"
}
```

Then add a new test (place it directly after that test's closing `}`) that exercises the with-tenant path and asserts the flag coexists with `--tenant`:

```bash
@test "azrl_login_capture: passes --allow-no-subscription (with tenant)" {
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
    PATH='$shimdir':\$PATH azrl_login_capture fiig.com.au
    echo \"PORT=\$AZRL_PORT\"
    kill \$AZRL_LOGIN_PID 2>/dev/null || true
  "
  [ "$status" -eq 0 ]
  [[ "$output" == *"PORT=40404"* ]]
  run grep -q -- '--tenant fiig.com.au' "$log"
  [ "$status" -eq 0 ]
  run grep -q -- '--allow-no-subscription' "$log"
  [ "$status" -eq 0 ]
  rm -rf "$shimdir"
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `bats tests/azrl.bats -f "azrl_login_capture"`
Expected: the two `--allow-no-subscription` assertions FAIL (the flag is not yet in the `az login` invocation), reported as `not ok` for both the (amended) tenant-less test and the new with-tenant test. The `PORT=40404` / `--tenant` assertions still pass.

- [ ] **Step 3: Add the flag**

In `azrl-lib.sh`, find the `az login` invocation in `azrl_login_capture`:

```bash
    az login ${tenant_args[@]+"${tenant_args[@]}"} --only-show-errors >/dev/null 2>&1 &
```

Replace it with:

```bash
    az login ${tenant_args[@]+"${tenant_args[@]}"} --allow-no-subscription --only-show-errors >/dev/null 2>&1 &
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `bats tests/azrl.bats -f "azrl_login_capture"`
Expected: PASS — all `azrl_login_capture` tests green (the original capture test, the tenant-less test with its new assertion, and the new with-tenant test).

- [ ] **Step 5: README note**

In `README.md`, under the "Usage" or "Configuration" prose, add one line:

```
azrl always signs in with `--allow-no-subscription`, so it works with tenants
that have no Azure subscription (Entra-ID-only / tenant-level accounts).
```

- [ ] **Step 6: Full suite + lint**

Run: `bats tests/azrl.bats`
Expected: all tests pass.
Run: `shellcheck azrl azrl-lib.sh azrl-capture`
Expected: clean (no output).

- [ ] **Step 7: Commit**

```bash
git add azrl-lib.sh tests/azrl.bats README.md
git commit -m "feat(azrl): always pass --allow-no-subscription to az login"
```

---

## Self-Review

**Spec coverage:** Always-on `--allow-no-subscription` (Step 3) ✅; both paths tested (Step 1, tenant-less + with-tenant) ✅; README note (Step 5) ✅; no orchestrator change needed (design rationale: sub-select already gated, assert checks tenant/user only) ✅.

**Placeholder scan:** No TBD/TODO; every code step shows complete code. ✅

**Type/name consistency:** Flag string `--allow-no-subscription` and function `azrl_login_capture` used consistently; assertions use the effective `run grep …; [ "$status" -eq/-ne 0 ]` form, never bare `! grep`. ✅
