#!/usr/bin/env bash
# Pure, sourceable helpers for azlogin. No side effects on source.

azl_extract_port() {
  local url="$1" decoded
  decoded="${url//%3A/:}"; decoded="${decoded//%2F//}"
  printf '%s' "$decoded" | grep -oE 'localhost:[0-9]+' | head -1 | cut -d: -f2
}

azl_resolve_profile() {
  local arg="$1" dir="${2:-$PWD}"
  if [[ -n "$arg" ]]; then printf '%s\n' "$arg"; return 0; fi
  local d="$dir"
  while [[ -n "$d" && "$d" != "/" ]]; do
    if [[ -f "$d/.azprofile" ]]; then
      tr -d '[:space:]' < "$d/.azprofile"; printf '\n'; return 0
    fi
    d="$(dirname "$d")"
  done
  printf 'azlogin: no profile arg and no .azprofile found from %s\n' "$dir" >&2
  return 1
}

azl_load_profile_conf() {
  local profile="$1" confdir="${2:-$HOME/.azure-profiles}"
  local f="$confdir/$profile.conf"
  [[ -f "$f" ]] || { printf 'azlogin: missing config %s\n' "$f" >&2; return 1; }
  # shellcheck disable=SC1090
  source "$f"
  [[ -n "${AZ_TENANT:-}" ]] || { printf 'azlogin: AZ_TENANT not set in %s\n' "$f" >&2; return 1; }
  return 0
}

azl_paste_line() {
  # $1=port $2=vm_host $3=browser_cmd $4=url
  printf 'ssh -fNL %s:localhost:%s %s && %s "%s"\n' "$1" "$1" "$2" "$3" "$4"
}

azl_assert_account() {
  # $1=account_json $2=expected_tenant (GUID or domain) $3=expected_user (optional)
  local json="$1" exp_tenant="$2" exp_user="$3"
  local got_tenant got_domain got_user
  got_tenant="$(printf '%s' "$json" | jq -r '.tenantId // empty')"
  got_domain="$(printf '%s' "$json" | jq -r '.tenantDefaultDomain // empty')"
  got_user="$(printf '%s' "$json" | jq -r '.user.name // empty')"
  if [[ "$exp_tenant" != "$got_tenant" && "$exp_tenant" != "$got_domain" ]]; then
    printf 'azlogin: TENANT MISMATCH — expected %q, got tenantId=%q domain=%q\n' \
      "$exp_tenant" "$got_tenant" "$got_domain" >&2
    return 1
  fi
  if [[ -n "$exp_user" && "$exp_user" != "$got_user" ]]; then
    printf 'azlogin: USER MISMATCH — expected %q, got %q\n' "$exp_user" "$got_user" >&2
    return 1
  fi
  return 0
}
