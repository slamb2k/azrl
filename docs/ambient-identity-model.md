# The Ambient Identity Model

**Status:** Accepted (2026-07-03)
**Context:** [`specs/resolution-strategies.md`](../specs/resolution-strategies.md)
§1–3 and §7 (v0.7.0). This note records the identity-model definitions and the
ownership boundary settled after v0.7.0 shipped, including the decision *not*
to build "ambient-backed profiles."

## Definitions

- **Ambient identity** — the account a provider CLI would use *right now* with
  no azrl involvement: whatever its native default state plus the process
  environment selects. azrl **mirrors** it (the dashboard's AMBIENT rows and
  `azrl status`) and never mutates it (PAT-002). Its definition — which
  account is "the default" — is controlled entirely outside azrl (`gh auth
  switch`, `az login`, `AWS_PROFILE`, gcloud's `active_config`), and it is of
  informational value only.
- **Managed / unmanaged** — an *onboarding marker*, not a health state. An
  ambient identity is *managed* when some saved profile holds the same
  identity: every azrl capability (bridge re-login, guardrails, pinning,
  expiry tracking) is one command away for it. *Unmanaged* means azrl can do
  nothing for that identity — which is a **legitimate steady state** for an
  account you never intend to pin or protect.
- **Mapping is the opt-in.** Binding a directory to a profile is the act that
  declares "I care about this identity here," and it is what earns tracking:
  drift markers, expiry warnings, and guardrail assertions attach to mapped
  profiles. The default identity is going to be there whether azrl cares
  about it or not; a mapping is where caring starts.
- **`capture` is the only verb aimed at the default** — and it is a
  *metadata-only takeover*: it records the identity's conf (tenant, expected
  user, …) and a pointer, never tokens. A captured profile holds no session
  until you `login` it. Capture uses the default as raw material to mint
  something azrl can make promises about; it does not manage the default.

## Decision: the ownership boundary

azrl owns **mappings, guardrails, isolation, the browser bridge, and the
mirror**. The native default pointer is user-and-provider territory. azrl
gains no `default` verb, no `use --home` flag, no GCP auto-activation, no
GitHub env wiring, and no nagging about unmanaged ambient identities.

The reasoning: azrl's guarantees are trustworthy because it only makes claims
about state it owns. A mapped directory's pointer stays true until azrl
changes it. The global default is legitimately mutated by anything — another
CLI, an editor extension, a setup script — so expectations recorded against it
would rot constantly, and noisy guardrails train users to ignore guardrails.

## Rejected alternatives

- **Ambient-backed profiles** — a profile mode delegating token management to
  the global session while keeping guardrails/mappings. Rejected as YAGNI:
  capture is already metadata-only, AWS/GCP profiles are already native
  entries, and the root-mapping pattern (P2 below) expresses "a managed
  default" with existing machinery. Building it would give azrl custody of
  the one thing it deliberately only mirrors.
- **`use --home` convenience flag** — the pattern is already one
  `cd ~ && azrl use <name>` away; a flag would enshrine P2 as *the* pattern.
- **Symlinking `~/.azure` to a profile dir** — collapses the mirror/managed
  distinction (the mirror would report azrl's own state as ambient) and
  breaks CleanSlate's assumption that an isolated dir is disposable.

## The pattern language

How a native default coexists with managed profiles. All patterns compose
existing verbs; none require new azrl features.

| # | Pattern | Fits when | azrl involvement |
|---|---------|-----------|------------------|
| P1 | Empty default | Multi-account work where a bare CLI acting as *anyone* is a risk | none — document |
| P2 | Root mapping | One primary account that should apply everywhere, tracked | none — mechanics exist |
| P3 | Capture-then-login split | (avoid) | none — explain the cost |
| P4 | Unmanaged primary | Primary is stable; azrl manages only secondaries | none — legitimate |
| P5 | Shell-rc default | P2 without direnv | none — caveat visibility |
| P6 | Native default → managed profile | AWS/GCP zero-duplication | none — PAT-002 |

- **P1 — Empty default.** Never sign in globally; every session lives in a
  profile. AMBIENT shows nothing, bare CLIs in unmapped directories error
  instead of acting as the wrong account — fail-safe by construction.
- **P2 — Root mapping.** `cd ~ && azrl use <name>` (plus the offered
  `.envrc`). The pointer walk-up runs to the filesystem root, so the profile
  becomes the effective identity everywhere beneath `$HOME`; deeper pins
  override (nearest wins); drift detection stays quiet at home and correctly
  fires on divergent deeper pins. You have not managed *the* default — the
  global config is untouched — you have **superseded it with a mapping**,
  which is the opt-in principle applied to the fallback case. Covers
  `az`/`aws`/`gcloud`; **not `gh`** (see below).
- **P3 — Capture-then-login split.** Keeping a live global session *and*
  logging in a profile for the same account means two token caches with two
  expiry clocks. Nothing breaks, but the dashboard's expiry describes azrl's
  copy, not your global session. A consequence to be aware of, not a pattern
  to seek: prefer P1/P2 (let the global session lapse) or P4 (don't capture).
- **P4 — Unmanaged primary.** Leave your primary account purely ambient;
  capture only the secondary accounts you pin to client/work directories.
  The primary's AMBIENT row reads *unmanaged* forever — that is information,
  not a problem.
- **P5 — Shell-rc default.** Export the selector in your shell rc
  (`AZURE_CONFIG_DIR=…`, `AWS_PROFILE=…`, `CLOUDSDK_ACTIVE_CONFIG_NAME=…`)
  instead of using direnv. Same effect as P2, but invisible to azrl's
  mappings index — no drift or scope tracking for it.
- **P6 — Native default selects the managed profile (AWS/GCP).** azrl's AWS
  profiles *are* native named profiles in `~/.aws/config`, and its GCP
  profiles *are* native configurations (created `--no-activate`). So
  `export AWS_PROFILE=<name>` in your shell rc, or
  `gcloud config configurations activate <name>`, makes the managed profile
  the native default with **zero duplication** — endorsed, with the same
  visibility caveat as P5: azrl mirrors the result but does not track it.
  azrl never performs the activation itself (PAT-002).

**GitHub is the exception throughout.** `gh` has no environment-based
selection azrl could piggyback on for P2/P5/P6; its per-directory mechanism
is repo-local git credential config (which azrl wires via `use`), and its
native default is `gh auth switch` — native multi-account territory, outside
azrl by design.

## Recommended ladder

Start at **P4**: leave your primary account unmanaged; capture nothing until
you need pinning, guardrails, or bridge re-login for an identity. When you
want your default to live under azrl's tracking, promote it to **P2**: capture
it, then map it at `$HOME`. Use **P6** on AWS/GCP if you want the native
default and the managed profile to be literally the same entry.

## Future (recorded, not planned)

- `rm` warning when an AWS/GCP profile being removed is currently the native
  default / active configuration (stranded native pointer).
- A distinct dashboard scope marker for a `$HOME` mapping, if P2 sees real use.
