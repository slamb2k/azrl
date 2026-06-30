package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteEnvrc(t *testing.T) {
	dir := t.TempDir()
	wrote, err := WriteEnvrc(dir)
	if err != nil || !wrote {
		t.Fatalf("first write: wrote=%v err=%v", wrote, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".envrc"))
	if !strings.Contains(string(b), "AZURE_CONFIG_DIR") || !strings.Contains(string(b), ".azprofile") {
		t.Fatalf("envrc content: %q", string(b))
	}
	if !HasEnvrc(dir) {
		t.Fatal("HasEnvrc false after write")
	}
	// second call must not clobber
	wrote, err = WriteEnvrc(dir)
	if err != nil || wrote {
		t.Fatalf("second write should be no-op: wrote=%v err=%v", wrote, err)
	}
}

func TestHasEnvrcFalse(t *testing.T) {
	if HasEnvrc(t.TempDir()) {
		t.Fatal("HasEnvrc should be false for empty dir")
	}
}

func TestDirenvAllowRunsBinary(t *testing.T) {
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "called")
	// fake direnv records its args so we can assert it was invoked as `allow`.
	shim := "#!/bin/sh\necho \"$@\" > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "direnv"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	work := t.TempDir()
	ran, err := DirenvAllow(work)
	if !ran || err != nil {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
	b, err := os.ReadFile(marker)
	if err != nil || !strings.Contains(string(b), "allow") {
		t.Fatalf("direnv not invoked with allow: %q (%v)", string(b), err)
	}
}

func TestDirenvAllowMissingIsNoError(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no direnv on PATH
	ran, err := DirenvAllow(t.TempDir())
	if ran || err != nil {
		t.Fatalf("missing direnv: ran=%v err=%v", ran, err)
	}
}
