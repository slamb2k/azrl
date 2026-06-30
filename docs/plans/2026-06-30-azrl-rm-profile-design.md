# azrl `--rm` / `-r` — remove a profile — Design

Date: 2026-06-30

## Goal

Add `azrl --rm | -r <name>` to delete a profile's artifacts cleanly:

- `~/.azure-profiles/<name>.conf` — the azrl profile file (`AZ_TENANT`, etc.)
- `~/.azure-profiles/<name>/` — the AZURE_CONFIG_DIR (token/state dir)
- `$PWD/.azprofile` — only when its content equals `<name>`

## Behavior

- `--rm | -r <name>` — `<name>` is **required**; omitted ⇒ error + exit 2.
  A destructive op never guesses what to delete (unlike `--init`/`--save`,
  which default to the directory name).
- Removes whichever targets **exist** (idempotent). If none exist, print
  `azrl: nothing to remove for <name>` and exit 0.
- `$PWD/.azprofile` is removed **only if** it exists and its content (trimmed)
  equals `<name>`. Checks `$PWD` only — not a walk-up — so a parent dir's
  pointer is never deleted.
- **Safety:** print the exact paths to be removed, then prompt
  `Remove these? [y/N]` (default **N**). `-y` / `--yes` skips the prompt for
  non-interactive use. Decline ⇒ exit 1, nothing removed. A closed/non-tty
  stdin reads as a decline (never hangs).
- **Guards:** refuse a `<name>` containing `/` (path traversal); refuse
  `--rm azrl` (that is the global config, not a profile).
- Needs **no global `azrl.conf`** — the `--rm` block runs before the conf
  load and exits (like `--save`).
- Does **not** run `az logout` — `rm -rf` of the state dir already discards
  the tokens; logging out would need network + config wiring for no benefit.

## Code shape

Per project convention, logic lives in `azrl-lib.sh` as a testable function:

```bash
azrl_rm_profile <name> <confdir> <pwd> <assume_yes>
```

- Builds the list of existing targets (`<confdir>/<name>.conf`,
  `<confdir>/<name>/`, and `<pwd>/.azprofile` iff it names `<name>`).
- Empty list ⇒ print "nothing to remove" + return 0.
- Print targets. If `assume_yes != 1`: prompt and `read -r ans || ans=n`;
  `[[ $ans =~ ^[Yy] ]]` else print "aborted" + return 1.
- Remove each existing target; return 0.

The orchestrator (`azrl`) parses `--rm/-r` (sets mode + name) and `-y/--yes`
(sets assume-yes), validates the name (non-empty, no `/`, not `azrl`), then
calls `azrl_rm_profile "$NAME" "$HOME/.azure-profiles" "$PWD" "$ASSUME_YES"`
and exits with its status.

## Testing (TDD)

Pure-filesystem tests (no PATH shims needed):

- removes conf + dir + matching `.azprofile` (assume_yes=1)
- leaves a non-matching `.azprofile` (names "other") untouched
- nothing-to-remove ⇒ return 0 + message
- decline: `printf 'n' | azrl_rm_profile … 0` ⇒ returns 1, files remain
- confirm: `printf 'y' | azrl_rm_profile … 0` ⇒ removes
- name guards: `/` rejected, `azrl` rejected (orchestrator level)
- orchestrator: `azrl --rm` with no name ⇒ exit 2

## Docs

Usage text (`azrl --rm <name>`) and a README line noting it deletes both the
conf and the token dir, and the `$PWD/.azprofile` only when it names `<name>`.
