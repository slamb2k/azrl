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
