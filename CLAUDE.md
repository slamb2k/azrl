# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`azrl` (Azure Remote Login) is a Go CLI that runs interactive `az login` from a
headless/remote Linux VM, popping the sign-in browser on your local machine and
forwarding the random OAuth callback port back to the VM — even when Conditional
Access blocks device-code flow.

## Commands

```bash
go build ./...             # build the binary
go test ./...              # run the unit + integration suite
gofmt -l .                 # check formatting (empty output = clean)
./install.sh               # go build + install into ~/.local/bin, gitignore .azprofile, bootstrap config
```

## Architecture

The tool is a single Go binary with Cobra subcommands, split across packages:

- **`main.go`** — entrypoint; calls `cmd.Execute()`.
- **`cmd/`** — Cobra subcommands: `login`, `init`, `capture`, `use`, `rm`, `list`, and the hidden `__browser-capture` self-shim. Bare `azrl` launches the `internal/ui` TUI.
- **`internal/config/`** — loads `~/.azure-profiles/azrl.conf` and parses `KEY=value` config files.
- **`internal/profile/`** — pure profile logic: resolve `.azprofile` by walking up from `$PWD`, read/write conf files, `use`, `rm`. No side effects; fully unit-tested.
- **`internal/azure/`** — the `az`/`ssh` login lifecycle: `CleanSlate`, `LoginCapture`, `Bridge`, `WaitForLogin`, `AssertAccount`, `SetSubscription`. Shells out to `az` and `ssh`; tested by shimming them onto `PATH` via `t.Setenv("PATH", ...)`.
- **`internal/ui/`** — Bubble Tea TUI: banner, angel art, profile list, login/action flow with spinner.

### The login flow

1. `CleanSlate` — `az logout` + `az account clear`, remove scoped MSAL caches within the isolated `AZURE_CONFIG_DIR`.
2. `LoginCapture` — runs `az login --allow-no-subscription` in the background with `BROWSER` set to `azrl __browser-capture`, polls for the capture file, parses the random callback port.
3. `Bridge` — **path B (zero-paste)**: if the local host is SSH-reachable, open a reverse tunnel (`ssh -R port:localhost:port`) and launch the browser there. **Path A (fallback / paste)**: print a one-line `ssh -fNL …` for the user to paste locally.
4. `WaitForLogin` — waits for `az login` to complete with a configurable timeout (default 180s).
5. `AssertAccount` — verifies tenant (by domain or GUID) and optionally the expected user.

### Configuration model

- `~/.azure-profiles/azrl.conf` — global: `LOCAL_HOST`, `LOCAL_BROWSER_CMD`, `VM_HOST`.
- `~/.azure-profiles/<profile>.conf` — per-profile: `AZ_TENANT` (required), `AZ_TENANT_ID` (GUID — required for guest/B2B where `tenantDefaultDomain` is null), `AZ_DEFAULT_SUB`, `AZ_EXPECT_USER`.
- `<repo>/.azprofile` — one line naming the profile; resolved by walking up from `$PWD`. Globally gitignored (never commit it).
- `~/.azure-profiles/<profile>/` — isolated `AZURE_CONFIG_DIR` per profile.

## Testing approach

Pure logic in `internal/profile` is unit-tested directly. `internal/azure` integration points are tested by shimming `az` and `ssh` onto `PATH` via `t.Setenv("PATH", tmpDir)` with fake executables (see the `Bridge` and `LoginCapture` tests for the pattern). TUI model behaviour is tested via `internal/ui` model unit tests that assert `View()` output and message handling. The project was built TDD-first; see `HANDOVER.md` for full historical context.

## Development Workflow

The `mad-skills` pipeline drives feature work here:

```
/speccy → specs/<name>.md → /build specs/<name>.md → /ship
```

- `/speccy` interviews and writes a structured spec to `specs/`
- `/build` reads the spec, explores, designs, implements, reviews, tests
- `/ship` commits, opens a PR, waits for CI, squash-merges, and cleans up
- `/sync` brings `main` up to date before new work

`specs/` holds specifications; `context/` holds domain knowledge and references;
`.tmp/` is gitignored scratch.

## Branch Discipline

- **Sync to main before new work** — `/sync` or `git checkout main && git pull`
- **Never branch from a feature branch** — always branch from an up-to-date `main`
- **One feature per branch** — don't stack unrelated changes
- **After shipping a PR, sync immediately** before starting the next task
- **If a PR is pending review**, switch to main before unrelated work

## Guardrails

- Verify tool output format before chaining into another tool
- Don't assume APIs support batch operations — check first
- Preserve intermediate outputs when workflows fail mid-execution
- Temporary files go in `.tmp/` — never store important data there
- Don't build before designing
