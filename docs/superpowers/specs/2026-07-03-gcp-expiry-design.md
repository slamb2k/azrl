# GCP Token Expiry — Design

**Date:** 2026-07-03
**Status:** Approved (brainstormed interactively; dependency choice confirmed by user)
**Follows:** #79 (mapped-row expiry surfacing), specs/multi-cloud-providers.md (GCP v1
shipped `Expiry: nil` as a documented gap)

## Problem

GCP is the only expiring-credential provider whose `Status.Expiry` is always nil, so
none of the expiry UX (DETAILS pane countdown, `⚠ expired` mapping annotations from
#79, the expired-governing-pin dashboard hint, `status --json` timestamps) applies to
GCP profiles. gcloud caches token expiry in `access_tokens.db`, a SQLite file, which
the disk-only `Status` contract could not read without a new dependency — the v1 gap.

## Decision

Read `access_tokens.db` with **`github.com/alicebob/sqlittle`** (user-confirmed):
a tiny, MIT-licensed, pure-Go, read-only SQLite *file* reader. Dormant since 2020,
but the SQLite3 file format is frozen and gcloud opens the DB with no pragmas —
rollback-journal mode, which sqlittle handles; it errors on WAL, which gcloud never
uses. Negligible binary impact. Rejected alternatives: `modernc.org/sqlite`
(adds ~5–9 MB to a ~10 MB binary for one SELECT), a hand-rolled b-tree parser
(overflow pages, varints, legacy short rows — fragile code to own), and keeping the
gap (loses expiry parity for no reason once a safe reader exists).

## Verified facts (gcloud SDK source, `googlecloudsdk/core/credentials/creds.py`)

- Table: `access_tokens (account_id TEXT PRIMARY KEY, access_token TEXT,
  token_expiry TIMESTAMP, rapt_token TEXT, id_token TEXT,
  regional_access_boundary TEXT, regional_access_boundary_expiry TIMESTAMP)`.
  The trailing columns arrived via `ALTER TABLE`; old DBs have short rows.
- `token_expiry` is Python `datetime.isoformat(' ')`: naive UTC
  `YYYY-MM-DD HH:MM:SS[.ffffff]`, no timezone suffix. May be NULL.
- No pragmas issued → rollback journal (not WAL) → a cold read of the main file
  sees the latest committed state.
- Storage is plaintext on all platforms; no encrypted-credential rollout exists.
- `credentials.db` and `application_default_credentials.json` carry no expiry.

## Semantics

Mirror Azure exactly. `Expiry` is the cached access token's expiry: best-effort,
display-only, never gating. A past timestamp renders `expired`, re-offers the
Sign in action, and (post-#79) raises the governing-pin hint.

**Accepted caveat:** gcloud access tokens live ~1 hour and refresh silently from
`credentials.db` on next use, so a GCP profile reads `expired` an hour after its
last command even though the next `gcloud` call succeeds without a browser. Azure
behaves identically today (MSAL access tokens are also ~1 h) and
specs/status-dashboard.md explicitly blesses "render `expired`; refreshing validity
is a drill-through concern". Consistency wins over suppressing the signal.

## Implementation

One new function in `internal/gcp/status.go`:

```go
// gcpExpiry reads the cached access-token expiry for account from
// <gcDir>/access_tokens.db (SQLite, read via sqlittle); nil on any error,
// missing row, or NULL/unparseable token_expiry.
func gcpExpiry(gcDir, account string) *time.Time
```

- Called from `Status` as `gcpExpiry(gcloudConfigDir(name, confdir, c.Isolate),
  identity)` — default and `--isolate` modes both covered by the existing dir
  resolution. Blank identity → nil without opening the DB.
- Selects `account_id, token_expiry` from `access_tokens`; exact string match on
  the account email (the profile's `[core] account`, unlike Azure's max-across-all-
  tokens, because the row is per-account).
- Parses both layouts (`2006-01-02 15:04:05` and `2006-01-02 15:04:05.999999`) as
  UTC.
- `access_tokens.db` joins the `LatestMtime` list so external token refreshes
  re-sort the dashboard, matching Azure's token-cache-mtime behaviour.
- The stale "cannot be read disk-only without a new dependency" comment is removed.
- `go.mod` gains `github.com/alicebob/sqlittle` (MIT).

No UI, cmd, or provider-interface changes: everything downstream keys off
`Expiry != nil`.

## Error handling

`gcpExpiry` returns nil and never errors for: absent file (fresh machine), sqlittle
open/select failure (corrupt file, hot journal, future format change), no row for
the account, NULL `token_expiry`, unparseable value, legacy short rows. Nil is
today's shipped behaviour, so every failure mode degrades to the status quo,
honouring the per-profile fault-isolation rule in specs/status-dashboard.md.

## Testing

TDD-first, following the existing `internal/gcp` patterns:

- Committed binary fixture `internal/gcp/testdata/access_tokens.db` (generated once
  with the `sqlite3` CLI at development time) containing: an account with a past
  expiry (1970), an account with a far-future expiry (2999), and an account with a
  NULL `token_expiry`.
- Unit tests for `gcpExpiry`: expired row, valid row, account-mismatch → nil,
  NULL → nil, absent file → nil, fractional-seconds parsing.
- `Status` integration: seeded profile whose config dir contains the fixture →
  `Expiry` non-nil; without the fixture → nil (current tests keep passing).
- `providertest.RunContract` unchanged and green; `Status` stays no-network/no-CLI.

## Out of scope

- Refresh-token-aware "session validity" (would require networked introspection —
  contradicts the disk-only contract).
- The GKE `gke-gcloud-auth-plugin` isolation gap (separate documented issue).
- Any suppression/tiering of the ~1 h expired signal (revisit only if it proves
  noisy in practice — would be a cross-provider change, not a GCP one).
