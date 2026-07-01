# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`azrl` (Azure Remote Login) is a Go CLI that runs interactive `az login` from a
headless/remote Linux VM, popping the sign-in browser on your local machine and
forwarding the random OAuth callback port back to the VM — even when Conditional
Access blocks device-code flow.

It is **provider-aware**: the same binary also manages multiple **GitHub**
accounts (`azrl gh …`, or the `ghrl` alias). Bare invocation opens a tabbed TUI
(Azure | GitHub). Each provider implements a shared `Provider` interface and both
pass one contract suite.

## Commands

```bash
go build ./...             # build the binary
go test ./...              # run the unit + integration suite
gofmt -l .                 # check formatting (empty output = clean)
./install.sh               # go build + install into ~/.local/bin, gitignore .azprofile, bootstrap config
```

## Architecture

The tool ships two binaries from one codebase (`azrl` + the `ghrl` alias) with
Cobra subcommands, split across packages:

- **`main.go`** — `azrl` entrypoint; calls `cmd.Execute()`.
- **`cmd/ghrl/`** — `ghrl` entrypoint; calls `cmd.ExecuteGhrl()` (GitHub subcommands promoted to top level, TUI preselected on the GitHub tab).
- **`cmd/`** — Cobra tree. Azure at top level (`login`, `init`, `capture`, `use`, `rm`, `list`); a `gh` group (`gh login/list/use/switch/rm/capture/status`); hidden `__browser` (smart shim) and `__browser-capture` self-shims. Bare `azrl` launches the `internal/ui` tabbed TUI.
- **`internal/config/`** — loads `~/.azure-profiles/azrl.conf`, parses `KEY=value`; `ProfilesDir()` / `GithubProfilesDir()`.
- **`internal/profile/`** — pure profile logic with a parameterized `Scheme` (pointer filename, reserved conf name, detail/label keys) shared by Azure (`.azprofile`/`AZ_*`) and GitHub (`.ghprofile`/`GH_*`). No side effects; fully unit-tested.
- **`internal/provider/`** — the `Provider` interface (Name/Title/ProfilesDir/Scheme/ListProfiles/Resolve/Use/Remove/SetLabel); `providertest.RunContract` is the shared suite both providers pass.
- **`internal/azure/`** — the `az`/`ssh` login lifecycle: `CleanSlate`, `LoginCapture`, `WaitForLogin`, `AssertAccount`, `SetSubscription`, plus the Azure `Provider`.
- **`internal/github/`** — the `gh`/`git` lifecycle: `Login` (`--insecure-storage`, isolated `GH_CONFIG_DIR`), `SetupRepo` (`gh auth setup-git` + repo-local credential username), `Switch`/`Current`, `WhoAmI`/`AssertAccount`, plus the GitHub `Provider`.
- **`internal/bridge/`** — shared SSH reverse-tunnel (`ssh -R` path B) / paste-line (path A) browser bridge, plus `PasteLine`.
- **`internal/browsercapture/`** — the smart `__browser` shim: `Classify` (device vs loopback), `ParseCallbackPort`, `Run` (relay or tunnel), and `XdgOpenShimScript` (shadow `xdg-open` for GCM).
- **`internal/ui/`** — tabbed Bubble Tea TUI: `tabsModel` container (`[`/`]` to switch), the existing Azure `Model`, and the GitHub `githubView`.

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
- `~/.github-profiles/<profile>.conf` — per-profile GitHub: `GH_HOST` (required), `GH_USER`, `GH_LABEL`, `GH_PROTOCOL`.
- `<repo>/.ghprofile` — one line naming the GitHub profile; resolved the same way. Globally gitignored.
- `~/.github-profiles/<profile>/` — isolated `GH_CONFIG_DIR` per profile (its own `hosts.yml`/token; requires `gh auth login --insecure-storage`).

## Testing approach

Pure logic in `internal/profile` is unit-tested directly. `internal/azure` and `internal/github` integration points are tested by shimming `az`/`gh`/`ssh`/`git` onto `PATH` via `t.Setenv("PATH", tmpDir)` with fake executables (see the `Bridge`, `LoginCapture`, and `internal/github` login/use tests for the pattern). Both providers pass the shared `internal/provider/providertest.RunContract` suite. TUI model behaviour is tested via `internal/ui` model + tab-container unit tests that assert `View()` output and message handling. Items that need a real laptop+VM+GCM+VS Code are listed in `specs/github-remote-login.manual-verify.md`. The project was built TDD-first; see `HANDOVER.md` for full historical context.

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
