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

CI runs in `.github/workflows/ci.yml` ("CI & Release"): `validate`
(build/test/gofmt on PRs and pushes) and, on every merge to `main`, `release` —
a conventional-commit semantic bump tags HEAD and GoReleaser publishes the
cross-platform binaries, Homebrew tap, `.deb`/`.rpm`, and curl installer with
OIDC-signed build provenance. `scripts/install.sh` is the packaged curl-based installer; the
top-level `./install.sh` is the local dev installer.

## Architecture

The tool ships two binaries from one codebase (`azrl` + the `ghrl` alias) with
Cobra subcommands, split across packages:

- **`main.go`** — `azrl` entrypoint; calls `cmd.Execute()`.
- **`cmd/ghrl/`** — `ghrl` entrypoint; calls `cmd.ExecuteGhrl()` (GitHub subcommands promoted to top level, TUI preselected on the GitHub tab).
- **`cmd/`** — Cobra tree. Azure at top level (`login`, `capture`, `use`, `rm`, `list`); `login <name>` creates a profile on first use (Azure discovers the tenant via a tenant-less sign-in; `--yes` skips the confirm) and pin-on-create writes the dir pointer uniformly across providers (existing-profile login never pins) — the old `init` command was removed (a hidden deprecated `init` stub just points users at `login`); cross-provider `status [--json]` (disk-only, three sections: MAPPINGS / AMBIENT / UNMAPPED PROFILES via `ui.BuildOverview`; `--json` emits `{"mappings":[…],"ambient":[…],"unmapped":[…]}` — reshaped in v0.7.0); a `gh` group (`gh login/list/use/rm/capture/status/browser` — `switch` was removed; a hidden deprecated `switch` stub on both the gh group and the promoted ghrl top level just points users at `gh auth switch` / `use`, like the `init` stub); an `aws` group (`aws login/list/use/rm/capture/status/browser` — no `switch` verb, self-wired via `init()`); a `gcp` group (`gcp login/list/use/rm/capture/status/browser`, likewise self-wired via `init()`); a top-level `setup` command (`cmd/setup.go`: env-detect wizard writing `azrl.conf`; `--yes`/`--print`); a top-level `browser <name>` verb (Azure); `browser` maps a profile to a local Edge/Chrome profile discovered via `internal/browserpick` (numbered pick, identity-match sorted, manual entry, clear); hidden `__browser` (smart shim) and `__browser-capture` self-shims. Bare `azrl` launches the `internal/ui` tabbed TUI (dashboard is the default landing view).
- **`internal/config/`** — loads `~/.azure-profiles/azrl.conf`, parses `KEY=value`; `ProfilesDir()` / `GithubProfilesDir()` / `AwsProfilesDir()` / `GcpProfilesDir()`.
- **`internal/profile/`** — pure profile logic with a parameterized `Scheme` (pointer filename, reserved conf name, detail/label keys) shared by Azure (`.azprofile`/`AZ_*`) and GitHub (`.ghprofile`/`GH_*`). Fully unit-tested. Also the per-provider `mappings` TSV index (`mapping.go`): `RecordMapping` appends/updates `<abs-dir>\t<profile>\t<source>` atomically (source `pointer` or `gitconfig`; hooked from `Scheme.Touch` and github `SetupRepo`), and `Scheme.ReadMappings` prunes + pointer-revalidates on every read (dead dirs dropped, retargeted pointers rewritten).
- **`internal/provider/`** — the `Provider` interface (Name/Title/ProfilesDir/Scheme/ListProfiles/Resolve/Use/Remove/SetLabel/`Status`/`WatchDirs`/`Ambient`); `providertest.RunContract` is the shared suite all providers pass (including an `Ambient()` no-network exercise). `Status(name, confdir)` returns a **disk-only** snapshot (identity, directory, expiry, drift, last-used — no network); its `LastUsed` folds the token-cache file mtime (via `LatestMtime`) so external CLI activity re-sorts the dashboard. `Ambient()` is a disk+process-env read-through of the CLI's native default identity (azure `azureProfile.json` BOM-aware; github `hosts.yml` legacy + multi-account; aws `AWS_PROFILE` env else `[default]` in `~/.aws/config` with SSO-cache enrichment; gcp `CLOUDSDK_ACTIVE_CONFIG_NAME` env else `active_config`) — never spawns a CLI, never networks, best-effort; `MatchProfile(statuses, identity)` reverse-maps the identity to a managed profile (MRU wins). `WatchDirs()` lists the existing cache/config dirs the dashboard watches via fsnotify. Shared helpers: `Drifted` (cwd pinned to profile AND ambient `*_CONFIG_DIR` ≠ isolated dir), and a self-registering `All()`/`Collect()` registry (providers register via `init()`).
- **`internal/azure/`** — the `az`/`ssh` login lifecycle: `CleanSlate`, `LoginCapture`, `WaitForLogin`, `AssertAccount`, `SetSubscription`, plus the Azure `Provider`.
- **`internal/github/`** — the `gh`/`git` lifecycle: `Login` (`--insecure-storage`, isolated `GH_CONFIG_DIR`), `SetupRepo` (`gh auth setup-git` + repo-local credential username), `WhoAmI`/`AssertAccount`, `ResolveDir` (native-first directory association: repo-local `git config credential.https://<host>.username` outranks the `.ghprofile` walk-up; a disagreement is a conflict rendered with both, and a username matching no profile is an unmanaged, adoptable mapping), plus the GitHub `Provider`. `Switch`/`Current` (and `~/.github-profiles/.current`) were deleted in v0.7.0 — azrl never mutates or stores a default identity.
- **`internal/aws/`** — the `aws`/`sts` SSO lifecycle: `Login` (`aws sso login` reusing the browser bridge unmodified — PKCE loopback `127.0.0.1` callback, `--use-device-code` fallback), `Status` (disk-only — reads `~/.aws/sso/cache/*.json` for identity/expiry, no network), `AssertAccount` (`aws sts get-caller-identity` guardrail: exact account + `AWSReservedSSO_<permset>` role-boundary match), `SyncConfig`, `.envrc` (`WriteEnvrc`), plus the self-registering AWS `Provider`. `--isolate` opt-in scopes `AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE`.
- **`internal/gcp/`** — the `gcloud` lifecycle over native named configurations: `Login` (`gcloud auth login` reusing the browser bridge unmodified — default binds a `localhost` loopback callback), `Status` (disk-only — reads the gcloud config dir: `configurations/config_<name>` `[core] account` for identity and `active_config` for drift; `Expiry` read per-account from `access_tokens.db` via the pure-Go read-only sqlittle reader, no network), `AssertAccount` (`gcloud auth list --filter=status:ACTIVE` guardrail: exact account-email match), `SyncConfig` (idempotent `gcloud config configurations create --no-activate` + project/region set), `.envrc` (`WriteEnvrc`), a GKE-isolation warning (`gke-gcloud-auth-plugin` ignores `CLOUDSDK_CONFIG` — a documented known gap; v1 warns), plus the self-registering GCP `Provider`. `--isolate` opt-in scopes `CLOUDSDK_CONFIG`; default uses `CLOUDSDK_ACTIVE_CONFIG_NAME`.
- **`internal/bridge/`** — shared SSH reverse-tunnel (`ssh -R` path B) / paste-line (path A) browser bridge, plus `PasteLine`. `VMHost(g)` resolves the VM's SSH name for the paste line: explicit `VM_SSH_HOST` wins, else derived from `$SSH_CONNECTION`, else a `<your-vm-host>` placeholder (ok=false).
- **`internal/envdetect/`** — pure, table-tested environment detection: `Detect(Env)` ranks `Candidate` configs (recommended first, ≥1) from injected signals (WSL via `/proc/version`~microsoft or `$WSL_DISTRO_NAME`→wslview; macOS→open; Linux desktop `$DISPLAY`+xdg-open→xdg-open; `$SSH_CONNECTION`/`$SSH_TTY`→remote); `DeriveVMHost` reads field 3 of `$SSH_CONNECTION`; `RealEnv()` is the only impure helper. Backs `azrl setup`.
- **`internal/browsercapture/`** — the smart `__browser` shim: `Classify` (device vs loopback), `ParseCallbackPort`, `Run` (relay or tunnel), and `XdgOpenShimScript` (shadow `xdg-open` for GCM).
- **`internal/browserpick/`** — ssh discovery of the local machine's Edge/Chrome profiles from Chromium's `Local State` (POSIX probe covering Linux/macOS/WSL in one round-trip, native-Windows `cmd /c type` fallback), rendered per-OS into a launch command (`Profile.Command`/`Label`); read-only and best-effort, any failure falls back to manual entry.
- **`internal/ui/`** — tabbed Bubble Tea TUI. `tabsModel` container: winged banner (blank line below), tab strip `Dashboard | Azure | GitHub | AWS | Google Cloud` (`preferredOrder`; the `PROVIDERS` key in `azrl.conf` — options popup `o`, default `azure,github` — selects which appear), `⇥`/`⇧⇥`/`[`/`]` cycle tabs, `↑` at the top of any list focuses the tab bar (`←`/`→` pick, `↓` returns), `d` opens a fuzzy change-directory picker (`dirpicker.go`; os.Chdir + `cwdChangedMsg` broadcast), `o` a centered options popup (`options.go`, ANSI-overlay via `overlayCenter`). One selection language: bright `selBlockActive` in the focused level, dim `selBlockParent` on ancestors only (`selMode`), tri-level focus tab bar → PROFILES → DETAILS/ACTIONS. Every tab shares the header anatomy (`headerStrip`: provider icon `providerIcon` · 📁 cwd · 👤 effective identity via `effectiveIdentity`) and pane frame (`renderPaneFrame`, legend `scopeLegend` bottom-anchored via the left-footer slot). Profile rows (`renderProfilePane` + azure's custom `profileDelegate`) lead with the five-tier relevance icon (`scopeSlot`: ● green cwd pin / ● orange parent / 🌐 global default / ● dark-white mapped-elsewhere / ● deep-grey unmapped), bold the most-active profile, italicise renamed labels. The DETAILS pane shows a key/value sheet (`profileInfoBlock`, including a `Browser` row when mapped) over an `ACTIONS (n)` radio (keycap chips left, hints right, hidden-when-inapplicable e.g. Use here on the cwd-pinned selection) — every provider tab includes a `b` "Browser profile" action that discovers profiles asynchronously and opens a fuzzy overlay picker (`browserpicker.go`), falling back to manual entry on discovery failure. All footers use `keyHelp` chips. The dashboard (`overview.go`'s `BuildOverview`, shared with `cmd status`) is live via fsnotify + `DASHBOARD_POLL_SECS` tick; its header carries a prioritised next-action hint (`dashboardHint`). Interactive flows (sign in, new profile, adopt) exec the real CLI via `runHandoff` and reload on `opDoneMsg`. `ghrl` opens on the GitHub tab.

### The login flow

1. `CleanSlate` — reaps orphaned `az login` processes (same user, parent dead — leftovers of earlier attempts that would steal the browser callback) and warns about live ones, then `az logout` + `az account clear`, remove scoped MSAL caches within the isolated `AZURE_CONFIG_DIR`.
2. `LoginCapture` — runs `az login --allow-no-subscription` in the background with `BROWSER` set to `azrl __browser-capture`, polls for the capture file, parses the random callback port.
3. `Bridge` — **path B (zero-paste)**: if the local host is SSH-reachable, open a reverse tunnel (`ssh -R port:localhost:port`) and launch the browser there. **Path A (fallback / paste)**: print a one-line `ssh -fNL …` for the user to paste locally.
4. `WaitForLogin` — waits for `az login` to complete with a configurable timeout (default 180s).
5. `AssertAccount` — verifies tenant (by domain or GUID) and optionally the expected user.

### Configuration model

- `~/.azure-profiles/azrl.conf` — global: `BROWSER_CMD` (required — the command that opens a URL on the browser machine), `BROWSER_HOST` (optional — SSH name of the machine running the browser; unset/`localhost`/`127.0.0.1` = local mode, no SSH bridge), `VM_SSH_HOST` (optional — this VM's SSH name, used only in the path-A paste line; auto-derived from `$SSH_CONNECTION` when unset), `PROVIDERS`, `DASHBOARD_POLL_SECS` (optional, default 3). The old keys `LOCAL_BROWSER_CMD`/`LOCAL_HOST`/`VM_HOST` are still read as aliases (new key wins if both present). The `AZRL_BROWSER_CMD` env var overrides `BROWSER_CMD` per process. `config.IsLocal()` is true iff `BROWSER_HOST` ∈ {"", localhost, 127.0.0.1} **and** `VM_SSH_HOST` is unset. `azrl setup` (Bubble Tea wizard via `internal/envdetect` + `internal/ui/setup.go`; `--yes` writes the recommended config non-interactively, `--print` shows it without writing) detects local (WSL→wslview / macOS→open / Linux-desktop→xdg-open) vs remote (SSH session) and writes this file, backing up any existing one to `azrl.conf.bak`; the installers call `azrl setup --yes`. Commands needing global config route through `loadGlobalOrSetup`, which nudges the user to run `azrl setup` (auto-launching the wizard on a TTY) when the config is missing, placeholder (`my-laptop`/`my-vm`), or invalid.
- `~/.<provider>-profiles/mappings` — per-provider TSV index of directory → profile mappings (`<abs-dir>\t<profile>\t<source>`, source `pointer` or `gitconfig`). Auto-managed: written on use/login/capture/`Touch` (and github `SetupRepo`), pruned + revalidated on read, self-healed when azrl first sees a hand-made pointer — not hand-edited.
- `~/.azure-profiles/<profile>.conf` — per-profile: `AZ_TENANT` (required), `AZ_TENANT_ID` (GUID — required for guest/B2B where `tenantDefaultDomain` is null), `AZ_DEFAULT_SUB`, `AZ_EXPECT_USER`, `AZ_BROWSER_CMD` (optional; local browser command overriding the global `BROWSER_CMD`, e.g. `google-chrome --profile-directory="Profile 2"`), `AZ_BROWSER_LABEL` (display label set by `azrl browser`/the TUI picker). `LAST_USED`/`LAST_DIR` are auto-managed (bumped on use/login/capture) — not hand-edited.
- `<repo>/.azprofile` — one line naming the profile; resolved by walking up from `$PWD`. Globally gitignored (never commit it).
- `~/.azure-profiles/<profile>/` — isolated `AZURE_CONFIG_DIR` per profile.
- `~/.github-profiles/<profile>.conf` — per-profile GitHub: `GH_HOST` (required), `GH_USER`, `GH_LABEL`, `GH_PROTOCOL`, `GH_BROWSER_CMD` (optional, same override as `AZ_BROWSER_CMD`), `GH_BROWSER_LABEL` (same as `AZ_BROWSER_LABEL`).
- `<repo>/.ghprofile` — one line naming the GitHub profile; resolved the same way. Globally gitignored.
- `~/.github-profiles/<profile>/` — isolated `GH_CONFIG_DIR` per profile (its own `hosts.yml`/token; requires `gh auth login --insecure-storage`).
- `~/.aws-profiles/<profile>.conf` — per-profile AWS: `AWS_SSO_START_URL`, `AWS_SSO_REGION`, `AWS_ACCOUNT_ID`, `AWS_ROLE_NAME`, `AWS_EXPECT_ACCOUNT`, `AWS_EXPECT_ARN`, `AWS_LABEL`, `AWS_ISOLATE`, `AWS_BROWSER_CMD` (optional, same override), `AWS_BROWSER_LABEL` (same as `AZ_BROWSER_LABEL`). `LAST_USED`/`LAST_DIR` are auto-managed.
- `<repo>/.awsprofile` — one line naming the AWS profile; resolved the same way. Globally gitignored. The `.envrc` exports `AWS_PROFILE=<name>` (default) or the isolate file-path vars.
- `~/.aws-profiles/<profile>/` — isolated `AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE` per profile (only under `--isolate`).
- `~/.gcp-profiles/<profile>.conf` — per-profile GCP: `GCP_CONFIG_NAME` (named gcloud configuration; defaults to the profile name), `GCP_PROJECT` (required), `GCP_REGION`, `GCP_EXPECT_ACCOUNT`, `GCP_LABEL`, `GCP_ISOLATE`, `GCP_BROWSER_CMD` (optional, same override), `GCP_BROWSER_LABEL` (same as `AZ_BROWSER_LABEL`). `LAST_USED`/`LAST_DIR` are auto-managed.
- `<repo>/.gcpprofile` — one line naming the GCP profile; resolved the same way. Globally gitignored. The `.envrc` exports `CLOUDSDK_ACTIVE_CONFIG_NAME=<name>` (default) or `CLOUDSDK_CONFIG=~/.gcp-profiles/<name>` (under `--isolate`).
- `~/.gcp-profiles/<profile>/` — isolated `CLOUDSDK_CONFIG` per profile (only under `--isolate`).

Known limitation: Git Credential Manager prompts triggered by a plain `git push` run outside any `azrl` login, so they fall back to the global `BROWSER_CMD` unless `AZRL_BROWSER_CMD` is exported in the shell.

Identity model: the ambient default is a **read-only mirror** of the provider's
native state (never mutated — PAT-002); **mappings are the opt-in** that earns
tracking (drift/expiry/guardrails); `capture` is metadata-only. Ambient-backed
profiles were considered and rejected — see `docs/ambient-identity-model.md`
before proposing features that manage the native default.

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
`status-dashboard.*` shipped as Phase 5.5; `multi-cloud-providers.*` shipped
as Phases 8–9 — the AWS and GCP providers above; `resolution-strategies.*`
shipped as v0.7.0, amended by #79 which added expiry to MAPPINGS rows and an
expired-governing-pin dashboard hint). `context/` holds domain knowledge and
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
