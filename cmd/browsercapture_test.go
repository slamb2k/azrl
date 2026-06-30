package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserCaptureWritesURL(t *testing.T) {
	cap := filepath.Join(t.TempDir(), "capfile")
	t.Setenv("AZRL_CAPFILE", cap)
	RootCmd.SetArgs([]string{"__browser-capture", "https://login/x?foo=bar"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(cap)
	if err != nil || string(b) != "https://login/x?foo=bar" {
		t.Fatalf("capfile=%q err=%v", string(b), err)
	}
}
