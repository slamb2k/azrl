package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserShimDeviceRelayPrintsLocalLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AZRL_BROWSER_CMD", "")
	confdir := filepath.Join(home, ".azure-profiles")
	if err := os.MkdirAll(confdir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(confdir, "azrl.conf"),
		[]byte("LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm\n"), 0o644)

	var out bytes.Buffer
	RootCmd.SetOut(&out)
	RootCmd.SetArgs([]string{"__browser", "--paste", "https://github.com/login/device"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "wslview https://github.com/login/device") {
		t.Fatalf("expected local-open line; got %q", out.String())
	}
}

func TestBrowserShimEnvBrowserCmdOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := filepath.Join(home, ".azure-profiles")
	if err := os.MkdirAll(confdir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(confdir, "azrl.conf"),
		[]byte("LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm\n"), 0o644)
	t.Setenv("AZRL_BROWSER_CMD", "chrome-work")

	var out bytes.Buffer
	RootCmd.SetOut(&out)
	RootCmd.SetArgs([]string{"__browser", "--paste", "https://github.com/login/device"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "chrome-work https://github.com/login/device") {
		t.Fatalf("shim should honour AZRL_BROWSER_CMD; got %q", out.String())
	}
}
