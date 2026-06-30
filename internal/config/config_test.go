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
