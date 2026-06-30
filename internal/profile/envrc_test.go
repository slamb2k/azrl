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
