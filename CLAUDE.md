# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`azrl` (Azure Remote Login) is a Go CLI that runs interactive `az login` from a
headless/remote Linux VM, popping the sign-in browser on your local machine and
forwarding the random OAuth callback port back to the VM — even when Conditional
Access blocks device-code flow.

It is **provider-aware**: the same binary also manages multiple **GitHub**
accounts (`azrl gh …`, or the `ghrl` alias), **AWS** SSO profiles
(`azrl aws …`), and **GCP** `gcloud` configurations (`azrl gcp …`). Bare
invocation opens a tabbed TUI (dashboard | Azure | AWS | GCP | GitHub). Each
provider implements a shared `Provider` interface and all pass one contract
suite.

## Commands

```bash
go build ./...             # build the binary
go test ./...              # run the unit + integration suite
gofmt -l .                 # check formatting (empty output = clean)
./install.sh               # go build + install into ~/.local/bin, gitignore .azprofile, bootstrap config
lefthook install           # activate the pre-commit/pre-push git hooks (once per clone)
```

CI runs in `.github/workflows/`: `ci.yml` (build/test/gofmt on PRs) and
`release.yml` (GoReleaser cross-platform binaries, Homebrew tap, `.deb`/`.rpm`,
curl installer). `scripts/install.sh` is the packaged curl-based installer; the
top-level `./install.sh` is the local dev installer.

## Architecture

The tool ships two binaries from one codebase (`azrl` + the `ghrl` alias) with
Cobra subcommands, split across packages:

- **`main.go`** — `azrl` entrypoint; calls `cmd.Execute()`.
- **`cmd/ghrl/`** — `ghrl` entrypoint; calls `cmd.ExecuteGhrl()` (GitHub subcommands promoted to top level, TUI preselected on the GitHub tab).
- **`cmd/`** — Cobra tree. Azure at top level (`login`, `capture`, `use`, `rm`, `list`); `login <name>` creates a profile on first use (Azure discovers the tenant via a tenant-less sign-in; `--yes` skips the confirm) — the old `init` command was removed (a hidden deprecated `init` stub just points users at `login`); cross-provider `status [--json]` (disk-only, three sections: MAPPINGS / AMBIENT / UNMAPPED PROFILES via `ui.BuildOverview`; `--json` emits `{"mappings":[…],"ambient":[…],"unmapped":[…]}` — reshaped in v0.7.0); a `gh` group (`gh login/list/use/rm/capture/status` — `switch` was removed; a hidden deprecated `switch` stub on both the gh group and the promoted ghrl top level just points users at `gh auth switch` / `use`, like the `init` stub); an `aws` group (`aws login/list/use/rm/capture/status` — no `switch` verb, self-wired via `init()`); a `gcp` group (`gcp login/list/use/rm/capture/status`, likewise self-wired via `init()`); hidden `__browser` (smart shim) and `__browser-capture` self-shims. Bare `azrl` launches the `internal/ui` tabbed TUI (dashboard is the default landing view).
- **`internal/config/`** — loads `~/.azure-profiles/azrl.conf`, parses `KEY=value`; `ProfilesDir()` / `GithubProfilesDir()` / `AwsProfilesDir()` / `GcpProfilesDir()`.
- **`internal/profile/`** — pure profile logic with a parameterized `Scheme` (pointer filename, reserved conf name, detail/label keys) shared by Azure (`.azprofile`/`AZ_*`) and GitHub (`.ghprofile`/`GH_*`). Fully unit-tested. Also the per-provider `mappings` TSV index (`mapping.go`): `RecordMapping` appends/updates `<abs-dir>\t<profile>\t<source>` atomically (source `pointer` or `gitconfig`; hooked from `Scheme.Touch` and github `SetupRepo`), and `Scheme.ReadMappings` prunes + pointer-revalidates on every read (dead dirs dropped, retargeted pointers rewritten).
- **`internal/provider/`** — the `Provider` interface (Name/Title/ProfilesDir/Scheme/ListProfiles/Resolve/Use/Remove/SetLabel/`Status`/`WatchDirs`/`Ambient`); `providertest.RunContract` is the shared suite all providers pass (including an `Ambient()` no-network exercise). `Status(name, confdir)` returns a **disk-only** snapshot (identity, directory, expiry, drift, last-used — no network); its `LastUsed` folds the token-cache file mtime (via `LatestMtime`) so external CLI activity re-sorts the dashboard. `Ambient()` is a disk+process-env read-through of the CLI's native default identity (azure `azureProfile.json` BOM-aware; github `hosts.yml` legacy + multi-account; aws `AWS_PROFILE` env else `[default]` in `~/.aws/config` with SSO-cache enrichment; gcp `CLOUDSDK_ACTIVE_CONFIG_NAME` env else `active_config`) — never spawns a CLI, never networks, best-effort; `MatchProfile(statuses, identity)` reverse-maps the identity to a managed profile (MRU wins). `WatchDirs()` lists the existing cache/config dirs the dashboard watches via fsnotify. Shared helpers: `Drifted` (cwd pinned to profile AND ambient `*_CONFIG_DIR` ≠ isolated dir), and a self-registering `All()`/`Collect()` registry (providers register via `init()`).
- **`internal/azure/`** — the `az`/`ssh` login lifecycle: `CleanSlate`, `LoginCapture`, `WaitForLogin`, `AssertAccount`, `SetSubscription`, plus the Azure `Provider`.
- **`internal/github/`** — the `gh`/`git` lifecycle: `Login` (`--insecure-storage`, isolated `GH_CONFIG_DIR`), `SetupRepo` (`gh auth setup-git` + repo-local credential username), `WhoAmI`/`AssertAccount`, `ResolveDir` (native-first directory association: repo-local `git config credential.https://<host>.username` outranks the `.ghprofile` walk-up; a disagreement is a conflict rendered with both, and a username matching no profile is an unmanaged, adoptable mapping), plus the GitHub `Provider`. `Switch`/`Current` (and `~/.github-profiles/.current`) were deleted in v0.7.0 — azrl never mutates or stores a default identity.
- **`internal/aws/`** — the `aws`/`sts` SSO lifecycle: `Login` (`aws sso login` reusing the browser bridge unmodified — PKCE loopback `127.0.0.1` callback, `--use-device-code` fallback), `Status` (disk-only — reads `~/.aws/sso/cache/*.json` for identity/expiry, no network), `AssertAccount` (`aws sts get-caller-identity` guardrail: exact account + `AWSReservedSSO_<permset>` role-boundary match), `SyncConfig`, `.envrc` (`WriteEnvrc`), plus the self-registering AWS `Provider`. `--isolate` opt-in scopes `AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE`.
- **`internal/gcp/`** — the `gcloud` lifecycle over native named configurations: `Login` (`gcloud auth login` reusing the browser bridge unmodified — default binds a `localhost` loopback callback), `Status` (disk-only — reads the gcloud config dir: `configurations/config_<name>` `[core] account` for identity and `active_config` for drift; `Expiry` nil in v1, no network), `AssertAccount` (`gcloud auth list --filter=status:ACTIVE` guardrail: exact account-email match), `SyncConfig` (idempotent `gcloud config configurations create --no-activate` + project/region set), `.envrc` (`WriteEnvrc`), a GKE-isolation warning (`gke-gcloud-auth-plugin` ignores `CLOUDSDK_CONFIG` — a documented known gap; v1 warns), plus the self-registering GCP `Provider`. `--isolate` opt-in scopes `CLOUDSDK_CONFIG`; default uses `CLOUDSDK_ACTIVE_CONFIG_NAME`.
- **`internal/bridge/`** — shared SSH reverse-tunnel (`ssh -R` path B) / paste-line (path A) browser bridge, plus `PasteLine`.
- **`internal/browsercapture/`** — the smart `__browser` shim: `Classify` (device vs loopback), `ParseCallbackPort`, `Run` (relay or tunnel), and `XdgOpenShimScript` (shadow `xdg-open` for GCM).
- **`internal/ui/`** — tabbed Bubble Tea TUI: `tabsModel` container (`←`/`→` or `[`/`]` to switch), the existing Azure `Model`, name-keyed provider tabs, and the `dashboard` view. The container renders the winged banner **horizontally centered** above the tab bar on every tab (compact one-line wordmark on narrow terminals), and each tab's view fills the full terminal width and height. Tab order is `Dashboard | Azure | AWS | Google Cloud | GitHub` — an `azureFirst` reorder puts Azure first after the dashboard, then the rest in `provider.All()`'s order (the landing view's UNMAPPED PROFILES rows still sort by last-used). The AWS, GCP, and GitHub tabs share a `providerTabView` component (`provider_view.go`) that renders with the Azure `Model`'s header/two-pane (`PROFILES` | `ACTION`)/footer template via shared helpers (`renderPaneFrame`/`renderProfilePane`/`selectionBar`); each keeps its own action set. The dashboard is the default landing view for bare `azrl` (tab 0), built by `BuildOverview` (`overview.go`, shared with `cmd`'s `status`) as three sections: **MAPPINGS** — directory → `provider:profile` rows from the mapping index with scope markers (● cwd / ↑ ancestor), a pointer-vs-`(git)` source icon, the `⚠ drift` marker, git-config-vs-pointer conflict warnings showing both, and unmanaged rows in a distinct accent with `[a]dopt` handing off to the provider's capture flow; **AMBIENT** — 🌐 one row per provider with the CLI's native default identity, its winning source, and the matched managed profile or `unmanaged`; **UNMAPPED PROFILES** — muted one-liners (`provider:name · identity · expiry`) so expiry warnings stay visible. It is **live** — besides the `tea.Tick` poll (`DASHBOARD_POLL_SECS`, default 3s) it watches each provider's `WatchDirs()` — which now include the native ambient dirs (`~/.azure`, `~/.config/gh`, `~/.aws`, `~/.config/gcloud`) — via **fsnotify** (`github.com/fsnotify/fsnotify`) and re-aggregates immediately when tokens or ambient state change outside azrl; the watcher is torn down on exit (`Close`). `Enter` drills into the provider tab with the profile pre-selected (inside a provider tab, `Enter` on a profile opens the ACTION pane, `Esc` returns), `F5` refreshes all, `[w]` rechecks drift; `Remove` is bound to the `Delete` key. Reading the overview also self-heals the index (hand-made pointers are recorded on first contact). `ghrl` still opens on the GitHub tab.

### The login flow

1. `CleanSlate` — `az logout` + `az account clear`, remove scoped MSAL caches within the isolated `AZURE_CONFIG_DIR`.
2. `LoginCapture` — runs `az login --allow-no-subscription` in the background with `BROWSER` set to `azrl __browser-capture`, polls for the capture file, parses the random callback port.
3. `Bridge` — **path B (zero-paste)**: if the local host is SSH-reachable, open a reverse tunnel (`ssh -R port:localhost:port`) and launch the browser there. **Path A (fallback / paste)**: print a one-line `ssh -fNL …` for the user to paste locally.
4. `WaitForLogin` — waits for `az login` to complete with a configurable timeout (default 180s).
5. `AssertAccount` — verifies tenant (by domain or GUID) and optionally the expected user.

### Configuration model

- `~/.azure-profiles/azrl.conf` — global: `LOCAL_HOST`, `LOCAL_BROWSER_CMD`, `VM_HOST`, `DASHBOARD_POLL_SECS` (optional, default 3).
- `~/.<provider>-profiles/mappings` — per-provider TSV index of directory → profile mappings (`<abs-dir>\t<profile>\t<source>`, source `pointer` or `gitconfig`). Auto-managed: written on use/login/capture/`Touch` (and github `SetupRepo`), pruned + revalidated on read, self-healed when azrl first sees a hand-made pointer — not hand-edited.
- `~/.azure-profiles/<profile>.conf` — per-profile: `AZ_TENANT` (required), `AZ_TENANT_ID` (GUID — required for guest/B2B where `tenantDefaultDomain` is null), `AZ_DEFAULT_SUB`, `AZ_EXPECT_USER`. `LAST_USED`/`LAST_DIR` are auto-managed (bumped on use/login/capture) — not hand-edited.
- `<repo>/.azprofile` — one line naming the profile; resolved by walking up from `$PWD`. Globally gitignored (never commit it).
- `~/.azure-profiles/<profile>/` — isolated `AZURE_CONFIG_DIR` per profile.
- `~/.github-profiles/<profile>.conf` — per-profile GitHub: `GH_HOST` (required), `GH_USER`, `GH_LABEL`, `GH_PROTOCOL`.
- `<repo>/.ghprofile` — one line naming the GitHub profile; resolved the same way. Globally gitignored.
- `~/.github-profiles/<profile>/` — isolated `GH_CONFIG_DIR` per profile (its own `hosts.yml`/token; requires `gh auth login --insecure-storage`).
- `~/.aws-profiles/<profile>.conf` — per-profile AWS: `AWS_SSO_START_URL`, `AWS_SSO_REGION`, `AWS_ACCOUNT_ID`, `AWS_ROLE_NAME`, `AWS_EXPECT_ACCOUNT`, `AWS_EXPECT_ARN`, `AWS_LABEL`, `AWS_ISOLATE`. `LAST_USED`/`LAST_DIR` are auto-managed.
- `<repo>/.awsprofile` — one line naming the AWS profile; resolved the same way. Globally gitignored. The `.envrc` exports `AWS_PROFILE=<name>` (default) or the isolate file-path vars.
- `~/.aws-profiles/<profile>/` — isolated `AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE` per profile (only under `--isolate`).
- `~/.gcp-profiles/<profile>.conf` — per-profile GCP: `GCP_CONFIG_NAME` (named gcloud configuration; defaults to the profile name), `GCP_PROJECT` (required), `GCP_REGION`, `GCP_EXPECT_ACCOUNT`, `GCP_LABEL`, `GCP_ISOLATE`. `LAST_USED`/`LAST_DIR` are auto-managed.
- `<repo>/.gcpprofile` — one line naming the GCP profile; resolved the same way. Globally gitignored. The `.envrc` exports `CLOUDSDK_ACTIVE_CONFIG_NAME=<name>` (default) or `CLOUDSDK_CONFIG=~/.gcp-profiles/<name>` (under `--isolate`).
- `~/.gcp-profiles/<profile>/` — isolated `CLOUDSDK_CONFIG` per profile (only under `--isolate`).

## Testing approach

Pure logic in `internal/profile` is unit-tested directly. `internal/azure` and `internal/github` integration points are tested by shimming `az`/`gh`/`ssh`/`git` onto `PATH` via `t.Setenv("PATH", tmpDir)` with fake executables (see the `Bridge`, `LoginCapture`, and `internal/github` login/use tests for the pattern). Both providers pass the shared `internal/provider/providertest.RunContract` suite. TUI model behaviour is tested via `internal/ui` model + tab-container unit tests that assert `View()` output and message handling. Items that need a real laptop+VM+GCM+VS Code are listed in `specs/github-remote-login.manual-verify.md`. The project was built TDD-first; see `docs/HANDOVER-origin.md` for full historical context.

## Development Workflow

The `mad-skills` pipeline drives feature work here:

```
/speccy → specs/<name>.md → /build specs/<name>.md → /ship
```

- `/speccy` interviews and writes a structured spec to `specs/`
- `/build` reads the spec, explores, designs, implements, reviews, tests
- `/ship` commits, opens a PR, waits for CI, squash-merges, and cleans up
- `/sync` brings `main` up to date before new work

`specs/` holds specifications (`github-remote-login.*` shipped in #17;
`status-dashboard.*` shipped as Phase 5.5; `multi-cloud-providers.*` is
scoped/designed but not yet built). `context/` holds domain knowledge and
references. `docs/` holds
design notes and historical plans: `docs/HANDOVER-origin.md` (full historical
context), `docs/design.md`, `docs/build-plan.md`, and dated design plans under
`docs/plans/` and `docs/superpowers/plans/`. `.tmp/` is gitignored scratch.

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
