package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseKV(t *testing.T) {
	in := "# comment\nAZ_TENANT=acme.com\n\n  AZ_DEFAULT_SUB = sub-1 \n"
	m, err := ParseKV(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if m["AZ_TENANT"] != "acme.com" || m["AZ_DEFAULT_SUB"] != "sub-1" {
		t.Fatalf("got %v", m)
	}
}

func TestLoadGlobal(t *testing.T) {
	dir := t.TempDir()
	conf := "LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm\n"
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGlobal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.LocalHost != "pc" || g.LocalBrowserCmd != "wslview" || g.VMHost != "vm" {
		t.Fatalf("got %+v", g)
	}
}

func TestLoadGlobalMissing(t *testing.T) {
	if _, err := LoadGlobal(t.TempDir()); err == nil {
		t.Fatal("expected error for missing azrl.conf")
	}
}

func TestAwsProfilesDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := AwsProfilesDir(); got != filepath.Join(home, ".aws-profiles") {
		t.Fatalf("AwsProfilesDir = %q", got)
	}
}

func TestGcpProfilesDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := GcpProfilesDir(); got != filepath.Join(home, ".gcp-profiles") {
		t.Fatalf("GcpProfilesDir = %q", got)
	}
}

func TestDashboardPollSecs(t *testing.T) {
	if got := DashboardPollSecs(t.TempDir()); got != 3 {
		t.Fatalf("missing conf: got %d, want 3", got)
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte("DASHBOARD_POLL_SECS=7\n"), 0o644)
	if got := DashboardPollSecs(dir); got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
	bad := t.TempDir()
	os.WriteFile(filepath.Join(bad, "azrl.conf"), []byte("DASHBOARD_POLL_SECS=nope\n"), 0o644)
	if got := DashboardPollSecs(bad); got != 3 {
		t.Fatalf("bad value: got %d, want 3", got)
	}
	zero := t.TempDir()
	os.WriteFile(filepath.Join(zero, "azrl.conf"), []byte("DASHBOARD_POLL_SECS=0\n"), 0o644)
	if got := DashboardPollSecs(zero); got != 3 {
		t.Fatalf("zero value: got %d, want 3", got)
	}
}

func TestEnabledProvidersDefaultAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := EnabledProviders(dir); len(got) != 2 || got[0] != "azure" || got[1] != "github" {
		t.Fatalf("default = %v", got)
	}
	if err := SetEnabledProviders(dir, []string{"azure", "aws"}); err != nil {
		t.Fatal(err)
	}
	if got := EnabledProviders(dir); len(got) != 2 || got[0] != "azure" || got[1] != "aws" {
		t.Fatalf("round trip = %v", got)
	}
}

func TestSetEnabledProvidersPreservesOtherLines(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "azrl.conf")
	os.WriteFile(conf, []byte("# hosts\nLOCAL_HOST=laptop\nPROVIDERS=azure\nVM_HOST=vm\n"), 0o644)
	if err := SetEnabledProviders(dir, []string{"github", "gcp"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(conf)
	s := string(b)
	for _, want := range []string{"# hosts", "LOCAL_HOST=laptop", "PROVIDERS=github,gcp", "VM_HOST=vm"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "PROVIDERS=azure") {
		t.Fatalf("old assignment not replaced:\n%s", s)
	}
}
