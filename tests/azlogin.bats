#!/usr/bin/env bats

setup() {
  load_lib() { source "${BATS_TEST_DIRNAME}/../azlogin-lib.sh"; }
  load_lib
}

@test "azl_extract_port: url-encoded redirect_uri" {
  url='https://login.microsoftonline.com/x?redirect_uri=http%3A%2F%2Flocalhost%3A38149%2F&state=y'
  run azl_extract_port "$url"
  [ "$status" -eq 0 ]
  [ "$output" = "38149" ]
}

@test "azl_extract_port: plain redirect_uri" {
  url='https://login.microsoftonline.com/x?redirect_uri=http://localhost:55322/&state=y'
  run azl_extract_port "$url"
  [ "$output" = "55322" ]
}

@test "azl_resolve_profile: explicit arg wins" {
  run azl_resolve_profile "fiig" "/tmp"
  [ "$output" = "fiig" ]
}

@test "azl_resolve_profile: reads .azprofile walking up" {
  tmp="$(mktemp -d)"; mkdir -p "$tmp/a/b"
  printf 'digital-it-apps\n' > "$tmp/.azprofile"
  run azl_resolve_profile "" "$tmp/a/b"
  [ "$status" -eq 0 ]
  [ "$output" = "digital-it-apps" ]
  rm -rf "$tmp"
}

@test "azl_resolve_profile: no arg, no file -> error" {
  tmp="$(mktemp -d)"
  run azl_resolve_profile "" "$tmp"
  [ "$status" -ne 0 ]
  rm -rf "$tmp"
}

@test "azl_load_profile_conf: sources tenant and returns 0" {
  tmp="$(mktemp -d)"
  printf 'AZ_TENANT=fiig.com.au\nAZ_DEFAULT_SUB=sub-123\n' > "$tmp/fiig.conf"
  run bash -c "source '${BATS_TEST_DIRNAME}/../azlogin-lib.sh'; azl_load_profile_conf fiig '$tmp'; echo \"\$AZ_TENANT\""
  [ "$status" -eq 0 ]
  [[ "$output" == *"fiig.com.au"* ]]
  rm -rf "$tmp"
}

@test "azl_load_profile_conf: missing file -> error" {
  tmp="$(mktemp -d)"
  run azl_load_profile_conf nope "$tmp"
  [ "$status" -ne 0 ]
  rm -rf "$tmp"
}

@test "azl_load_profile_conf: missing AZ_TENANT -> error" {
  tmp="$(mktemp -d)"
  printf 'AZ_DEFAULT_SUB=x\n' > "$tmp/bad.conf"
  run azl_load_profile_conf bad "$tmp"
  [ "$status" -ne 0 ]
  rm -rf "$tmp"
}

@test "azl_paste_line: builds local forward+open command" {
  run azl_paste_line 38149 vm-always wslview 'https://login/x?y=z'
  [ "$status" -eq 0 ]
  [ "$output" = 'ssh -fNL 38149:localhost:38149 vm-always && wslview "https://login/x?y=z"' ]
}

@test "azl_assert_account: matches tenant domain and user" {
  json='{"tenantId":"guid-1","tenantDefaultDomain":"fiig.com.au","user":{"name":"simon@fiig.com.au"},"name":"sub"}'
  run azl_assert_account "$json" "fiig.com.au" "simon@fiig.com.au"
  [ "$status" -eq 0 ]
}

@test "azl_assert_account: matches tenant by GUID" {
  json='{"tenantId":"guid-1","tenantDefaultDomain":"fiig.com.au","user":{"name":"x"}}'
  run azl_assert_account "$json" "guid-1" ""
  [ "$status" -eq 0 ]
}

@test "azl_assert_account: tenant mismatch -> error" {
  json='{"tenantId":"guid-1","tenantDefaultDomain":"other.com","user":{"name":"x"}}'
  run azl_assert_account "$json" "fiig.com.au" ""
  [ "$status" -ne 0 ]
}

@test "azl_assert_account: user mismatch -> error" {
  json='{"tenantId":"g","tenantDefaultDomain":"fiig.com.au","user":{"name":"wrong@x"}}'
  run azl_assert_account "$json" "fiig.com.au" "right@x"
  [ "$status" -ne 0 ]
}

@test "azl_assert_account: guest tenant — null domain, expected GUID matches tenantId" {
  json='{"tenantId":"96e360c3-4483-43a9-9025-195a431eba14","tenantDefaultDomain":null,"user":{"name":"Simon.Lamb@velrada.com"}}'
  run azl_assert_account "$json" "96e360c3-4483-43a9-9025-195a431eba14" "Simon.Lamb@velrada.com"
  [ "$status" -eq 0 ]
}

@test "azl_clean_slate: calls az logout+clear and removes only scoped caches" {
  shimdir="$(mktemp -d)"; cfg="$(mktemp -d)"
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
echo "az $*" >> "$AZ_SHIM_LOG"
EOF
  chmod +x "$shimdir/az"
  export AZ_SHIM_LOG="$cfg/shim.log"
  export AZURE_CONFIG_DIR="$cfg"
  : > "$cfg/msal_token_cache.json"
  : > "$cfg/service_principal_entries.json"
  PATH="$shimdir:$PATH" run azl_clean_slate
  [ "$status" -eq 0 ]
  grep -q "az logout" "$AZ_SHIM_LOG"
  grep -q "az account clear" "$AZ_SHIM_LOG"
  [ ! -f "$cfg/msal_token_cache.json" ]
  [ ! -f "$cfg/service_principal_entries.json" ]
  rm -rf "$shimdir" "$cfg"
}

@test "azl_login_capture: sets AZL_PORT and AZL_URL from captured browser URL" {
  shimdir="$(mktemp -d)"
  # Fake az: emulate webbrowser by invoking $BROWSER with a URL, then block briefly.
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F&state=z'
# $BROWSER is like "/path/azlogin-capture %s"
cmd="${BROWSER/\%s/$url}"
eval "$cmd"
sleep 2
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azlogin-lib.sh'
    export AZLOGIN_CAPTURE='${BATS_TEST_DIRNAME}/../azlogin-capture'
    PATH="$shimdir:\$PATH" azl_login_capture fiig.com.au
    echo \"PORT=\$AZL_PORT URL=\$AZL_URL\"
    kill \$AZL_LOGIN_PID 2>/dev/null || true
  "
  [ "$status" -eq 0 ]
  [[ "$output" == *"PORT=40404"* ]]
  rm -rf "$shimdir"
}

@test "azl_bridge: B path when local reachable (uses reverse tunnel + browser cmd)" {
  shimdir="$(mktemp -d)"; log="$shimdir/ssh.log"
  cat > "$shimdir/ssh" <<EOF
#!/usr/bin/env bash
echo "ssh \$*" >> "$log"
# reachability probe ("ssh ... <host> true") and browser cmd should succeed.
# The reverse tunnel (-R) must stay alive so the liveness check sees it running.
for a in "\$@"; do [[ "\$a" == "-R" ]] && { sleep 2; exit 0; }; done
exit 0
EOF
  chmod +x "$shimdir/ssh"
  export LOCAL_HOST=velrada-pc-wsl LOCAL_BROWSER_CMD=wslview VM_HOST=vm-always AZL_FORCE_PASTE=0
  PATH="$shimdir:$PATH" run azl_bridge 40404 'https://login/x'
  [ "$status" -eq 0 ]
  grep -q -- "-R 40404:localhost:40404 velrada-pc-wsl" "$log"
  grep -q "wslview" "$log"
  rm -rf "$shimdir"
}

@test "azl_bridge: A path when forced to paste" {
  export LOCAL_HOST=velrada-pc-wsl LOCAL_BROWSER_CMD=wslview VM_HOST=vm-always AZL_FORCE_PASTE=1
  run azl_bridge 40404 'https://login/x'
  [ "$status" -eq 0 ]
  [[ "$output" == *"ssh -fNL 40404:localhost:40404 vm-always && wslview"* ]]
}

@test "azl_bridge: B falls back to A when reverse tunnel dies" {
  shimdir="$(mktemp -d)"
  cat > "$shimdir/ssh" <<'EOF'
#!/usr/bin/env bash
# reachability probe succeeds; the reverse-tunnel (-R) invocation fails immediately
for a in "$@"; do [[ "$a" == "-R" ]] && exit 1; done
exit 0
EOF
  chmod +x "$shimdir/ssh"
  export LOCAL_HOST=velrada-pc-wsl LOCAL_BROWSER_CMD=wslview VM_HOST=vm-always AZL_FORCE_PASTE=0
  PATH="$shimdir:$PATH" run azl_bridge 40404 'https://login/x'
  [ "$status" -eq 0 ]
  [[ "$output" == *"ssh -fNL 40404:localhost:40404 vm-always && wslview"* ]]
  rm -rf "$shimdir"
}
