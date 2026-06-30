#!/usr/bin/env bash
# Pure, sourceable helpers for azrl. No side effects on source.

azrl_usage() {
  cat <<'EOF'
azrl — Azure Remote Login

Interactive `az login` from a headless/remote VM: the sign-in browser opens on
your local machine and the OAuth callback is forwarded back to the VM.

Usage:
  azrl [profile] [--paste]
  azrl --derive [profile]
  azrl --list | --help | --version

Arguments:
  profile          Azure profile name. If omitted, resolved from the nearest
                   .azprofile found walking up from the current directory.

Options:
  --paste          Force the manual paste-line path (A) instead of the
                   zero-paste reverse-tunnel path (B).
  --derive         Generate <profile>.conf from the current logged-in session
                   for that profile (does not log in; refuses to overwrite).
  --list           List configured profiles and their tenants.
  -h, --help       Show this help and exit.
  -V, --version    Show version and exit.
EOF
}

azrl_list_profiles() {
  # [$1=confdir]. Prints "<profile>  <AZ_TENANT>" per configured profile,
  # excluding the global azrl.conf. Silent (exit 0) when none exist.
  local confdir="${1:-$HOME/.azure-profiles}" f name tenant
  for f in "$confdir"/*.conf; do
    [[ -e "$f" ]] || continue
    name="$(basename "$f" .conf)"
    [[ "$name" == "azrl" ]] && continue
    # shellcheck disable=SC1090
    tenant="$( source "$f" 2>/dev/null || true; printf '%s' "${AZ_TENANT:-?}" )"
    printf '%-24s %s\n' "$name" "$tenant"
  done
}

azrl_save_conf() {
  # $1=account_json (`az account show`) $2=domains_json (graph /v1.0/domains).
  # Emits a ready-to-save <profile>.conf to stdout. AZ_TENANT prefers the
  # verified default domain; falls back to the tenant GUID (e.g. guest/B2B).
  local acct="$1" doms="$2" tenant_id user sub domain
  tenant_id="$(jq -r '.tenantId // empty'   <<<"$acct")"
  user="$(jq -r '.user.name // empty'       <<<"$acct")"
  # Subscription id (GUID), not name: space-free and canonical, so the emitted
  # conf is safe to `source` (a name like "VS Enterprise – Lamb" would not be).
  sub="$(jq -r '.id // empty'               <<<"$acct")"
  domain="$(jq -r '[.value[]? | select(.isDefault==true).id][0] // empty' <<<"$doms")"
  [[ -n "$domain" ]] || domain="$tenant_id"
  cat <<EOF
AZ_TENANT=$domain
AZ_TENANT_ID=$tenant_id
AZ_DEFAULT_SUB=$sub
AZ_EXPECT_USER=$user
EOF
}

azrl_write_profile() {
  # $1=profile $2=target_dir. Uses AZURE_CONFIG_DIR (set by caller).
  # Writes ~/.azure-profiles/<profile>.conf from the current session (refusing to
  # clobber) and <target_dir>/.azprofile. Returns 1 on existing conf or no session.
  local profile="$1" dir="$2"
  local out="$HOME/.azure-profiles/$profile.conf"
  [[ -e "$out" ]] && { printf 'azrl: %s already exists — remove it first to re-save\n' "$out" >&2; return 1; }
  local acct doms
  acct="$(az account show -o json 2>/dev/null)" \
    || { printf 'azrl: not logged in for %q — run azrl --init first\n' "$profile" >&2; return 1; }
  doms="$(az rest --url 'https://graph.microsoft.com/v1.0/domains' -o json 2>/dev/null || printf '{}')"
  azrl_save_conf "$acct" "$doms" > "$out"
  printf '%s\n' "$profile" > "$dir/.azprofile"
  printf 'azrl: wrote %s and %s/.azprofile\n' "$out" "$dir"
  return 0
}

azrl_extract_port() {
  local url="$1" decoded
  decoded="${url//%3A/:}"; decoded="${decoded//%2F//}"
  printf '%s' "$decoded" | grep -oE 'localhost:[0-9]+' | head -1 | cut -d: -f2
}

azrl_resolve_profile() {
  local arg="$1" dir="${2:-$PWD}"
  if [[ -n "$arg" ]]; then printf '%s\n' "$arg"; return 0; fi
  local d="$dir"
  while [[ -n "$d" && "$d" != "/" ]]; do
    if [[ -f "$d/.azprofile" ]]; then
      tr -d '[:space:]' < "$d/.azprofile"; printf '\n'; return 0
    fi
    d="$(dirname "$d")"
  done
  printf 'azrl: no profile arg and no .azprofile found from %s\n' "$dir" >&2
  return 1
}

azrl_sanitize_name() {
  # $1=raw. lowercase; non [a-z0-9._-] runs -> '-'; strip leading/trailing '-'.
  local s="${1,,}"
  s="$(printf '%s' "$s" | sed -E 's/[^a-z0-9._-]+/-/g; s/^-+//; s/-+$//')"
  printf '%s\n' "$s"
}

azrl_default_name() {
  # $1=arg $2=dir. Explicit arg verbatim; else sanitized basename of dir.
  local arg="$1" dir="${2:-$PWD}"
  if [[ -n "$arg" ]]; then printf '%s\n' "$arg"; return 0; fi
  azrl_sanitize_name "$(basename "$dir")"
}

azrl_load_profile_conf() {
  local profile="$1" confdir="${2:-$HOME/.azure-profiles}"
  local f="$confdir/$profile.conf"
  [[ -f "$f" ]] || { printf 'azrl: missing config %s\n' "$f" >&2; return 1; }
  # shellcheck disable=SC1090
  source "$f"
  [[ -n "${AZ_TENANT:-}" ]] || { printf 'azrl: AZ_TENANT not set in %s\n' "$f" >&2; return 1; }
  return 0
}

azrl_paste_line() {
  # $1=port $2=vm_host $3=browser_cmd $4=url
  printf 'ssh -fNL %s:localhost:%s %s && %s "%s"\n' "$1" "$1" "$2" "$3" "$4"
}

azrl_assert_account() {
  # $1=account_json $2=expected_tenant (GUID or domain) $3=expected_user (optional)
  local json="$1" exp_tenant="$2" exp_user="$3"
  local got_tenant got_domain got_user
  got_tenant="$(printf '%s' "$json" | jq -r '.tenantId // empty')"
  got_domain="$(printf '%s' "$json" | jq -r '.tenantDefaultDomain // empty')"
  got_user="$(printf '%s' "$json" | jq -r '.user.name // empty')"
  if [[ "$exp_tenant" != "$got_tenant" && "$exp_tenant" != "$got_domain" ]]; then
    printf 'azrl: TENANT MISMATCH — expected %q, got tenantId=%q domain=%q\n' \
      "$exp_tenant" "$got_tenant" "$got_domain" >&2
    return 1
  fi
  if [[ -n "$exp_user" && "$exp_user" != "$got_user" ]]; then
    printf 'azrl: USER MISMATCH — expected %q, got %q\n' "$exp_user" "$got_user" >&2
    return 1
  fi
  return 0
}

azrl_clean_slate() {
  # Operates only within $AZURE_CONFIG_DIR (isolated profile).
  az logout >/dev/null 2>&1 || true
  az account clear >/dev/null 2>&1 || true
  rm -f "${AZURE_CONFIG_DIR:?}/msal_token_cache.json" \
        "${AZURE_CONFIG_DIR:?}/service_principal_entries.json"
  return 0
}

azrl_login_capture() {
  # $1 = tenant. Sets globals: AZRL_CAPFILE, AZRL_URL, AZRL_PORT, AZRL_LOGIN_PID.
  local tenant="$1"
  local poll_max="${AZRL_CAPTURE_POLL:-200}"   # 200 × 0.1s = 20s
  AZRL_CAPFILE="$(mktemp)"; : > "$AZRL_CAPFILE"
  local capture="${AZRL_CAPTURE:-$HOME/.local/bin/azrl-capture}"
  local -a tenant_args=()
  [[ -n "$tenant" ]] && tenant_args=(--tenant "$tenant")
  AZRL_CAPFILE="$AZRL_CAPFILE" BROWSER="$capture %s" \
    az login ${tenant_args[@]+"${tenant_args[@]}"} --only-show-errors >/dev/null 2>&1 &
  AZRL_LOGIN_PID=$!
  local _
  for _ in $(seq 1 "$poll_max"); do
    [[ -s "$AZRL_CAPFILE" ]] && break
    kill -0 "$AZRL_LOGIN_PID" 2>/dev/null || break
    sleep 0.1
  done
  if [[ ! -s "$AZRL_CAPFILE" ]]; then
    if kill -0 "$AZRL_LOGIN_PID" 2>/dev/null; then
      printf 'azrl: timed out waiting for auth URL after %ss\n' "$((poll_max/10))" >&2
    else
      printf 'azrl: az login exited before producing an auth URL (check tenant/credentials)\n' >&2
    fi
    return 1
  fi
  AZRL_URL="$(cat "$AZRL_CAPFILE")"
  AZRL_PORT="$(azrl_extract_port "$AZRL_URL")"
  [[ -n "$AZRL_PORT" ]] || { printf 'azrl: could not parse callback port\n' >&2; return 1; }
  return 0
}

azrl_wait_for_login() {
  # $1=login_pid $2=timeout_s $3=port $4=vm_host $5=browser_cmd $6=url
  # Sets AZRL_WATCHDOG_PID. Returns the login process's exit code; on nonzero,
  # prints a path-A recovery hint to stderr. Does not exit (caller decides).
  local login_pid="$1" timeout="$2" port="$3" vm_host="$4" browser_cmd="$5" url="$6"
  ( sleep "$timeout"; kill "$login_pid" 2>/dev/null ) &
  # shellcheck disable=SC2034  # consumed cross-file by the orchestrator cleanup trap
  AZRL_WATCHDOG_PID=$!
  local rc=0
  wait "$login_pid" || rc=$?
  kill "$AZRL_WATCHDOG_PID" 2>/dev/null || true
  if (( rc != 0 )); then
    printf '✗ azrl: sign-in did not complete (rc=%s). Either it failed/was cancelled, or the browser callback never reached this VM (timeout %ss).\n' "$rc" "$timeout" >&2
    printf '  Recover by pasting this on your LOCAL machine, then retrying the browser:\n\n' >&2
    azrl_paste_line "$port" "$vm_host" "$browser_cmd" "$url" >&2
    printf '\n' >&2
  fi
  return "$rc"
}

# shellcheck disable=SC2153  # VM_HOST is a global from azrl.conf, not the vm_host local elsewhere
azrl_bridge() {
  # $1=port $2=url. Uses LOCAL_HOST, LOCAL_BROWSER_CMD, VM_HOST, AZRL_FORCE_PASTE.
  # Sets AZRL_TUNNEL_PID when a reverse tunnel is started (for teardown).
  local port="$1" url="$2"
  if [[ "${AZRL_FORCE_PASTE:-0}" != "1" ]] \
     && ssh -o BatchMode=yes -o ConnectTimeout=5 "$LOCAL_HOST" true 2>/dev/null; then
    ssh -N -R "$port:localhost:$port" "$LOCAL_HOST" 2>/dev/null &
    # shellcheck disable=SC2034  # consumed cross-file by the orchestrator cleanup trap
    AZRL_TUNNEL_PID=$!
    sleep 0.5
    if kill -0 "$AZRL_TUNNEL_PID" 2>/dev/null; then
      # shellcheck disable=SC2029  # $url intentionally expands locally to run on LOCAL_HOST
      ssh "$LOCAL_HOST" "$LOCAL_BROWSER_CMD '$url'" >/dev/null 2>&1 || true
      printf 'azrl: browser opened on %s (zero-paste path B)\n' "$LOCAL_HOST"
    else
      unset AZRL_TUNNEL_PID
      printf 'azrl: reverse tunnel failed — paste this on your LOCAL machine:\n\n' >&2
      azrl_paste_line "$port" "$VM_HOST" "$LOCAL_BROWSER_CMD" "$url" >&2
      printf '\n' >&2
    fi
  else
    printf 'azrl: local callback unavailable — paste this on your LOCAL machine:\n\n' >&2
    azrl_paste_line "$port" "$VM_HOST" "$LOCAL_BROWSER_CMD" "$url" >&2
    printf '\n' >&2
  fi
  return 0
}
