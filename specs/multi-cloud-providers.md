# Multi-Cloud Providers (AWS + Google Cloud) — Design Spec

- **Date:** 2026-07-01
- **Status:** Approved (scoped); slots into the provider-aware roadmap as
  **Phases 8–9**. The GitHub work (Phases 1–7) shipped in #17; these continue
  after it.
- **Author:** brainstormed with Claude
- **Related:** extends `specs/github-remote-login.md` (the `Provider` interface,
  parameterized `internal/profile`, shared browser-bridge). Formalizes the README
  roadmap's "More login providers (GitHub, AWS, Google Cloud)" line into real
  phases. Assumes `specs/status-dashboard.md` (Phase 5.5) has landed, so both new
  providers must implement `Status()` from day one.

## Summary

Add two new `Provider` implementations behind the **existing** interface,
following the exact pattern set by Azure and GitHub:

- **`internal/aws`** — AWS IAM Identity Center (`aws sso login`) across multiple
  named profiles.
- **`internal/gcp`** — Google Cloud (`gcloud auth login`) across multiple named
  configurations.

**This is not a redesign.** The `Provider` interface, profile mechanics
(list/resolve/use/remove/relabel + `Status()`), the tabbed TUI, the CLI dispatch,
and the SSH browser-bridge subsystem are already provider-agnostic and get
**reused as-is**. Each new provider is one self-registering package that plugs a
new auth lifecycle behind the interface and passes the shared contract suite
unchanged. The headline win is the same as Azure/GitHub: **run the interactive
browser login from a headless/remote VM, popping the sign-in page on your local
machine** — which for AWS and (especially) GCP is a materially better UX than the
CLIs' own remote-login stories.

## Motivation & key finding

The load-bearing insight from the GitHub phase generalizes: different clouds have
different "the browser must reach a localhost callback on the CLI host" problems,
and azrl's reverse-SSH-tunnel browser-bridge already solves the general case.

- **AWS.** `aws sso login` on AWS CLI **v2.22+ defaults to PKCE**, which starts a
  **local listener** and needs the browser to reach `127.0.0.1:<port>` **on the
  CLI host** — structurally identical to Azure's random OAuth callback port.
  azrl's bridge forwards it unmodified. There is an open upstream AWS CLI issue
  requesting a `--redirect-host` flag for remote `redirect_uri` customization;
  **azrl's browser bridge already solves that problem natively**, so there is no
  upstream dependency to wait on.
- **GCP.** `gcloud`'s remote-login story is *worse* than device code: its
  `--no-browser` mode requires **gcloud installed on the machine with the
  browser** to complete the handshake — a heavy prerequisite for "just pop my
  laptop browser." azrl's reverse-SSH-tunnel bridge **replaces `--no-browser`
  entirely**, so the laptop needs only a browser, not a gcloud install. This is
  the **single highest-value adapter** in this feature — a bigger UX jump than
  the AWS case.

The corollary is that the multi-profile *switching* mechanics for these two
clouds are lighter than Azure's, because both clouds already have native
multi-profile primitives. We lean on those rather than reinventing per-profile
config-dir isolation.

## Goals

- Manage multiple AWS profiles and multiple GCP configurations on a remote VM,
  with fast switching and per-repo pinning — same model as Azure/GitHub.
- Sign in with the browser on the *local* machine, reusing azrl's existing
  bridge subsystem **unmodified** for both `aws sso login` (PKCE loopback) and
  `gcloud auth login` (loopback), replacing `--no-browser`.
- Device-code fallback where the loopback path is blocked (`aws sso login
  --use-device-code`), same interactive-first / device-code-only-when-blocked
  rationale as Azure.
- Wrong-account guardrails (`AssertAccount` equivalents) as a **high priority**,
  because wrong-account destructive cloud commands are a well-known footgun class.
- Both providers pass `providertest.RunContract` (including `Status()`) unchanged.

## Non-goals (v1)

- **Full config-dir isolation as the default** for either cloud (it's opt-in;
  see below). Both clouds' native multi-profile stores coexist safely in shared
  files, so hard directory isolation is unnecessary overhead by default.
- Solving the upstream AWS `--redirect-host` gap (the bridge makes it moot) or
  the upstream `gke-gcloud-auth-plugin` / `CLOUDSDK_CONFIG` bug (we warn, we
  don't patch — see GCP known gap).
- Non-browser auth paths (AWS access keys / static credentials, GCP service-
  account key files) — azrl is about *interactive browser* login.
- SSO/OIDC provider *administration* (creating permission sets, etc.).

## Architecture

Nothing on the interface changes. Both packages implement the current
`internal/provider.Provider` surface (`Name`/`Title`/`ProfilesDir`/`Scheme`/
`ListProfiles`/`Resolve`/`Use`/`Remove`/`SetLabel`/`Status`) plus their own
concrete sign-in orchestration (`Login`, `AssertAccount`, device-code fallback)
that lives on the concrete type, exactly as Azure/GitHub do. They self-register
so the TUI tab container and CLI dispatch pick them up without edits.

The only shared-code work is **A3 (bridge generalization)** — an audit-and-tidy
pass to confirm the bridge/browsercapture packages carry no Azure-specific
assumptions. It is **the first step of the AWS phase (Phase 8)**, done *before*
the AWS provider itself so both new providers build on a confirmed-generic
foundation.

---

## A1. AWS provider (`internal/aws`) — Phase 8

### Profile switching — native `AWS_PROFILE`, driven by `.envrc`

AWS already supports multiple named profiles coexisting safely in a single
`~/.aws/config` + `~/.aws/credentials` pair. The default switching mechanism is
therefore the **`AWS_PROFILE` env var**, wired through the repo's `.envrc` (the
same direnv mechanism azrl already writes for Azure). Pinning a repo to a profile
writes `.awsprofile` (the pointer, walk-up resolved by the shared resolver) and
an `.envrc` line exporting `AWS_PROFILE=<name>`.

**Do NOT build full `AWS_CONFIG_FILE` / `AWS_SHARED_CREDENTIALS_FILE` directory
isolation (à la `AZURE_CONFIG_DIR`) as the default** — AWS profiles already
coexist safely in one file, so per-profile config-dir isolation is unnecessary
overhead and diverges from every AWS user's muscle memory.

### Opt-in full isolation

Spec a **`--isolate` opt-in flag** (per profile, recorded in the conf) that
switches that profile to a dedicated config dir — separate `AWS_CONFIG_FILE`
and `AWS_SHARED_CREDENTIALS_FILE` under `~/.aws-profiles/<name>/` — for users who
want **hard separation between clients** (e.g. consultants who must guarantee two
customers' credentials never share a file). This is the non-default path; the
`.envrc` then exports the two file-path vars instead of `AWS_PROFILE`.

### Login flow — reuse the bridge unmodified

`aws sso login` (CLI v2.22+, PKCE default) starts a local listener and opens a
browser at an authorize URL whose `redirect_uri` is `http://127.0.0.1:<port>`.
This is the **same shape as the Azure case**: parse the port, reuse the existing
`Bridge` (path B reverse `ssh -R`, path A `ssh -fNL` paste fallback) and the
`browsercapture` shim to pop the laptop browser and tunnel the callback home.
**No bridge changes required** — the shim already classifies loopback
`redirect_uri` and tunnels it.

### Fallback — device code

Spec **`aws sso login --use-device-code`** as the Conditional-Access-blocked /
loopback-unavailable fallback, same rationale as azrl's Azure device-code
avoidance: **interactive browser first; device code only when the loopback path
is blocked** (locked-down egress, no reachable local host). The shim's existing
device-vs-loopback classification routes it (device → relay the code/URL to the
laptop; no tunnel).

Note for the spec record: there is an open upstream AWS CLI issue requesting a
`--redirect-host` flag so a remote `aws sso login` can advertise a non-localhost
`redirect_uri`. **azrl's browser bridge already solves this natively** (it
tunnels the localhost callback rather than rewriting it), so azrl carries **no
upstream dependency** on that flag landing.

### Guardrail — `AssertAccount` via `aws sts get-caller-identity`

**High priority.** After login (and available as a standalone assert / drill-in
action), run `aws sts get-caller-identity` under the profile and verify the
resolved account/ARN matches the profile's expectation
(`AWS_EXPECT_ACCOUNT` / `AWS_EXPECT_ARN`). Wrong-account destructive AWS commands
are a well-known footgun class — the same category of problem `aws-vault` and
`aws-whoami` exist to solve — so this guardrail is a first-class part of the
provider, not an afterthought. (This is the one *network* assertion; it is **not**
`Status()`, which stays disk-only per the dashboard spec — `Status()` reads the
SSO token cache under `~/.aws/sso/cache`, `AssertAccount` is the on-demand
network check.)

### Profile / conf model

- **Profile store** — `~/.aws-profiles/<name>.conf`:
  - `AWS_SSO_START_URL` / `AWS_SSO_REGION` — the IAM Identity Center portal.
  - `AWS_ACCOUNT_ID` + `AWS_ROLE_NAME` — the account/role this profile assumes.
  - `AWS_EXPECT_ACCOUNT` / `AWS_EXPECT_ARN` — for post-auth assertion.
  - `AWS_LABEL` — optional display label (reuses the `*`-marker / relabel).
  - `AWS_ISOLATE` — `1` if this profile uses opt-in file isolation.
  - `LAST_USED` — the shared dashboard timestamp.
- **Native profile mapping** — `Use` ensures a matching `[profile <name>]`
  stanza exists in `~/.aws/config` (sso-session wired), then writes `.awsprofile`
  + `.envrc` (`export AWS_PROFILE=<name>`, or the two file-path vars under
  `--isolate`).
- **Repo pin** — `<repo>/.awsprofile` (one line, gitignored), walk-up resolved.
- **`Status()`** (disk-only) — read SSO cached token `expiresAt` from
  `~/.aws/sso/cache` for `Expiry`, account/role for `Identity`, compare ambient
  `AWS_PROFILE` vs pinned for `Drifted`. No `sts` call.

### Contract test

`internal/aws` must pass the full `providertest.RunContract` suite **unchanged**,
including the `Status()` no-network assertion. Sign-in orchestration is tested by
shimming `aws`/`ssh` onto `PATH` with fakes (same pattern as Azure/GitHub).

---

## A2. Google Cloud provider (`internal/gcp`) — Phase 9

### Profile switching — native `gcloud` configurations, driven by `.envrc`

gcloud has a **named-configuration primitive** (`gcloud config configurations
create/activate`) that is *closer to azrl's model than AWS's flat profiles* —
each configuration is an isolated set of account/project/region. The default,
lightweight path drives these with `CLOUDSDK_ACTIVE_CONFIG_NAME` exported via the
repo `.envrc`. Google's own docs recommend exactly this direnv pattern, so we're
riding a supported mechanism, not a hack. Pinning writes `.gcpprofile` +
`.envrc` (`export CLOUDSDK_ACTIVE_CONFIG_NAME=<name>`).

### Opt-in full config-dir override

Spec **`CLOUDSDK_CONFIG`** (full config-directory override, `~/.gcp-profiles/
<name>/`) as **opt-in only**, for **read-only-`$HOME` / container scenarios**
where a writable shared `~/.config/gcloud` isn't available. It is **not** the
default multi-account mechanism — named configurations are.

### Login flow — bridge replaces `--no-browser` (highest-value adapter)

`gcloud auth login`'s remote story, `--no-browser`, requires **gcloud installed
on the machine with the browser** to complete the handshake — worse UX than
device code or Azure's flow. Spec azrl's **reverse-SSH-tunnel browser bridge as a
full replacement for `--no-browser`**: run the normal `gcloud auth login` (which
opens a `127.0.0.1:<port>` loopback), let the shim tunnel the callback, and pop
the laptop browser at the authorize URL. The laptop needs only a browser — no
gcloud install. **This is the highest-value part of the whole feature**, a bigger
UX win than the AWS case, and it uses the existing bridge unmodified.

### Guardrail — `AssertAccount` via active-account check

After login (and as a standalone assert), verify the active account with
`gcloud auth list` / `gcloud config get-value account` under the configuration,
matching the profile's `GCP_EXPECT_ACCOUNT`. Same footgun rationale as AWS. This
is the one network assertion; `Status()` stays disk-only (reads the credential
db / config files directly).

### Known gap to document (not necessarily solved in v1)

`gke-gcloud-auth-plugin` **does not respect `CLOUDSDK_CONFIG`**. So if a user
opts into full config-dir isolation for GCP, `kubectl`/GKE credential resolution
can **silently use the wrong cached account** — a genuinely dangerous
wrong-cluster footgun. **v1 spec: surface a warning** when *both* full isolation
(`CLOUDSDK_CONFIG`) **and** GKE usage are detected (e.g. a `gke-gcloud-auth-
plugin` on `PATH` or a GKE context in kubeconfig), rather than attempting to fix
the upstream bug. The warning fires at `use`/login time and is reflected in the
dashboard drift column for that profile. Solving the upstream bug is out of scope.

### Profile / conf model

- **Profile store** — `~/.gcp-profiles/<name>.conf`:
  - `GCP_CONFIG_NAME` — the gcloud configuration name (default path).
  - `GCP_PROJECT` / `GCP_REGION` — bound project/region.
  - `GCP_EXPECT_ACCOUNT` — for post-auth assertion.
  - `GCP_LABEL` — optional display label.
  - `GCP_ISOLATE` — `1` if this profile uses opt-in `CLOUDSDK_CONFIG`.
  - `LAST_USED` — shared dashboard timestamp.
- **Native config mapping** — `Use` ensures `gcloud config configurations create
  <name>` has run (idempotent), writes `.gcpprofile` + `.envrc`
  (`CLOUDSDK_ACTIVE_CONFIG_NAME`, or `CLOUDSDK_CONFIG` under `--isolate`), and
  emits the GKE warning if applicable.
- **Repo pin** — `<repo>/.gcpprofile` (one line, gitignored), walk-up resolved.
- **`Status()`** (disk-only) — active account from the gcloud config /
  `credentials.db`, cached credential expiry for `Expiry`, ambient
  `CLOUDSDK_ACTIVE_CONFIG_NAME` vs pinned for `Drifted`. No `gcloud auth list`
  call on the timer.

### Contract test

`internal/gcp` must pass `providertest.RunContract` **unchanged**, including the
`Status()` no-network assertion. Sign-in tested by shimming `gcloud`/`ssh` onto
`PATH`.

---

## A3. Shared browser-bridge generalization (Phase 8, step 1)

Positioning: an **audit-and-confirm** pass, not a rewrite, and **the opening
step of the AWS phase** rather than a standalone phase — it's small enough to
fold in, and gates only the loopback-tunnel path both AWS and GCP rely on. The
bridge (`internal/bridge`, reverse `ssh -R` / `ssh -fNL`) and the smart shim
(`internal/browsercapture`, classify loopback-vs-device + relay/tunnel) already
serve Azure + GitHub generically. The claim to **verify and, if needed, tidy** is
that they carry **no Azure-specific assumptions** and already generalize to
arbitrary "open this URL, capture this callback" cases for AWS/GCP.

Concretely, this step must:

- **Confirm URL classification is provider-agnostic** — the shim keys off the
  `redirect_uri` host/port (`127.0.0.1` / `localhost`) and device-endpoint
  shape, not off any Azure/GitHub-specific host or query param. `aws sso login`
  PKCE and `gcloud auth login` loopback URLs must classify as loopback-tunnel
  with **no code change**; verify with captured/synthetic URLs from both.
- **Confirm port parsing is generic** — random callback port parsed from the
  URL, not assumed fixed (already true for GCM; AWS/GCP also use random ports).
- **Confirm the tunnel window / teardown is provider-neutral** — the ~180s
  bounded auth window and no-daemon teardown apply unchanged.
- **Note (don't necessarily do) any refactor** needed to strip lingering
  Azure-specific naming/assumptions from the shared packages, so the code reads
  as provider-agnostic. If the audit finds none, this step is a short
  confirmation + a couple of table-driven classification tests for AWS/GCP URLs;
  if it finds Azure-isms, they're refactored here with all providers green.

Deliverable: the bridge/shim demonstrably handle AWS and GCP loopback callbacks
with no provider-specific branching, proven by classification unit tests over
real AWS/GCP authorize-URL shapes — landed as the first commits of Phase 8,
before the AWS provider itself.

## Spike — verify before implementing (gates the AWS/GCP phases)

Some load-bearing facts should be confirmed in the target environment before
committing feature code (mirrors the GitHub Phase 0 discipline; produce a short
findings note, and flag anything that needs a real laptop+VM to close):

1. **AWS PKCE loopback** — confirm `aws sso login` on the installed CLI version
   (v2.22+) uses PKCE with a `127.0.0.1:<port>` `redirect_uri` the shim can
   parse; confirm `--use-device-code` produces a classifiable device flow.
2. **AWS isolation** — confirm `AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE`
   fully isolate a profile (opt-in path) and that the default `AWS_PROFILE` path
   coexists safely in shared files.
3. **GCP loopback vs `--no-browser`** — confirm plain `gcloud auth login` opens a
   `127.0.0.1:<port>` loopback the bridge can tunnel (so we can avoid
   `--no-browser` and its laptop-side gcloud requirement).
4. **GCP config isolation** — confirm `CLOUDSDK_ACTIVE_CONFIG_NAME` (default) and
   `CLOUDSDK_CONFIG` (opt-in) behave as specified.
5. **GKE gap** — confirm `gke-gcloud-auth-plugin` ignores `CLOUDSDK_CONFIG` so
   the warning trigger is real, and decide the detection heuristic (plugin on
   `PATH` + GKE context present).

## Testing

- **Contract-first, same pattern as Azure/GitHub:** both providers run
  `providertest.RunContract` unchanged (incl. `Status()` no-network assertion).
- **Shimmed integration:** fake `aws`/`gcloud`/`ssh` on `PATH` via
  `t.Setenv("PATH", tmpDir)`; assert the right sub-commands/flags
  (`sso login`, `--use-device-code`, `configurations activate`, `sts
  get-caller-identity`, `auth list`) and the right `ssh -R`/`-fNL` bridge
  invocations per path.
- **Pure logic unit tests:** conf parsing, `.envrc` emission
  (`AWS_PROFILE`/`CLOUDSDK_ACTIVE_CONFIG_NAME` vs isolation vars), URL
  classification for AWS/GCP authorize URLs, GKE-warning trigger.
- **TUI:** the new tabs render via the existing tab container with no view-layer
  changes beyond registration; dashboard rows for AWS/GCP verified by the
  Phase 5.5 model tests once these providers exist.

## Packaging / release

- Two new self-registering packages; the tab container and CLI dispatch pick
  them up with no edits. No new alias entrypoints required (the neutral unified
  binary hosts all four providers); optionally add `awsrl`/`gcprl` aliases later
  for symmetry with `azrl`/`ghrl` if there's demand — deferred.
- No new third-party Go deps expected (shell out to `aws`/`gcloud` like `az`/`gh`).

## Deferred / open questions

- **`awsrl` / `gcprl` alias entrypoints** — symmetry with `azrl`/`ghrl`; deferred
  until requested.
- **AWS non-SSO / static-credential profiles** — out of scope (not browser
  login); revisit only if users ask.
- **GCP Application Default Credentials (ADC)** — `gcloud auth application-
  default login` also uses a loopback and could ride the same bridge; note as a
  likely-trivial follow-on once `gcloud auth login` works.
- **Solving the GKE `/CLOUDSDK_CONFIG` bug upstream** vs the v1 warning —
  warning ships first; upstream fix tracked, not owned.

## Roadmap position

Continues the `specs/github-remote-login.md` roadmap after Phase 7 (shipped, #17):

```
7.  Docs + release (GitHub feature ships).                  (shipped, #17)
8.  AWS provider (internal/aws) —                           (A3 + A1)
    step 1: bridge generalization — audit/confirm bridge +
      shim carry no Azure-isms; AWS/GCP loopback classification tests.
    then: sso login PKCE bridge, device-code fallback,
      AWS_PROFILE/.envrc, opt-in isolation,
      sts get-caller-identity guardrail, Status().
9.  GCP provider (internal/gcp) — auth login bridge          (A2)
    (replaces --no-browser), named configs/.envrc, opt-in
    CLOUDSDK_CONFIG, GKE warning, auth-list guardrail, Status().
```
