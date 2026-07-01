# Goal Prompt — Multi-Cloud Providers (AWS + Google Cloud)

**Spec:** `specs/multi-cloud-providers.md` (read it first; it is authoritative).

## Objective

Add two new `Provider` implementations behind the **existing** interface, in the
exact pattern set by Azure and GitHub:

- **`internal/aws`** — AWS IAM Identity Center (`aws sso login`) across multiple
  `AWS_PROFILE`s.
- **`internal/gcp`** — Google Cloud (`gcloud auth login`) across multiple named
  configurations.

**Not a redesign.** The `Provider` interface, `internal/profile` mechanics
(list/resolve/use/remove/relabel + `Status()`), tabbed TUI, CLI dispatch, and the
SSH browser-bridge (`internal/bridge` + `internal/browsercapture`) are reused
as-is. Each provider is one self-registering package that plugs a new auth
lifecycle behind the interface and passes `providertest.RunContract` unchanged.
The win: run the interactive browser login from a headless VM, popping the
sign-in page on the local machine — a materially better UX than either CLI's own
remote-login story.

## Key facts (design-defining)

- **AWS:** `aws sso login` (CLI v2.22+) defaults to PKCE → a `127.0.0.1:<port>`
  loopback, same shape as Azure. Reuse the bridge **unmodified**. Fallback:
  `aws sso login --use-device-code` when the loopback path is blocked.
- **GCP:** the bridge **replaces `--no-browser`** (which needs gcloud on the
  laptop). Plain `gcloud auth login` opens a loopback the shim tunnels — the
  laptop needs only a browser. This is the highest-value adapter in the feature.
- **Switching stays native:** default to `AWS_PROFILE` (AWS) and
  `CLOUDSDK_ACTIVE_CONFIG_NAME` (GCP) via `.envrc`. **Do NOT** build per-profile
  config-dir isolation as the default — both clouds' profiles coexist safely in
  shared files. Full isolation (`AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE`;
  `CLOUDSDK_CONFIG`) is an **opt-in** flag only.
- **Guardrails are high priority:** `AssertAccount` via
  `aws sts get-caller-identity` and `gcloud auth list` — wrong-account
  destructive commands are the footgun class this removes. These are the only
  network calls; `Status()` stays disk-only.
- **GCP known gap:** `gke-gcloud-auth-plugin` ignores `CLOUDSDK_CONFIG`. When
  full isolation + GKE usage are both detected, **warn** the user (v1); do not
  try to fix the upstream bug.

## Hard gate — spike first (gates AWS/GCP feature code)

Verify in the target environment, produce a short findings note, flag anything
needing a real laptop+VM to close:

1. `aws sso login` PKCE `127.0.0.1:<port>` `redirect_uri` is parseable;
   `--use-device-code` classifies as device flow.
2. `AWS_CONFIG_FILE`/`AWS_SHARED_CREDENTIALS_FILE` isolation works (opt-in);
   default `AWS_PROFILE` coexists safely.
3. Plain `gcloud auth login` opens a tunnelable loopback (so `--no-browser` is
   avoidable).
4. `CLOUDSDK_ACTIVE_CONFIG_NAME` (default) and `CLOUDSDK_CONFIG` (opt-in) behave
   as specified.
5. `gke-gcloud-auth-plugin` ignores `CLOUDSDK_CONFIG` → the warning trigger is
   real; settle the detection heuristic.

## Build phases

8. **AWS provider (A3 + A1)** — `internal/aws`.
   - **Step 1 (bridge generalization, A3):** audit/confirm `internal/bridge` +
     `internal/browsercapture` carry no Azure-specific assumptions; add
     table-driven classification tests over real AWS/GCP authorize-URL shapes;
     refactor out any Azure-isms found, all providers green. Land as the first
     commits, before the provider itself.
   - **Then (A1):** `Provider` + `Status()`, `sso login` PKCE bridge,
     `--use-device-code` fallback, `AWS_PROFILE`/`.envrc`, opt-in file isolation,
     `sts get-caller-identity` guardrail. Passes `RunContract`.
9. **GCP provider (A2)** — `internal/gcp`: `Provider` + `Status()`, `auth login`
   bridge replacing `--no-browser`, named-config/`.envrc`, opt-in
   `CLOUDSDK_CONFIG`, GKE warning, `auth list` guardrail. Passes `RunContract`.

## Constraints

- **Strict TDD** — red/green/refactor; no production code without a failing test
  first. Use `superpowers:test-driven-development`.
- Conventional commits, scope `aws`/`gcp`/`bridge`/`profile` as apt.
- Test pattern: shim `aws`/`gcloud`/`ssh` onto `PATH` with fakes; provider
  contract test (incl. `Status()` no-network); URL-classification unit tests.
- No new third-party Go deps (shell out like `az`/`gh`).
- Do not regress Azure/GitHub or the bridge; interface stays unchanged.

## Acceptance criteria

- `go build ./...`, `go test ./...` green; `gofmt -l .` clean at every phase.
- `internal/aws` and `internal/gcp` both pass `providertest.RunContract`
  unchanged, including `Status()`.
- Bare invocation shows AWS + GCP tabs (and dashboard rows) alongside Azure +
  GitHub; each lists/creates/uses/switches profiles with native switching.
- `aws sso login` and `gcloud auth login` bridge their loopback callbacks to the
  laptop browser (verified per spike capabilities); device-code fallback works
  for AWS.
- Wrong-account assertion fires for both; GCP GKE-isolation warning fires when
  applicable.

## Exit — auto-ship

When the Definition of Done is met and everything is green, **auto-ship** each
provider phase via `mad-skills:ship` — commit/push, PR, wait for CI,
squash-merge, sync `main`. Report merged PR URLs. Skip shipping only if CI can't
be made green or a spike item is a hard blocker.

## Out of scope (v1)

Non-browser auth (AWS static keys, GCP service-account key files); SSO/OIDC admin;
solving the upstream AWS `--redirect-host` or GKE/`CLOUDSDK_CONFIG` bugs;
`awsrl`/`gcprl` alias entrypoints (deferred).
