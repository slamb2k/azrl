package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/envdetect"
)

func TestRecommendedGlobal(t *testing.T) {
	cands := []envdetect.Candidate{
		{Mode: envdetect.Remote, VMSSHHost: "vm", Recommended: true},
		{Mode: envdetect.Local, BrowserCmd: "wslview", BrowserHost: "localhost"},
	}
	// Without fillDefault the remote recommendation keeps its empty BrowserCmd.
	if g := recommendedGlobal(cands, false); g.BrowserCmd != "" || g.VMSSHHost != "vm" {
		t.Fatalf("no-fill = %+v", g)
	}
	// With fillDefault (the --yes path) an empty BrowserCmd falls back to xdg-open.
	if g := recommendedGlobal(cands, true); g.BrowserCmd != "xdg-open" {
		t.Fatalf("fill default = %+v", g)
	}
	// A local recommendation keeps its own command untouched.
	local := []envdetect.Candidate{{Mode: envdetect.Local, BrowserCmd: "open", BrowserHost: "localhost", Recommended: true}}
	if g := recommendedGlobal(local, true); g.BrowserCmd != "open" {
		t.Fatalf("local fill = %+v", g)
	}
}

func TestWriteConfBacksUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "azrl.conf")
	if err := os.WriteFile(path, []byte("BROWSER_CMD=old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	g := config.Global{BrowserCmd: "wslview", BrowserHost: "localhost"}
	if err := writeConf(dir, g, &out); err != nil {
		t.Fatal(err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil || !strings.Contains(string(bak), "old") {
		t.Fatalf("backup missing/wrong: %q err=%v", bak, err)
	}
	if !strings.Contains(out.String(), "local mode") {
		t.Fatalf("summary should note mode: %q", out.String())
	}
	t.Setenv("AZRL_BROWSER_CMD", "")
	got, err := config.LoadGlobal(dir)
	if err != nil || got.BrowserCmd != "wslview" {
		t.Fatalf("reload = %+v err=%v", got, err)
	}
}

func TestPrintResolved(t *testing.T) {
	var out bytes.Buffer
	printResolved(&out, config.Global{BrowserCmd: "open", VMSSHHost: "vm"})
	s := out.String()
	for _, want := range []string{"BROWSER_CMD=open", "BROWSER_HOST=", "VM_SSH_HOST=vm"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
}

// TestLoadGlobalOrSetupNonTTY: with no config and no TTY, the nudge returns an
// error that points the user at `azrl setup` rather than launching the wizard.
func TestLoadGlobalOrSetupNonTTY(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := isInteractive
	isInteractive = func() bool { return false }
	defer func() { isInteractive = prev }()

	var out bytes.Buffer
	_, err := loadGlobalOrSetup(&out)
	if err == nil || !strings.Contains(err.Error(), "azrl setup") {
		t.Fatalf("expected setup hint, got %v", err)
	}
}

// TestLoadGlobalOrSetupPlaceholderNonTTY: a placeholder config is treated as
// unconfigured and nudges too.
func TestLoadGlobalOrSetupPlaceholderNonTTY(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AZRL_BROWSER_CMD", "")
	dir := filepath.Join(home, ".azure-profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	conf := "BROWSER_CMD=xdg-open\nBROWSER_HOST=my-laptop\nVM_SSH_HOST=my-vm\n"
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := isInteractive
	isInteractive = func() bool { return false }
	defer func() { isInteractive = prev }()

	var out bytes.Buffer
	if _, err := loadGlobalOrSetup(&out); err == nil || !strings.Contains(err.Error(), "azrl setup") {
		t.Fatalf("placeholder should nudge, got %v", err)
	}
}

// TestLoadGlobalOrSetupValid: a real config loads without touching the wizard.
func TestLoadGlobalOrSetupValid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AZRL_BROWSER_CMD", "")
	dir := filepath.Join(home, ".azure-profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte("BROWSER_CMD=wslview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	g, err := loadGlobalOrSetup(&out)
	if err != nil || g.BrowserCmd != "wslview" {
		t.Fatalf("valid config = %+v err=%v", g, err)
	}
}
