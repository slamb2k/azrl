package profile

import (
	"os"
	"os/exec"
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

func TestLocateAzprofile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".azprofile"), []byte("acme\n"), 0o644)
	sub := filepath.Join(root, "a", "b")
	os.MkdirAll(sub, 0o755)
	if d, ok := LocateAzprofile(sub); !ok || d != root {
		t.Fatalf("from subdir: d=%q ok=%v want %q", d, ok, root)
	}
	if _, ok := LocateAzprofile(t.TempDir()); ok {
		t.Fatal("dir with no .azprofile should not be located")
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

// The guarded stanza is inert when the pointer is missing or names a missing
// profile dir, and exports only when both exist — executed in real bash, the
// same way direnv evaluates it.
func TestEnvrcStanzaGuardsAgainstStalePointer(t *testing.T) {
	run := func(dir, home string) string {
		t.Helper()
		cmd := exec.Command("bash", "-ec", `source .envrc; echo "${AZURE_CONFIG_DIR:-unset}"`)
		cmd.Dir = dir
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("stanza errored (direnv would refuse it): %v\n%s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	home := t.TempDir()
	dir := t.TempDir()
	if _, err := WriteEnvrc(dir); err != nil {
		t.Fatal(err)
	}
	// No .azprofile: inert.
	if got := run(dir, home); got != "unset" {
		t.Fatalf("no pointer should leave AZURE_CONFIG_DIR unset, got %q", got)
	}
	// Pointer naming a missing profile dir: still inert.
	os.WriteFile(filepath.Join(dir, ".azprofile"), []byte("ghost\n"), 0o644)
	if got := run(dir, home); got != "unset" {
		t.Fatalf("missing profile dir should leave AZURE_CONFIG_DIR unset, got %q", got)
	}
	// Pointer + existing profile dir: exports.
	os.MkdirAll(filepath.Join(home, ".azure-profiles", "acme"), 0o755)
	os.WriteFile(filepath.Join(dir, ".azprofile"), []byte("acme\n"), 0o644)
	if got := run(dir, home); got != filepath.Join(home, ".azure-profiles", "acme") {
		t.Fatalf("mapped dir should export the profile config dir, got %q", got)
	}
}

func TestEnvrcWarning(t *testing.T) {
	dir := t.TempDir()
	if w := EnvrcWarning("azure", dir); w != "" {
		t.Fatalf("no .envrc should warn nothing, got %q", w)
	}
	os.WriteFile(EnvrcPath(dir), []byte("export AWS_PROFILE=prod\n"), 0o644)
	if w := EnvrcWarning("aws", dir); !strings.Contains(w, ".envrc") {
		t.Fatalf("aws stanza should warn on aws unmap, got %q", w)
	}
	if w := EnvrcWarning("azure", dir); w != "" {
		t.Fatalf("aws stanza must not warn on azure unmap, got %q", w)
	}
	if w := EnvrcWarning("github", dir); w != "" {
		t.Fatalf("github has no .envrc stanza to warn about, got %q", w)
	}
}
