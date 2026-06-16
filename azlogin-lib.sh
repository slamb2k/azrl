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

azl_clean_slate() {
  # Operates only within $AZURE_CONFIG_DIR (isolated profile).
  az logout >/dev/null 2>&1 || true
  az account clear >/dev/null 2>&1 || true
  rm -f "${AZURE_CONFIG_DIR:?}/msal_token_cache.json" \
        "${AZURE_CONFIG_DIR:?}/service_principal_entries.json"
  return 0
}

azl_login_capture() {
  # $1 = tenant. Sets globals: AZL_CAPFILE, AZL_URL, AZL_PORT, AZL_LOGIN_PID.
  local tenant="$1"
  local poll_max="${AZL_CAPTURE_POLL:-200}"   # 200 × 0.1s = 20s
  AZL_CAPFILE="$(mktemp)"; : > "$AZL_CAPFILE"
  local capture="${AZLOGIN_CAPTURE:-$HOME/.local/bin/azlogin-capture}"
  AZLOGIN_CAPFILE="$AZL_CAPFILE" BROWSER="$capture %s" \
    az login --tenant "$tenant" --only-show-errors >/dev/null 2>&1 &
  AZL_LOGIN_PID=$!
  local _
  for _ in $(seq 1 "$poll_max"); do
    [[ -s "$AZL_CAPFILE" ]] && break
    kill -0 "$AZL_LOGIN_PID" 2>/dev/null || break
    sleep 0.1
  done
  if [[ ! -s "$AZL_CAPFILE" ]]; then
    if kill -0 "$AZL_LOGIN_PID" 2>/dev/null; then
      printf 'azlogin: timed out waiting for auth URL after %ss\n' "$((poll_max/10))" >&2
    else
      printf 'azlogin: az login exited before producing an auth URL (check tenant/credentials)\n' >&2
    fi
    return 1
  fi
  AZL_URL="$(cat "$AZL_CAPFILE")"
  AZL_PORT="$(azl_extract_port "$AZL_URL")"
  [[ -n "$AZL_PORT" ]] || { printf 'azlogin: could not parse callback port\n' >&2; return 1; }
  return 0
}

azl_bridge() {
  # $1=port $2=url. Uses LOCAL_HOST, LOCAL_BROWSER_CMD, VM_HOST, AZL_FORCE_PASTE.
  # Sets AZL_TUNNEL_PID when a reverse tunnel is started (for teardown).
  local port="$1" url="$2"
  if [[ "${AZL_FORCE_PASTE:-0}" != "1" ]] \
     && ssh -o BatchMode=yes -o ConnectTimeout=5 "$LOCAL_HOST" true 2>/dev/null; then
    ssh -N -R "$port:localhost:$port" "$LOCAL_HOST" 2>/dev/null &
    # shellcheck disable=SC2034  # consumed cross-file by the orchestrator cleanup trap
    AZL_TUNNEL_PID=$!
    sleep 0.5
    if kill -0 "$AZL_TUNNEL_PID" 2>/dev/null; then
      # shellcheck disable=SC2029  # $url intentionally expands locally to run on LOCAL_HOST
      ssh "$LOCAL_HOST" "$LOCAL_BROWSER_CMD '$url'" >/dev/null 2>&1 || true
      printf 'azlogin: browser opened on %s (zero-paste path B)\n' "$LOCAL_HOST"
    else
      unset AZL_TUNNEL_PID
      printf 'azlogin: reverse tunnel failed — paste this on your LOCAL machine:\n\n' >&2
      azl_paste_line "$port" "$VM_HOST" "$LOCAL_BROWSER_CMD" "$url" >&2
      printf '\n' >&2
    fi
  else
    printf 'azlogin: local callback unavailable — paste this on your LOCAL machine:\n\n' >&2
    azl_paste_line "$port" "$VM_HOST" "$LOCAL_BROWSER_CMD" "$url" >&2
    printf '\n' >&2
  fi
  return 0
}
