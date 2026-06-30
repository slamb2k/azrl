#!/usr/bin/env bats

setup() {
  load_lib() { source "${BATS_TEST_DIRNAME}/../azrl-lib.sh"; }
  load_lib
}

@test "azrl_extract_port: url-encoded redirect_uri" {
  url='https://login.microsoftonline.com/x?redirect_uri=http%3A%2F%2Flocalhost%3A38149%2F&state=y'
  run azrl_extract_port "$url"
  [ "$status" -eq 0 ]
  [ "$output" = "38149" ]
}

@test "azrl_extract_port: plain redirect_uri" {
  url='https://login.microsoftonline.com/x?redirect_uri=http://localhost:55322/&state=y'
  run azrl_extract_port "$url"
  [ "$output" = "55322" ]
}

@test "azrl_resolve_profile: explicit arg wins" {
  run azrl_resolve_profile "fiig" "/tmp"
  [ "$output" = "fiig" ]
}

@test "azrl_resolve_profile: reads .azprofile walking up" {
  tmp="$(mktemp -d)"; mkdir -p "$tmp/a/b"
  printf 'digital-it-apps\n' > "$tmp/.azprofile"
  run azrl_resolve_profile "" "$tmp/a/b"
  [ "$status" -eq 0 ]
  [ "$output" = "digital-it-apps" ]
  rm -rf "$tmp"
}

@test "azrl_resolve_profile: no arg, no file -> error" {
  tmp="$(mktemp -d)"
  run azrl_resolve_profile "" "$tmp"
  [ "$status" -ne 0 ]
  rm -rf "$tmp"
}

@test "azrl_load_profile_conf: sources tenant and returns 0" {
  tmp="$(mktemp -d)"
  printf 'AZ_TENANT=fiig.com.au\nAZ_DEFAULT_SUB=sub-123\n' > "$tmp/fiig.conf"
  run bash -c "source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'; azrl_load_profile_conf fiig '$tmp'; echo \"\$AZ_TENANT\""
  [ "$status" -eq 0 ]
  [[ "$output" == *"fiig.com.au"* ]]
  rm -rf "$tmp"
}

@test "azrl_load_profile_conf: missing file -> error" {
  tmp="$(mktemp -d)"
  run azrl_load_profile_conf nope "$tmp"
  [ "$status" -ne 0 ]
  rm -rf "$tmp"
}

@test "azrl_load_profile_conf: missing AZ_TENANT -> error" {
  tmp="$(mktemp -d)"
  printf 'AZ_DEFAULT_SUB=x\n' > "$tmp/bad.conf"
  run azrl_load_profile_conf bad "$tmp"
  [ "$status" -ne 0 ]
  rm -rf "$tmp"
}

@test "azrl_paste_line: builds local forward+open command" {
  run azrl_paste_line 38149 vm-always wslview 'https://login/x?y=z'
  [ "$status" -eq 0 ]
  [ "$output" = 'ssh -fNL 38149:localhost:38149 vm-always && wslview "https://login/x?y=z"' ]
}

@test "azrl_assert_account: matches tenant domain and user" {
  json='{"tenantId":"guid-1","tenantDefaultDomain":"fiig.com.au","user":{"name":"simon@fiig.com.au"},"name":"sub"}'
  run azrl_assert_account "$json" "fiig.com.au" "simon@fiig.com.au"
  [ "$status" -eq 0 ]
}

@test "azrl_assert_account: matches tenant by GUID" {
  json='{"tenantId":"guid-1","tenantDefaultDomain":"fiig.com.au","user":{"name":"x"}}'
  run azrl_assert_account "$json" "guid-1" ""
  [ "$status" -eq 0 ]
}

@test "azrl_assert_account: tenant mismatch -> error" {
  json='{"tenantId":"guid-1","tenantDefaultDomain":"other.com","user":{"name":"x"}}'
  run azrl_assert_account "$json" "fiig.com.au" ""
  [ "$status" -ne 0 ]
}

@test "azrl_assert_account: user mismatch -> error" {
  json='{"tenantId":"g","tenantDefaultDomain":"fiig.com.au","user":{"name":"wrong@x"}}'
  run azrl_assert_account "$json" "fiig.com.au" "right@x"
  [ "$status" -ne 0 ]
}

@test "azrl_assert_account: guest tenant — null domain, expected GUID matches tenantId" {
  json='{"tenantId":"96e360c3-4483-43a9-9025-195a431eba14","tenantDefaultDomain":null,"user":{"name":"Simon.Lamb@velrada.com"}}'
  run azrl_assert_account "$json" "96e360c3-4483-43a9-9025-195a431eba14" "Simon.Lamb@velrada.com"
  [ "$status" -eq 0 ]
}

@test "azrl_clean_slate: calls az logout+clear and removes only scoped caches" {
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
  PATH="$shimdir:$PATH" run azrl_clean_slate
  [ "$status" -eq 0 ]
  grep -q "az logout" "$AZ_SHIM_LOG"
  grep -q "az account clear" "$AZ_SHIM_LOG"
  [ ! -f "$cfg/msal_token_cache.json" ]
  [ ! -f "$cfg/service_principal_entries.json" ]
  rm -rf "$shimdir" "$cfg"
}

@test "azrl_login_capture: sets AZRL_PORT and AZRL_URL from captured browser URL" {
  shimdir="$(mktemp -d)"
  # Fake az: emulate webbrowser by invoking $BROWSER with a URL, then block briefly.
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F&state=z'
# $BROWSER is like "/path/azrl-capture %s"
cmd="${BROWSER/\%s/$url}"
eval "$cmd"
sleep 2
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export AZRL_CAPTURE='${BATS_TEST_DIRNAME}/../azrl-capture'
    PATH="$shimdir:\$PATH" azrl_login_capture fiig.com.au
    echo \"PORT=\$AZRL_PORT URL=\$AZRL_URL\"
    kill \$AZRL_LOGIN_PID 2>/dev/null || true
  "
  [ "$status" -eq 0 ]
  [[ "$output" == *"PORT=40404"* ]]
  rm -rf "$shimdir"
}

@test "azrl_login_capture: omits --tenant when tenant is empty" {
  shimdir="$(mktemp -d)"; log="$shimdir/az.log"
  cat > "$shimdir/az" <<EOF
#!/usr/bin/env bash
echo "\$*" >> "$log"
url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F&state=z'
cmd="\${BROWSER/\\%s/\$url}"
eval "\$cmd"
sleep 2
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export AZRL_CAPTURE='${BATS_TEST_DIRNAME}/../azrl-capture'
    PATH='$shimdir':\$PATH azrl_login_capture ''
    echo \"PORT=\$AZRL_PORT\"
    kill \$AZRL_LOGIN_PID 2>/dev/null || true
  "
  [ "$status" -eq 0 ]
  [[ "$output" == *"PORT=40404"* ]]
  grep -q 'login' "$log"
  run grep -q -- '--tenant' "$log"
  [ "$status" -ne 0 ]
  run grep -q -- '--allow-no-subscription' "$log"
  [ "$status" -eq 0 ]
  rm -rf "$shimdir"
}

@test "azrl_login_capture: passes --allow-no-subscription (with tenant)" {
  shimdir="$(mktemp -d)"; log="$shimdir/az.log"
  cat > "$shimdir/az" <<EOF
#!/usr/bin/env bash
echo "\$*" >> "$log"
url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F&state=z'
cmd="\${BROWSER/\\%s/\$url}"
eval "\$cmd"
sleep 2
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export AZRL_CAPTURE='${BATS_TEST_DIRNAME}/../azrl-capture'
    PATH='$shimdir':\$PATH azrl_login_capture fiig.com.au
    echo \"PORT=\$AZRL_PORT\"
    kill \$AZRL_LOGIN_PID 2>/dev/null || true
  "
  [ "$status" -eq 0 ]
  [[ "$output" == *"PORT=40404"* ]]
  run grep -q -- '--tenant fiig.com.au' "$log"
  [ "$status" -eq 0 ]
  run grep -q -- '--allow-no-subscription' "$log"
  [ "$status" -eq 0 ]
  rm -rf "$shimdir"
}

@test "azrl_bridge: B path when local reachable (uses reverse tunnel + browser cmd)" {
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
  export LOCAL_HOST=velrada-pc-wsl LOCAL_BROWSER_CMD=wslview VM_HOST=vm-always AZRL_FORCE_PASTE=0
  PATH="$shimdir:$PATH" run azrl_bridge 40404 'https://login/x'
  [ "$status" -eq 0 ]
  grep -q -- "-R 40404:localhost:40404 velrada-pc-wsl" "$log"
  grep -q "wslview" "$log"
  rm -rf "$shimdir"
}

@test "azrl_bridge: A path when forced to paste" {
  export LOCAL_HOST=velrada-pc-wsl LOCAL_BROWSER_CMD=wslview VM_HOST=vm-always AZRL_FORCE_PASTE=1
  run azrl_bridge 40404 'https://login/x'
  [ "$status" -eq 0 ]
  [[ "$output" == *"ssh -fNL 40404:localhost:40404 vm-always && wslview"* ]]
}

@test "azrl_wait_for_login: returns 0 and prints no recovery hint when login succeeds" {
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    ( exit 0 ) &
    azrl_wait_for_login \$! 5 40404 vm-always wslview 'https://login/x'
  "
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "azrl_wait_for_login: returns login rc and prints recovery line on failure" {
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    ( exit 3 ) &
    azrl_wait_for_login \$! 5 40404 vm-always wslview 'https://login/x'
  "
  [ "$status" -eq 3 ]
  [[ "$output" == *"sign-in did not complete"* ]]
  [[ "$output" == *"ssh -fNL 40404:localhost:40404 vm-always && wslview"* ]]
}

@test "azrl_wait_for_login: watchdog kills login on timeout and reports failure" {
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    ( sleep 10 ) &
    azrl_wait_for_login \$! 1 40404 vm-always wslview 'https://login/x'
  "
  [ "$status" -ne 0 ]
  [[ "$output" == *"sign-in did not complete"* ]]
  [[ "$output" == *"ssh -fNL 40404:localhost:40404 vm-always"* ]]
}

@test "azrl_bridge: B falls back to A when reverse tunnel dies" {
  shimdir="$(mktemp -d)"
  cat > "$shimdir/ssh" <<'EOF'
#!/usr/bin/env bash
# reachability probe succeeds; the reverse-tunnel (-R) invocation fails immediately
for a in "$@"; do [[ "$a" == "-R" ]] && exit 1; done
exit 0
EOF
  chmod +x "$shimdir/ssh"
  export LOCAL_HOST=velrada-pc-wsl LOCAL_BROWSER_CMD=wslview VM_HOST=vm-always AZRL_FORCE_PASTE=0
  PATH="$shimdir:$PATH" run azrl_bridge 40404 'https://login/x'
  [ "$status" -eq 0 ]
  [[ "$output" == *"ssh -fNL 40404:localhost:40404 vm-always && wslview"* ]]
  rm -rf "$shimdir"
}

@test "azrl_usage: includes synopsis and all flags" {
  run azrl_usage
  [ "$status" -eq 0 ]
  [[ "$output" == *"Usage:"* ]]
  [[ "$output" == *"--paste"* ]]
  [[ "$output" == *"--help"* ]]
  [[ "$output" == *"--version"* ]]
}

@test "azrl --help: prints usage and exits 0 without needing config" {
  run "${BATS_TEST_DIRNAME}/../azrl" --help
  [ "$status" -eq 0 ]
  [[ "$output" == *"Usage:"* ]]
  [[ "$output" == *"--paste"* ]]
}

@test "azrl -h: prints usage and exits 0" {
  run "${BATS_TEST_DIRNAME}/../azrl" -h
  [ "$status" -eq 0 ]
  [[ "$output" == *"Usage:"* ]]
}

@test "azrl --version: prints version and exits 0" {
  run "${BATS_TEST_DIRNAME}/../azrl" --version
  [ "$status" -eq 0 ]
  [[ "$output" =~ azrl\ [0-9]+\.[0-9]+\.[0-9]+ ]]
}

@test "azrl: unknown flag exits 2" {
  run "${BATS_TEST_DIRNAME}/../azrl" --bogus
  [ "$status" -eq 2 ]
}

@test "azrl: missing <profile>.conf exits nonzero without creating an orphan state dir" {
  home="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles"
  cat > "$home/.azure-profiles/azrl.conf" <<'EOF'
LOCAL_HOST=localhost
LOCAL_BROWSER_CMD=true
VM_HOST=vm
EOF
  HOME="$home" run "${BATS_TEST_DIRNAME}/../azrl" ghost
  [ "$status" -ne 0 ]
  [ ! -e "$home/.azure-profiles/ghost" ]
  rm -rf "$home"
}

@test "azrl_list_profiles: lists profiles with tenant, excluding azrl.conf" {
  tmp="$(mktemp -d)"
  printf 'AZ_TENANT=fiig.com.au\n'             > "$tmp/fiig.conf"
  printf 'AZ_TENANT=onenrg.onmicrosoft.com\n'  > "$tmp/nrg.conf"
  printf 'LOCAL_HOST=x\n'                      > "$tmp/azrl.conf"
  run azrl_list_profiles "$tmp"
  [ "$status" -eq 0 ]
  [[ "$output" == *"fiig"* ]]
  [[ "$output" == *"fiig.com.au"* ]]
  [[ "$output" == *"nrg"* ]]
  [[ "$output" == *"onenrg.onmicrosoft.com"* ]]
  [[ "$output" != *"azrl"* ]]
  rm -rf "$tmp"
}

@test "azrl_list_profiles: empty confdir prints nothing, exits 0" {
  tmp="$(mktemp -d)"
  run azrl_list_profiles "$tmp"
  [ "$status" -eq 0 ]
  [ -z "$output" ]
  rm -rf "$tmp"
}

@test "azrl_save_conf: builds conf from account + domains json (sub by id, space-safe)" {
  # subscription name has spaces; the conf must use the space-free id so a later
  # `source <profile>.conf` doesn't choke.
  acct='{"tenantId":"guid-1","id":"sub-guid-9","name":"VS Enterprise – Lamb","user":{"name":"simon@onenrg.onmicrosoft.com"}}'
  doms='{"value":[{"id":"onenrg.mail.onmicrosoft.com","isDefault":false},{"id":"onenrg.onmicrosoft.com","isDefault":true}]}'
  run azrl_save_conf "$acct" "$doms"
  [ "$status" -eq 0 ]
  [[ "$output" == *"AZ_TENANT=onenrg.onmicrosoft.com"* ]]
  [[ "$output" == *"AZ_TENANT_ID=guid-1"* ]]
  [[ "$output" == *"AZ_DEFAULT_SUB=sub-guid-9"* ]]
  [[ "$output" != *"AZ_DEFAULT_SUB=VS Enterprise"* ]]
  [[ "$output" == *"AZ_EXPECT_USER=simon@onenrg.onmicrosoft.com"* ]]
}

@test "azrl_save_conf: falls back to tenantId when no default domain" {
  acct='{"tenantId":"guid-2","name":"sub","user":{"name":"u@x"}}'
  doms='{"value":[]}'
  run azrl_save_conf "$acct" "$doms"
  [ "$status" -eq 0 ]
  [[ "$output" == *"AZ_TENANT=guid-2"* ]]
  [[ "$output" == *"AZ_TENANT_ID=guid-2"* ]]
}

@test "azrl --list: prints configured profiles and exits 0" {
  home="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles"
  printf 'AZ_TENANT=fiig.com.au\n' > "$home/.azure-profiles/fiig.conf"
  HOME="$home" run "${BATS_TEST_DIRNAME}/../azrl" --list
  [ "$status" -eq 0 ]
  [[ "$output" == *"fiig"* ]]
  [[ "$output" == *"fiig.com.au"* ]]
  rm -rf "$home"
}

@test "azrl --save: writes a profile conf and .azprofile" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/nrg"
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *"account show"*)      echo '{"tenantId":"guid-1","name":"nrgpov01","user":{"name":"u@onenrg.onmicrosoft.com"}}' ;;
  *"rest"*"domains"*)    echo '{"value":[{"id":"onenrg.onmicrosoft.com","isDefault":true}]}' ;;
  *)                     echo '{}' ;;
esac
EOF
  chmod +x "$shimdir/az"
  work="$(mktemp -d)"
  HOME="$home" PATH="$shimdir:$PATH" run bash -c "cd '$work' && '${BATS_TEST_DIRNAME}/../azrl' --save nrg"
  [ "$status" -eq 0 ]
  [ -f "$home/.azure-profiles/nrg.conf" ]
  grep -q 'AZ_TENANT=onenrg.onmicrosoft.com' "$home/.azure-profiles/nrg.conf"
  grep -q 'AZ_TENANT_ID=guid-1' "$home/.azure-profiles/nrg.conf"
  grep -q 'AZ_EXPECT_USER=u@onenrg.onmicrosoft.com' "$home/.azure-profiles/nrg.conf"
  [ "$(cat "$work/.azprofile")" = "nrg" ]
  rm -rf "$home" "$shimdir" "$work"
}

@test "azrl --save: refuses to clobber an existing conf" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/nrg"
  printf 'AZ_TENANT=keep.me\n' > "$home/.azure-profiles/nrg.conf"
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
echo '{}'
EOF
  chmod +x "$shimdir/az"
  HOME="$home" PATH="$shimdir:$PATH" run "${BATS_TEST_DIRNAME}/../azrl" --save nrg
  [ "$status" -ne 0 ]
  grep -q 'AZ_TENANT=keep.me' "$home/.azure-profiles/nrg.conf"
  rm -rf "$home" "$shimdir"
}

@test "azrl_sanitize_name: lowercases and dashes spaces" {
  run azrl_sanitize_name "Contoso Migration"
  [ "$status" -eq 0 ]
  [ "$output" = "contoso-migration" ]
}

@test "azrl_sanitize_name: collapses junk runs and trims edges" {
  run azrl_sanitize_name "  --Foo__Bar!!  "
  [ "$output" = "foo__bar" ]
}

@test "azrl_default_name: explicit arg used verbatim" {
  run azrl_default_name "My Profile" "/home/x/whatever"
  [ "$output" = "My Profile" ]
}

@test "azrl_default_name: empty arg falls back to sanitized basename" {
  run azrl_default_name "" "/home/x/Contoso Migration"
  [ "$output" = "contoso-migration" ]
}

@test "azrl_write_profile: writes conf and .azprofile from session" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/acme"
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *"account show"*)   echo '{"tenantId":"guid-9","id":"sub-1","name":"Sub","user":{"name":"u@acme.onmicrosoft.com"}}' ;;
  *"rest"*"domains"*) echo '{"value":[{"id":"acme.onmicrosoft.com","isDefault":true}]}' ;;
  *)                  echo '{}' ;;
esac
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export HOME='$home' AZURE_CONFIG_DIR='$home/.azure-profiles/acme'
    PATH='$shimdir':\$PATH azrl_write_profile acme '$work'
  "
  [ "$status" -eq 0 ]
  grep -q 'AZ_TENANT=acme.onmicrosoft.com' "$home/.azure-profiles/acme.conf"
  grep -q 'AZ_TENANT_ID=guid-9' "$home/.azure-profiles/acme.conf"
  [ "$(cat "$work/.azprofile")" = "acme" ]
  rm -rf "$home" "$shimdir" "$work"
}

@test "azrl_write_profile: fails clearly when not logged in" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export HOME='$home' AZURE_CONFIG_DIR='$home/.azure-profiles/acme'
    PATH='$shimdir':\$PATH azrl_write_profile acme '$work'
  "
  [ "$status" -ne 0 ]
  [[ "$output" == *"not logged in"* ]]
  [ ! -e "$home/.azure-profiles/acme.conf" ]
  [ ! -e "$work/.azprofile" ]
  rm -rf "$home" "$shimdir" "$work"
}

@test "azrl_write_profile: refuses to clobber existing conf" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles"
  printf 'AZ_TENANT=keep.me\n' > "$home/.azure-profiles/acme.conf"
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
echo '{}'
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export HOME='$home' AZURE_CONFIG_DIR='$home/.azure-profiles/acme'
    PATH='$shimdir':\$PATH azrl_write_profile acme '$work'
  "
  [ "$status" -ne 0 ]
  grep -q 'AZ_TENANT=keep.me' "$home/.azure-profiles/acme.conf"
  [ ! -e "$work/.azprofile" ]
  rm -rf "$home" "$shimdir" "$work"
}

@test "azrl_write_profile: succeeds when ~/.azure-profiles dir does not pre-exist" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"
  # Deliberately do NOT create $home/.azure-profiles
  cat > "$shimdir/az" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *"account show"*)   echo '{"tenantId":"guid-9","id":"sub-1","name":"Sub","user":{"name":"u@acme.onmicrosoft.com"}}' ;;
  *"rest"*"domains"*) echo '{"value":[{"id":"acme.onmicrosoft.com","isDefault":true}]}' ;;
  *)                  echo '{}' ;;
esac
EOF
  chmod +x "$shimdir/az"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    export HOME='$home' AZURE_CONFIG_DIR='$home/.azure-profiles/acme'
    PATH='$shimdir':\$PATH azrl_write_profile acme '$work'
  "
  [ "$status" -eq 0 ]
  grep -q 'AZ_TENANT=acme.onmicrosoft.com' "$home/.azure-profiles/acme.conf"
  grep -q 'AZ_TENANT_ID=guid-9' "$home/.azure-profiles/acme.conf"
  [ "$(cat "$work/.azprofile")" = "acme" ]
  rm -rf "$home" "$shimdir" "$work"
}

@test "azrl --init: tenant-less login then writes conf and .azprofile" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"; log="$shimdir/az.log"
  mkdir -p "$home/.azure-profiles"
  cat > "$home/.azure-profiles/azrl.conf" <<'EOF'
LOCAL_HOST=localhost
LOCAL_BROWSER_CMD=true
VM_HOST=vm
EOF
  cat > "$shimdir/az" <<EOF
#!/usr/bin/env bash
echo "\$*" >> "$log"
case "\$*" in
  *"login"*)
    url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F'
    cmd="\${BROWSER/\\%s/\$url}"; eval "\$cmd"; exit 0 ;;
  *"account show"*)   echo '{"tenantId":"guid-7","id":"sub-7","name":"Sub","user":{"name":"u@boot.onmicrosoft.com"}}' ;;
  *"rest"*"domains"*) echo '{"value":[{"id":"boot.onmicrosoft.com","isDefault":true}]}' ;;
  *) echo '{}' ;;
esac
EOF
  chmod +x "$shimdir/az"
  cat > "$shimdir/ssh" <<'EOF'
#!/usr/bin/env bash
for a in "$@"; do [[ "$a" == "-R" ]] && { sleep 1; exit 0; }; done
exit 0
EOF
  chmod +x "$shimdir/ssh"
  HOME="$home" PATH="$shimdir:$PATH" AZRL_CAPTURE="${BATS_TEST_DIRNAME}/../azrl-capture" \
    run bash -c "cd '$work' && '${BATS_TEST_DIRNAME}/../azrl' --init boot"
  [ "$status" -eq 0 ]
  run grep -q -- '--tenant' "$log"
  [ "$status" -ne 0 ]
  grep -q 'AZ_TENANT=boot.onmicrosoft.com' "$home/.azure-profiles/boot.conf"
  grep -q 'AZ_TENANT_ID=guid-7' "$home/.azure-profiles/boot.conf"
  [ "$(cat "$work/.azprofile")" = "boot" ]
  rm -rf "$home" "$shimdir" "$work"
}

@test "azrl: no profile -> tenant-less sign-in into default config" {
  home="$(mktemp -d)"; shimdir="$(mktemp -d)"; work="$(mktemp -d)"; log="$shimdir/az.log"
  mkdir -p "$home/.azure-profiles"
  cat > "$home/.azure-profiles/azrl.conf" <<'EOF'
LOCAL_HOST=localhost
LOCAL_BROWSER_CMD=true
VM_HOST=vm
EOF
  cat > "$shimdir/az" <<EOF
#!/usr/bin/env bash
echo "\$*" >> "$log"
case "\$*" in
  *"login"*)
    url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F'
    cmd="\${BROWSER/\\%s/\$url}"; eval "\$cmd"; exit 0 ;;
  *"account show"*) echo '{"tenantId":"g","tenantDefaultDomain":"d","name":"s","user":{"name":"u@x"}}' ;;
  *) echo '{}' ;;
esac
EOF
  chmod +x "$shimdir/az"
  # ssh shim: reachability + reverse tunnel both succeed quickly.
  cat > "$shimdir/ssh" <<'EOF'
#!/usr/bin/env bash
for a in "$@"; do [[ "$a" == "-R" ]] && { sleep 1; exit 0; }; done
exit 0
EOF
  chmod +x "$shimdir/ssh"
  HOME="$home" PATH="$shimdir:$PATH" AZRL_CAPTURE="${BATS_TEST_DIRNAME}/../azrl-capture" \
    run bash -c "cd '$work' && '${BATS_TEST_DIRNAME}/../azrl'"
  [ "$status" -eq 0 ]
  grep -q 'login' "$log"
  run grep -q -- '--tenant' "$log"
  [ "$status" -ne 0 ]
  [ ! -e "$work/.azprofile" ]
  rm -rf "$home" "$shimdir" "$work"
}

@test "azrl_rm_profile: removes conf, dir, and matching .azprofile (assume_yes)" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/acme"
  printf 'AZ_TENANT=acme.com\n' > "$home/.azure-profiles/acme.conf"
  printf 'acme\n' > "$work/.azprofile"
  run azrl_rm_profile acme "$home/.azure-profiles" "$work" 1
  [ "$status" -eq 0 ]
  [ ! -e "$home/.azure-profiles/acme.conf" ]
  [ ! -e "$home/.azure-profiles/acme" ]
  [ ! -e "$work/.azprofile" ]
  rm -rf "$home" "$work"
}

@test "azrl_rm_profile: leaves a non-matching .azprofile untouched" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles"
  printf 'AZ_TENANT=acme.com\n' > "$home/.azure-profiles/acme.conf"
  printf 'other\n' > "$work/.azprofile"
  run azrl_rm_profile acme "$home/.azure-profiles" "$work" 1
  [ "$status" -eq 0 ]
  [ ! -e "$home/.azure-profiles/acme.conf" ]
  [ -f "$work/.azprofile" ]
  [ "$(cat "$work/.azprofile")" = "other" ]
  rm -rf "$home" "$work"
}

@test "azrl_rm_profile: nothing to remove returns 0 with message" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles"
  run azrl_rm_profile ghost "$home/.azure-profiles" "$work" 1
  [ "$status" -eq 0 ]
  [[ "$output" == *"nothing to remove"* ]]
  rm -rf "$home" "$work"
}

@test "azrl_rm_profile: declines on 'n', removes nothing, returns 1" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/acme"
  printf 'AZ_TENANT=acme.com\n' > "$home/.azure-profiles/acme.conf"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    printf 'n\n' | azrl_rm_profile acme '$home/.azure-profiles' '$work' 0
  "
  [ "$status" -eq 1 ]
  [ -f "$home/.azure-profiles/acme.conf" ]
  [ -d "$home/.azure-profiles/acme" ]
  rm -rf "$home" "$work"
}

@test "azrl_rm_profile: confirms on 'y', removes, returns 0" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/acme"
  printf 'AZ_TENANT=acme.com\n' > "$home/.azure-profiles/acme.conf"
  run bash -c "
    source '${BATS_TEST_DIRNAME}/../azrl-lib.sh'
    printf 'y\n' | azrl_rm_profile acme '$home/.azure-profiles' '$work' 0
  "
  [ "$status" -eq 0 ]
  [ ! -e "$home/.azure-profiles/acme.conf" ]
  [ ! -e "$home/.azure-profiles/acme" ]
  rm -rf "$home" "$work"
}

@test "azrl --rm: requires a profile name (exit 2)" {
  home="$(mktemp -d)"
  HOME="$home" run "${BATS_TEST_DIRNAME}/../azrl" --rm
  [ "$status" -eq 2 ]
  rm -rf "$home"
}

@test "azrl --rm: removes a named profile with -y" {
  home="$(mktemp -d)"; work="$(mktemp -d)"
  mkdir -p "$home/.azure-profiles/acme"
  printf 'AZ_TENANT=acme.com\n' > "$home/.azure-profiles/acme.conf"
  HOME="$home" run bash -c "cd '$work' && '${BATS_TEST_DIRNAME}/../azrl' --rm acme -y"
  [ "$status" -eq 0 ]
  [ ! -e "$home/.azure-profiles/acme.conf" ]
  [ ! -e "$home/.azure-profiles/acme" ]
  rm -rf "$home" "$work"
}

@test "azrl --rm: refuses the reserved name azrl (exit 2)" {
  home="$(mktemp -d)"
  HOME="$home" run "${BATS_TEST_DIRNAME}/../azrl" --rm azrl -y
  [ "$status" -eq 2 ]
  rm -rf "$home"
}
