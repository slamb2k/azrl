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
