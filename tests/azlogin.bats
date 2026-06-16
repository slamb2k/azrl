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
