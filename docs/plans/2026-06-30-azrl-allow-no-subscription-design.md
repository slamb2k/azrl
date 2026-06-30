# azrl: always pass `--allow-no-subscription` — Design

Date: 2026-06-30

## Goal

Let `azrl` sign in to tenants that have **no Azure subscription** (Entra-ID-only
tenants, or where the user is a guest/member with no sub — e.g.
`fernebuilt.com.au`). Today `az login` errors with *"No subscriptions found
for ..."* and aborts, because `azrl` never passes `--allow-no-subscription`.

## Decision

Pass `--allow-no-subscription` **unconditionally** on every `az login` (zero
config). When subscriptions exist, `az` behaves identically (the default
subscription is still selected); when none exist, login succeeds and
`az account show` returns a tenant-level account instead of failing. This
covers the profile, tenant-less, `--init`, and bare flows uniformly.

Rejected alternative: a per-profile `AZ_ALLOW_NO_SUB=1` flag. More config
surface for no benefit — the always-on form has no downside for sub-having
tenants and also fixes the tenant-less/`--init` paths without extra wiring.

## Change

Single line in `azrl_login_capture` (`azrl-lib.sh`, the `az login`
invocation):

```bash
az login ${tenant_args[@]+"${tenant_args[@]}"} --allow-no-subscription --only-show-errors >/dev/null 2>&1 &
```

## Why nothing else changes

- Subscription-select is already gated on `AZ_DEFAULT_SUB` being non-empty
  (`azrl` orchestrator) → a sub-less profile skips `az account set`.
- `azrl_assert_account` verifies tenant (domain or GUID) + optional user
  only → still passes for a tenant-level account.
- `azrl_save_conf` reads `.id` for `AZ_DEFAULT_SUB`; on a sub-less tenant it
  records the tenant-level placeholder id (or empty) — harmless; `--save`/
  `--init` still write the conf.

## Testing (TDD)

Extend the existing PATH-shimmed `azrl_login_capture` tests (which capture the
`az` argv to a log) to assert the invocation includes `--allow-no-subscription`
on both the with-tenant and tenant-less paths. Use the effective assertion
form (`run grep ...; [ "$status" -eq 0 ]`) — never a bare `! grep`/`! cmd`
(inert under bats). Full suite + `shellcheck` stay green.

## Docs

One-line README note: azrl signs in even to tenants with no Azure
subscription (tenant-level / Entra-only).

## Scope

Capability only. Creating the `fernebuilt` profile (a live `azrl --init
fernebuilt`) is a separate follow-up.
