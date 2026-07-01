package github

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSwitchRecordsCurrentProfile(t *testing.T) {
	profilesDir := t.TempDir()
	os.WriteFile(filepath.Join(profilesDir, "work.conf"), []byte("GH_HOST=github.com\n"), 0o644)

	if Current(profilesDir) != "" {
		t.Fatal("expected no current profile initially")
	}
	if err := Switch(profilesDir, "work"); err != nil {
		t.Fatal(err)
	}
	if got := Current(profilesDir); got != "work" {
		t.Fatalf("Current=%q want work", got)
	}
}

func TestSwitchUnknownProfileErrors(t *testing.T) {
	if err := Switch(t.TempDir(), "ghost"); err == nil {
		t.Fatal("expected error switching to unknown profile")
	}
}
