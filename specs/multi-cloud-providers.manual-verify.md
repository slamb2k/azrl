# Multi-Cloud Providers (Phase 8: AWS) — Manual Verification

- **Date:** 2026-07-01
- **Purpose:** Items implemented and unit/contract/shimmed-integration-tested
  here, but whose end-to-end behaviour can only be *closed out* on a real laptop
  + remote VM with a live AWS IAM Identity Center (SSO) tenant and the real
  `aws` CLI v2. Nothing below is claimed "done" — each is a scripted spike for
  the user to run in the real environment.
- **Related:** `specs/multi-cloud-providers.md`, `specs/multi-cloud-providers.goal.md`.

## What IS verified here (against fakes)

- The AWS `Provider` passes the shared `internal/provider/providertest.RunContract`
  suite **unchanged**, including the no-network proof (fake `aws`/`gcloud` that
  `exit 1` and touch a sentinel the test asserts is never created).
- `internal/aws` unit tests: conf round-trip + order-preserving `AWS_ISOLATE`
  persistence; `Status` identity/expiry from `~/.aws/sso/cache/*.json` filtered
  by `startUrl`; drift for both the shared (`AWS_PROFILE`) and isolated
  (`AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE`) models; `AssertAccount`
  (account exact-match + `AWSReservedSSO_<permset>` role-prefix match against the
  live `_<hash>` suffix); idempotent `SyncConfig` that never overwrites an
  existing `[profile <name>]`; `EnvrcContent`/`WriteEnvrc` for both models.
- `Login` is tested against shimmed `aws` + `ssh` on `PATH`: it starts
  `aws sso login --profile <name>` with the `BROWSER=<self> __browser-capture`
  shim, captures the authorize URL, parses the loopback callback port, and drives
  `bridge.Bridge` (path B reverse tunnel / path A paste) — covering both the PKCE
  loopback and the `--use-device-code` invocation shapes, plus `--isolate`
  scoping the config/credentials env.
- `browsercapture.Classify` / `ParseCallbackPort` classify the AWS PKCE
  `redirect_uri=http%3A%2F%2F127.0.0.1%3A<port>%2Foauth%2Fcallback` shape as
  Loopback (port extracted) and the device shape as Device — confirmed with no
  production change to `classify.go`.
- `cmd/aws.go` (`azrl aws login/list/use/rm/capture/status`) and the tabbed TUI
  AWS view (list + Sign in / Use here / New profile / Remove; no Switch) are
  unit-tested; the tab container maps views by provider **name**, so AWS slotting
  in alphabetically (AWS | Azure | GitHub) keeps every tab wired to its view.

## Items requiring the real laptop + VM + live SSO tenant (spike)

### 0. Does `$BROWSER` fire the `__browser-capture` shim for `aws sso login`?

**Why manual:** the fakes call `$BROWSER` explicitly; the real `aws` CLI v2 may
open the browser via its own mechanism (Python `webbrowser`, or a direct
`xdg-open`) rather than honouring `$BROWSER`.

**Repro (on the VM):**
```bash
azrl aws login work --sso-start-url https://<org>.awsapps.com/start --sso-region us-east-1
```
**Pass:** the sign-in page opens on the LOCAL machine (the shim relayed it).
**If it fails:** the CLI ignored `$BROWSER` — fall back to the `xdg-open` shadow
(`browsercapture.XdgOpenShimScript`, as used for GCM) prepended to `PATH` for the
`aws` child, mirroring the GitHub path.

### 1. Exact PKCE `redirect_uri` shape

**Why manual:** the port tunnel depends on the real callback host/port/path.

**Repro:** capture the URL the CLI opens (from the shim's capture file / logs).
**Pass:** it matches `http://127.0.0.1:<random-port>/oauth/callback` (or a shape
`ParseCallbackPort` already handles). Record the real string here; adjust the
regex only if the real shape differs.

### 2. `--use-device-code` classifiable and usable

**Why manual:** device flow has no loopback port; it prints a verification URL +
user code.

**Repro:**
```bash
azrl aws login work --use-device-code
```
**Pass:** the verification URL relays to the local browser; sign-in completes
without a tunnel. Confirm the captured URL classifies as Device (no
`redirect_uri`).

### 3. Isolation (`AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE`) vs shared-file coexistence

**Why manual:** confirms two AWS accounts can be signed in simultaneously without
their SSO token caches / credentials colliding, and that the `.envrc` for an
isolated profile makes plain `aws` follow it.

**Repro:**
```bash
azrl aws login prod  --isolate --sso-start-url https://prod.awsapps.com/start  --sso-region us-east-1
azrl aws login stage --isolate --sso-start-url https://stage.awsapps.com/start --sso-region us-east-1
cd <repo pinned with `azrl aws use prod --isolate`> && direnv allow
aws sts get-caller-identity   # expect the prod account
```
**Pass:** each isolated profile keeps its own `config`/`credentials` under
`~/.aws-profiles/<name>/`; switching directories switches the account with no
cross-talk; `azrl status` shows the AWS rows with correct identity/expiry and the
drift marker only when the ambient env diverges from the pin. Also confirm the
shared (non-isolate) model coexists via `AWS_PROFILE` when both profiles live in
`~/.aws/config`.

---

# GCP (Phase 9) — Manual Verification

- **Date:** 2026-07-01
- **Purpose:** Items implemented and unit/contract/shimmed-integration-tested
  here, but whose end-to-end behaviour can only be *closed out* on a real laptop
  + remote VM with a live Google account and the real `gcloud` CLI (Cloud SDK).
  Nothing below is claimed "done" — each is a scripted spike for the user to run
  in the real environment.
- **Related:** `specs/multi-cloud-providers.md` (§A2, §Spike), `.tmp/phase-9-gcp.md`.

## What IS verified here (against fakes)

- The GCP `Provider` passes the shared `internal/provider/providertest.RunContract`
  suite **unchanged**, including the no-network proof (fake `gcloud` that
  `exit 1` and touches a sentinel the test asserts is never created).
- `internal/gcp` unit tests: conf round-trip + order-preserving `GCP_ISOLATE`
  persistence; disk-only `Status` — Identity from
  `<config-dir>/configurations/config_<name>` INI (`[core] account`), drift for
  both the default (`CLOUDSDK_ACTIVE_CONFIG_NAME` / `active_config` file) and
  isolated (`CLOUDSDK_CONFIG`) models, and `Expiry == nil` (v1: no SQLite read of
  `access_tokens.db`); `AssertAccount` (exact account-email match, empty
  expectation skips); idempotent `SyncConfig` that creates the named
  configuration (ignoring "already exists") and binds project/compute-region;
  `EnvrcContent`/`WriteEnvrc` for both models; the pure-logic GKE isolation
  warning (fires only when `GCP_ISOLATE=1` AND the plugin is on `PATH` OR a GKE
  kubeconfig context is present — silent otherwise, matching the probe env).
- `Login` is tested against shimmed `gcloud` + `ssh` on `PATH`: it starts
  `gcloud auth login` with the `BROWSER=<self> __browser-capture` shim, captures
  the authorize URL, parses the loopback callback port, and drives `bridge.Bridge`
  (path B reverse tunnel / path A paste) — covering the default (`localhost:8085`)
  loopback and the `--isolate` (`CLOUDSDK_CONFIG`) vs default
  (`CLOUDSDK_ACTIVE_CONFIG_NAME`) scoping. The bridge/browsercapture code is reused
  unmodified.
- `cmd/gcp.go` (`azrl gcp login/list/use/rm/capture/status`) is unit-tested,
  including the wrong-account guardrail: after `gcp.Login`, `login` calls
  `gcp.ActiveAccount` + `gcp.AssertAccount` when `GCP_EXPECT_ACCOUNT` is set and
  **rejects a mismatched account without running Touch**. The tabbed TUI GCP view
  (list + Sign in / Use here / New profile / Remove; no Switch) is unit-tested;
  the tab container maps views by provider **name**, so GCP slotting in
  alphabetically (AWS | Azure | GCP | GitHub) keeps every tab wired to its view.

## Items requiring the real laptop + VM + live Google login (spike)

### 0. Does `$BROWSER` fire the `__browser-capture` shim for `gcloud auth login`?

**Why manual:** the fakes call `$BROWSER` explicitly; the real `gcloud` opens the
browser via Python's `webbrowser` module (which *should* honour `$BROWSER`) or a
direct `xdg-open` — only a real login closes which one fires.

**Repro (on the VM):**
```bash
azrl gcp login work --project <your-project>
```
**Pass:** the sign-in page opens on the LOCAL machine (the shim relayed it).
**If it fails:** `gcloud` ignored `$BROWSER` — fall back to the `xdg-open` shadow
(`browsercapture.XdgOpenShimScript`, as used for GCM) prepended to `PATH` for the
`gcloud` child, mirroring the GitHub/AWS path.

### 1. End-to-end loopback tunnel (`localhost:8085`)

**Why manual:** the tunnel depends on the real callback host/port that `gcloud`
binds for its OAuth `redirect_uri`.

**Repro:** capture the URL the CLI opens (from the shim's capture file / logs).
**Pass:** it matches `http://localhost:8085/` (a shape `ParseCallbackPort` already
handles) and the callback tunnels home over the reverse SSH bridge so sign-in
completes with **only a laptop browser — no laptop `gcloud` install** (the whole
point of replacing `--no-browser`). Record the real string here; adjust the regex
only if the real port/shape differs.

### 2. Simultaneous multi-configuration coexistence + `--isolate`

**Why manual:** confirms two gcloud configurations can hold live credentials
without colliding, and that the `.envrc` selector flips the active account.

**Repro:**
```bash
azrl gcp login prod  --project prod-proj  --region us-central1
azrl gcp login stage --project stage-proj --region us-central1 --isolate
cd <repo pinned with `azrl gcp use prod`> && direnv allow
gcloud auth list --filter=status:ACTIVE --format='value(account)'  # expect prod
```
**Pass:** named configurations coexist in the shared `~/.config/gcloud`; an
`--isolate` profile keeps its own `CLOUDSDK_CONFIG` dir under
`~/.gcp-profiles/<name>/`; switching directories (`CLOUDSDK_ACTIVE_CONFIG_NAME`)
switches the account with no cross-talk; `azrl status` shows the GCP rows with the
correct identity and the drift marker only when the ambient env diverges from the
pin (Expiry stays blank in v1).

### 3. GKE isolation warning end-to-end

**Why manual:** the warning is pure-logic here; the real trigger needs the
`gke-gcloud-auth-plugin` installed and/or a GKE kubeconfig context, plus the
downstream footgun (kubernetes/cloud-provider-gcp#554: the plugin ignores
`CLOUDSDK_CONFIG`).

**Repro:**
```bash
gcloud components install gke-gcloud-auth-plugin
gcloud container clusters get-credentials <cluster> --region <region>
azrl gcp use prod --isolate
```
**Pass:** `azrl gcp use --isolate` prints the GKE isolation warning, and the
dashboard drift column reflects the risk. Confirm the warning stays silent when
neither the plugin nor a GKE context is present.

---

# Resolution Strategies (v0.7.0) — Manual Verification

- **Date:** 2026-07-02
- **Purpose:** Items implemented and unit/contract-tested against fixtures here,
  but whose end-to-end behaviour can only be *closed out* on a real machine with
  the real `gh` CLI and native config dirs. Nothing below is claimed "done" —
  each is a scripted spike for the user to run in the real environment.
- **Related:** `specs/resolution-strategies.md`.

## Items requiring a real machine (spike)

### 0. Real `hosts.yml` multi-account shape matches the fixture (EXT-001)

**Why manual:** the ambient reader parses both the legacy single-`user` shape
and the modern multi-account shape (top-level `user:` / `users:` map) from
fixtures; the spec flags (EXT-001) that only a real `gh` install proves the
fixture assumption against what current `gh` actually writes.

**Repro (on a machine with `gh` signed into two accounts on one host):**
```bash
gh auth login   # second account on github.com
cat ~/.config/gh/hosts.yml
azrl status
```
**Pass:** the real `hosts.yml` matches one of the two fixture shapes, and the
AMBIENT GitHub row shows the active `user@host`. Record the real YAML shape here;
adjust the reader only if it differs.

### 1. Ambient rows live-update on `gh auth switch`

**Why manual:** the fsnotify watch on the native `~/.config/gh` dir is tested
against synthetic writes; only the real `gh` closes whether its rewrite of
`hosts.yml` (rename vs in-place) fires the watcher.

**Repro (with the TUI open on the landing view):**
```bash
azrl            # leave it on the landing tab
gh auth switch  # in another terminal
```
**Pass:** the AMBIENT GitHub row flips to the new `user@host` (and its matched
profile, if managed) without pressing `[r]`, within the poll interval at worst.

### 2. `[a]dopt` handoff launches capture from the landing view

**Why manual:** the adopt keybinding and the capture argv are unit-tested; the
real handoff (TUI exit → interactive capture prompt → new profile + mapping row)
needs a live signed-in CLI session to record.

**Repro (in a repo whose git config names an account azrl doesn't manage):**
```bash
cd <repo with `credential.https://github.com.username` set, no profile>
azrl            # landing view shows the row as unmanaged; press [a]
```
**Pass:** the capture flow launches for the right provider with the name prompt
defaulting to the directory name; accepting it creates the profile, and the
landing view re-renders the row as managed (mapping recorded in the index).

## TUI visual acceptance (v0.9.0 – v0.26.0)

The unit suite pins content and state machines but cannot see colour,
width, or overlay compositing. One real-terminal pass over:

- [ ] **Why:** selection blocks are pure background ANSI. **Repro:** walk tab bar → PROFILES → ACTIONS with `↑`/`→`. **Pass:** exactly one bright-azure block at a time; ancestors dim to deep-blue; descendants show nothing; contrast readable in your theme.
- [ ] **Why:** the five-tier icon ramp is colour-only in places. **Repro:** view a tab with cwd-pinned, parent-pinned, default, elsewhere-mapped, and unmapped profiles. **Pass:** green/orange/🌐/dark-white/deep-grey all distinguishable; legend centered at the pane bottom.
- [ ] **Why:** the options popup splices into the background with ANSI-aware truncation. **Repro:** press `o` on a busy dashboard. **Pass:** box centered, background visible around it, no colour bleed at the left/right seams.
- [ ] **Why:** the fuzzy dir picker walks the real filesystem. **Repro:** `d`, type fragments of a deep project path. **Pass:** sensible ranking, `~` display, enter changes every tab's 📁 header.
- [ ] **Why:** keycap chips and header glyphs (📁/👤/🌐/provider icons) are emoji/width-sensitive. **Repro:** resize to ~80 cols. **Pass:** footers truncate without frame overflow; header stays on one line; no misaligned columns.
- [ ] **Why:** pin-on-create now spans providers. **Repro:** `azrl aws login <new>` (or gcp/gh) in a scratch dir on a real tenant. **Pass:** browser pops via the bridge, profile created, pointer written, envrc offered (aws/gcp) or credentials wired (gh).
