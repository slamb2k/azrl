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
	t.Setenv("AZRL_BROWSER_CMD", "")
	dir := t.TempDir()
	conf := "BROWSER_HOST=pc\nBROWSER_CMD=wslview\nVM_SSH_HOST=vm\n"
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGlobal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.BrowserHost != "pc" || g.BrowserCmd != "wslview" || g.VMSSHHost != "vm" {
		t.Fatalf("got %+v", g)
	}
}

func TestLoadGlobalMissing(t *testing.T) {
	if _, err := LoadGlobal(t.TempDir()); err == nil {
		t.Fatal("expected error for missing azrl.conf")
	}
}

// TestLoadGlobalLegacyAliases verifies the old three-key config still populates
// the new fields via alias fallback (AC-02).
func TestLoadGlobalLegacyAliases(t *testing.T) {
	t.Setenv("AZRL_BROWSER_CMD", "")
	dir := t.TempDir()
	conf := "LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm\n"
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGlobal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.BrowserHost != "pc" || g.BrowserCmd != "wslview" || g.VMSSHHost != "vm" {
		t.Fatalf("alias fallback failed: %+v", g)
	}
}

// TestLoadGlobalNewKeyWins checks a new key overrides its legacy alias when both
// are present.
func TestLoadGlobalNewKeyWins(t *testing.T) {
	t.Setenv("AZRL_BROWSER_CMD", "")
	dir := t.TempDir()
	conf := "LOCAL_BROWSER_CMD=old\nBROWSER_CMD=new\n"
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGlobal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.BrowserCmd != "new" {
		t.Fatalf("new key should win: %+v", g)
	}
}

func TestIsLocal(t *testing.T) {
	for _, host := range []string{"", "localhost", "127.0.0.1"} {
		if !(Global{BrowserHost: host}).IsLocal() {
			t.Errorf("BrowserHost=%q should be local", host)
		}
	}
	if (Global{BrowserHost: "my-laptop"}).IsLocal() {
		t.Error("a named host must not be local")
	}
	// Tightened: a VM_SSH_HOST-only box is remote even with a local-ish host.
	if (Global{BrowserHost: "localhost", VMSSHHost: "my-vm"}).IsLocal() {
		t.Error("VM_SSH_HOST set must force remote")
	}
}

func TestIsPlaceholder(t *testing.T) {
	if !IsPlaceholder(Global{BrowserHost: "my-laptop"}) {
		t.Error("my-laptop host is a placeholder")
	}
	if !IsPlaceholder(Global{VMSSHHost: "my-vm"}) {
		t.Error("my-vm host is a placeholder")
	}
	if IsPlaceholder(Global{BrowserCmd: "wslview", BrowserHost: "localhost"}) {
		t.Error("a real config is not a placeholder")
	}
}

func TestLoadGlobalLocalMode(t *testing.T) {
	t.Setenv("AZRL_BROWSER_CMD", "")
	dir := t.TempDir()
	// Local mode: only BROWSER_CMD, no host keys (AC-01).
	conf := "BROWSER_CMD=wslview\n"
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGlobal(dir)
	if err != nil {
		t.Fatalf("local mode should validate without hosts: %v", err)
	}
	if !g.IsLocal() || g.BrowserCmd != "wslview" {
		t.Fatalf("got %+v", g)
	}
}

func TestLoadGlobalMissingBrowserCmd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte("BROWSER_HOST=localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadGlobal(dir); err == nil {
		t.Fatal("expected error when BROWSER_CMD is unset")
	}
}

func TestLoadGlobalBrowserCmdEnvOverride(t *testing.T) {
	dir := t.TempDir()
	conf := "BROWSER_HOST=pc\nBROWSER_CMD=wslview\nVM_SSH_HOST=vm\n"
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AZRL_BROWSER_CMD", "chrome-work")
	g, err := LoadGlobal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.BrowserCmd != "chrome-work" {
		t.Fatalf("env override not applied: %+v", g)
	}
}

// TestParseKVStripsInlineComment guards the fix for a value like
// `wslview   # opens a URL` leaking its comment into the parsed value.
func TestParseKVStripsInlineComment(t *testing.T) {
	m, err := ParseKV(strings.NewReader("BROWSER_CMD=wslview   # opens a URL\nX=a#b\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m["BROWSER_CMD"] != "wslview" {
		t.Fatalf("inline comment not stripped: %q", m["BROWSER_CMD"])
	}
	if m["X"] != "a#b" {
		t.Fatalf("literal # (no preceding space) should be kept: %q", m["X"])
	}
}

// TestGlobalWriteRoundTrip verifies Write→LoadGlobal is lossless for both modes.
func TestGlobalWriteRoundTrip(t *testing.T) {
	t.Setenv("AZRL_BROWSER_CMD", "")
	for _, g := range []Global{
		{BrowserCmd: "wslview", BrowserHost: "localhost"},
		{BrowserCmd: "open", VMSSHHost: "203.0.113.10"},
		{BrowserCmd: "xdg-open", BrowserHost: "my-laptop", VMSSHHost: "vm-1"},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "azrl.conf")
		if err := g.Write(path); err != nil {
			t.Fatalf("write %+v: %v", g, err)
		}
		got, err := LoadGlobal(dir)
		if err != nil {
			t.Fatalf("reload %+v: %v", g, err)
		}
		if got != g {
			t.Fatalf("round trip: wrote %+v, read %+v", g, got)
		}
	}
}

// TestGlobalWritePreservesUnknownKeys verifies a setup re-run keeps PROVIDERS
// and a real DASHBOARD_POLL_SECS rather than dropping them.
func TestGlobalWritePreservesUnknownKeys(t *testing.T) {
	t.Setenv("AZRL_BROWSER_CMD", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "azrl.conf")
	existing := "LOCAL_BROWSER_CMD=old\nPROVIDERS=azure,aws\nDASHBOARD_POLL_SECS=7\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	g := Global{BrowserCmd: "wslview", BrowserHost: "localhost"}
	if err := g.Write(path); err != nil {
		t.Fatal(err)
	}
	if got := EnabledProviders(dir); len(got) != 2 || got[0] != "azure" || got[1] != "aws" {
		t.Fatalf("PROVIDERS not preserved: %v", got)
	}
	if got := DashboardPollSecs(dir); got != 7 {
		t.Fatalf("DASHBOARD_POLL_SECS not preserved: %d", got)
	}
	// The stale legacy alias must not linger now that BROWSER_CMD is canonical.
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "LOCAL_BROWSER_CMD") {
		t.Fatalf("legacy alias should be dropped:\n%s", b)
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
