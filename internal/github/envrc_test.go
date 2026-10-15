package github

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvrcContent(t *testing.T) {
	if got, want := EnvrcContent("work"), "export GH_CONFIG_DIR=\"$HOME/.github-profiles/work\"\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWriteEnvrcRefusesClobber(t *testing.T) {
	dir := t.TempDir()
	if wrote, err := WriteEnvrc(dir, "work", false); err != nil || !wrote {
		t.Fatalf("first write: wrote=%v err=%v", wrote, err)
	}
	os.WriteFile(filepath.Join(dir, ".envrc"), []byte("keep\n"), 0o644)
	if wrote, err := WriteEnvrc(dir, "other", false); err != nil || wrote {
		t.Fatalf("second write must not clobber: wrote=%v err=%v", wrote, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".envrc")); string(b) != "keep\n" {
		t.Fatalf("clobbered: %q", b)
	}
}
