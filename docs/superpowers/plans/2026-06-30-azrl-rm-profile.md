# azrl `--rm` Profile Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `azrl --rm | -r <name>` to delete a profile's conf, its AZURE_CONFIG_DIR, and a matching `$PWD/.azprofile`, with a y/N confirmation (`-y` to skip).

**Architecture:** A pure, testable `azrl_rm_profile` in `azrl-lib.sh` computes the existing targets, confirms, and removes them; the `azrl` orchestrator parses `--rm/-r` + `-y/--yes`, validates the name, and calls it. No global `azrl.conf` needed. TDD with bats (pure-filesystem, no PATH shims).

**Tech Stack:** Bash (`set -euo pipefail`), bats, shellcheck.

## Global Constraints

- All scripts use `set -euo pipefail`.
- `azrl-lib.sh` must have **no side effects on source**.
- New logic goes in `azrl-lib.sh` as a testable function, called from `azrl`.
- Lint clean: `shellcheck azrl azrl-lib.sh azrl-capture`.
- Conventional commits with scope, e.g. `feat(azrl): ...`.
- Run `bats tests/azrl.bats` before committing; it must stay green.
- bats test rule: a bare `! grep`/`! cmd` assertion is INERT (bash exempts `!` from `set -e`; usually not the last line) — it asserts nothing. Use `[ "$status" -ne 0 ]` / `[ ! -e <path> ]` for negative checks. Never a bare `! cmd`.

---

### Task 1: `azrl --rm | -r <name>` profile removal

**Files:**
- Modify: `azrl-lib.sh` (add `azrl_rm_profile`; update `azrl_usage` text)
- Modify: `azrl` (arg parse: `--rm/-r`, `-y/--yes`; add the `--rm` block after the `--save` block)
- Modify: `tests/azrl.bats` (unit tests for `azrl_rm_profile` + an orchestrator no-name test)
- Modify: `README.md` (usage line + note)

**Interfaces:**
- Produces: `azrl_rm_profile <name> <confdir> <pwd> <assume_yes>` — removes `<confdir>/<name>.conf`, `<confdir>/<name>/`, and `<pwd>/.azprofile` (only if its trimmed content equals `<name>`); prompts for `[y/N]` on stdin unless `<assume_yes>` is `1`; returns 0 on success or nothing-to-remove, 1 if the user declines.

- [ ] **Step 1: Write the failing unit tests**

Add to `tests/azrl.bats` (after the existing `azrl_load_profile_conf` tests is fine — placement is not significant):

```bash
@test "azrl_rm_profile: removes conf, dir, and matching .azprofile (assume_yes)" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/acme"
  printf 'AZ_TENANT=acme.com\n' > "$home/.azure-profiles/acme.conf"
  printf 'acme\n' > "$work/.azprofile"
  run azrl_rm_profile acme "$home/.azure-profiles" "$work" 1
  [ "$status" -eq 0 ]
  [ ! -e "$home/.azure-profiles/acme.conf" ]
  [ ! -e "$home/.azure-profiles/acme" ]
  [ ! -e "$work/.azprofile" ]
  rm -rf "$home" "$work"
}

@test "azrl_rm_profile: leaves a non-matching .azprofile untouched" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  printf 'AZ_TENANT=acme.com\n' > "$home/.azure-profiles/acme.conf"
  printf 'other\n' > "$work/.azprofile"
  run azrl_rm_profile acme "$home/.azure-profiles" "$work" 1
  [ "$status" -eq 0 ]
  [ ! -e "$home/.azure-profiles/acme.conf" ]
  [ -f "$work/.azprofile" ]
  [ "$(cat "$work/.azprofile")" = "other" ]
  rm -rf "$home" "$work"
}

@test "azrl_rm_profile: nothing to remove returns 0 with message" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles"
  run azrl_rm_profile ghost "$home/.azure-profiles" "$work" 1
  [ "$status" -eq 0 ]
  [[ "$output" == *"nothing to remove"* ]]
  rm -rf "$home" "$work"
}

@test "azrl_rm_profile: declines on 'n', removes nothing, returns 1" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/acme"
  printf 'AZ_TENANT=acme.com\n' > "$home/.azure-profiles/acme.conf"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    printf 'n\n' | azrl_rm_profile acme '$home/.azure-profiles' '$work' 0
  "
  [ "$status" -eq 1 ]
  [ -f "$home/.azure-profiles/acme.conf" ]
  [ -d "$home/.azure-profiles/acme" ]
  rm -rf "$home" "$work"
}

@test "azrl_rm_profile: confirms on 'y', removes, returns 0" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/acme"
  printf 'AZ_TENANT=acme.com\n' > "$home/.azure-profiles/acme.conf"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    printf 'y\n' | azrl_rm_profile acme '$home/.azure-profiles' '$work' 0
  "
  [ "$status" -eq 0 ]
  [ ! -e "$home/.azure-profiles/acme.conf" ]
  [ ! -e "$home/.azure-profiles/acme" ]
  rm -rf "$home" "$work"
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `bats tests/azrl.bats -f "azrl_rm_profile"`
Expected: FAIL — `azrl_rm_profile: command not found` (function not defined yet).

- [ ] **Step 3: Implement `azrl_rm_profile`**

Add to `azrl-lib.sh` (after `azrl_load_profile_conf`):

```bash
azrl_rm_profile() {
  # $1=name $2=confdir $3=pwd $4=assume_yes(0/1). Removes <confdir>/<name>.conf,
  # <confdir>/<name>/, and <pwd>/.azprofile (only if it names <name>). Prompts for
  # [y/N] on stdin unless assume_yes=1. Returns 0 on success/nothing-to-remove,
  # 1 if the user declines.
  local name="$1" confdir="$2" pwd_dir="$3" assume_yes="${4:-0}"
  local conf="$confdir/$name.conf" dir="$confdir/$name" azprofile="$pwd_dir/.azprofile"
  local -a targets=()
  [[ -e "$conf" ]] && targets+=("$conf")
  [[ -d "$dir" ]] && targets+=("$dir")
  if [[ -f "$azprofile" ]]; then
    local pointed
    pointed="$(tr -d '[:space:]' < "$azprofile")"
    [[ "$pointed" == "$name" ]] && targets+=("$azprofile")
  fi
  if (( ${#targets[@]} == 0 )); then
    printf 'azrl: nothing to remove for %q\n' "$name"
    return 0
  fi
  printf 'azrl: will remove:\n'
  local t
  for t in "${targets[@]}"; do printf '  %s\n' "$t"; done
  if [[ "$assume_yes" != "1" ]]; then
    local ans
    printf 'Remove these? [y/N] '
    read -r ans || ans=n
    [[ "$ans" =~ ^[Yy] ]] || { printf 'azrl: aborted\n'; return 1; }
  fi
  for t in "${targets[@]}"; do rm -rf "$t"; done
  printf 'azrl: removed profile %q\n' "$name"
  return 0
}
```

- [ ] **Step 4: Run the unit tests to verify they pass**

Run: `bats tests/azrl.bats -f "azrl_rm_profile"`
Expected: PASS (5 tests).

- [ ] **Step 5: Write the failing orchestrator test**

Add to `tests/azrl.bats`:

```bash
@test "azrl --rm: requires a profile name (exit 2)" {
  home="$(mktemp -d)"
  HOME="$home" run "${BATS_TEST_DIRNAME}/../azrl" --rm
  [ "$status" -eq 2 ]
  rm -rf "$home"
}

@test "azrl --rm: removes a named profile with -y" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/acme"
  printf 'AZ_TENANT=acme.com\n' > "$home/.azure-profiles/acme.conf"
  HOME="$home" run bash -c "cd '$work' && '${BATS_TEST_DIRNAME}/../azrl' --rm acme -y"
  [ "$status" -eq 0 ]
  [ ! -e "$home/.azure-profiles/acme.conf" ]
  [ ! -e "$home/.azure-profiles/acme" ]
  rm -rf "$home" "$work"
}

@test "azrl --rm: refuses the reserved name azrl (exit 2)" {
  home="$(mktemp -d)"
  HOME="$home" run "${BATS_TEST_DIRNAME}/../azrl" --rm azrl -y
  [ "$status" -eq 2 ]
  rm -rf "$home"
}
```

- [ ] **Step 6: Run the orchestrator tests to verify they fail**

Run: `bats tests/azrl.bats -f "azrl --rm"`
Expected: FAIL — `--rm` is an unknown flag today, so it exits 2 for *all three*; the second test (`removes a named profile with -y`) fails because it expects exit 0 and a removed conf.

- [ ] **Step 7: Wire the orchestrator**

In `azrl`, add an assume-yes default near the other mode vars (after `export AZRL_FORCE_PASTE=0`):

```bash
ASSUME_YES=0
```

In the `for a in "$@"` arg-parse loop, add two cases alongside `--save|-s`:

```bash
    --rm|-r) AZRL_MODE=rm ;;
    -y|--yes) ASSUME_YES=1 ;;
```

Add the `--rm` block immediately after the `--save` block's closing `fi` (and before the `cleanup()` definition):

```bash
# --rm: delete a profile's conf, its AZURE_CONFIG_DIR, and $PWD/.azprofile (only
# when it names that profile). Requires an explicit name. No login, no global conf.
if [[ "$AZRL_MODE" == "rm" ]]; then
  NAME="$PROFILE_ARG"
  [[ -n "$NAME" ]] || { printf 'azrl: --rm requires a profile name\n' >&2; exit 2; }
  [[ "$NAME" == */* ]] && { printf 'azrl: invalid profile name %q\n' "$NAME" >&2; exit 2; }
  [[ "$NAME" == "azrl" ]] && { printf 'azrl: refusing to remove the global azrl config\n' >&2; exit 2; }
  rc=0
  azrl_rm_profile "$NAME" "$HOME/.azure-profiles" "$PWD" "$ASSUME_YES" || rc=$?
  exit "$rc"
fi
```

- [ ] **Step 8: Run the orchestrator tests to verify they pass**

Run: `bats tests/azrl.bats -f "azrl --rm"`
Expected: PASS (3 tests).

- [ ] **Step 9: Update usage text**

In `azrl-lib.sh` `azrl_usage`, add to the synopsis (after the `--init` line):

```
  azrl --rm <name>
```

and under Options (after the `--init` block):

```
  --rm, -r         Remove profile <name>: its <name>.conf, its AZURE_CONFIG_DIR
                   (~/.azure-profiles/<name>/), and $PWD/.azprofile if it names
                   <name>. Prompts unless -y/--yes is given.
  -y, --yes        Skip the --rm confirmation prompt.
```

- [ ] **Step 10: Update README**

In `README.md`, add to the Usage code block (after the `--save` line):

```
azrl --rm <name>           # remove a profile: conf + token dir (+ matching .azprofile)
```

And add one prose line after the "Saving and initializing profile configs" section:

```
`--rm <name>` deletes the profile's `<name>.conf`, its token dir
`~/.azure-profiles/<name>/`, and `$PWD/.azprofile` when it names `<name>`. It
prompts for confirmation unless you pass `-y`.
```

- [ ] **Step 11: Full suite + lint**

Run: `bats tests/azrl.bats`
Expected: all tests pass.
Run: `shellcheck azrl azrl-lib.sh azrl-capture`
Expected: clean (no output).

- [ ] **Step 12: Commit**

```bash
git add azrl azrl-lib.sh tests/azrl.bats README.md
git commit -m "feat(azrl): add --rm to remove a profile (conf, state dir, .azprofile)"
```

---

## Self-Review

**Spec coverage:** `azrl_rm_profile` with conf + dir + matching `.azprofile` removal (Step 3) ✅; require explicit name + `/` guard + `azrl` guard (Step 7) ✅; y/N prompt with `-y` skip, decline→1 (Step 3) ✅; idempotent nothing-to-remove (Step 3) ✅; no global conf needed (block placed before conf load, exits) ✅; usage + README (Steps 9–10) ✅; tests for all paths (Steps 1, 5) ✅.

**Placeholder scan:** No TBD/TODO; every code step shows complete code. ✅

**Type/name consistency:** `azrl_rm_profile <name> <confdir> <pwd> <assume_yes>` signature and `AZRL_MODE=rm` / `ASSUME_YES` / flags `--rm|-r` `-y|--yes` used consistently across lib, orchestrator, and tests. Negative assertions use `[ ! -e ]` / `[ "$status" -ne 0 ]`, never bare `! cmd`. ✅

**Note on the `azrl` guard test (Step 5):** it asserts that `--rm azrl -y` returns exit 2 from the name guard *before* any filesystem work — no profile setup is needed.
