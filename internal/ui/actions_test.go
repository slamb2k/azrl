package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunUseProducesMsg(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	cmd := runUse("acme")
	msg := cmd()
	done, ok := msg.(opDoneMsg)
	if !ok || done.err != nil {
		t.Fatalf("msg=%v ok=%v", msg, ok)
	}
	if b, _ := os.ReadFile(filepath.Join(work, ".azprofile")); string(b) != "acme\n" {
		t.Fatalf("azprofile=%q", string(b))
	}
}

func TestRunWriteEnvrc(t *testing.T) {
	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	if done, ok := runWriteEnvrc()().(opDoneMsg); !ok || done.err != nil {
		t.Fatalf("write: msg ok=%v err=%v", ok, done.err)
	}
	if _, err := os.Stat(filepath.Join(work, ".envrc")); err != nil {
		t.Fatalf(".envrc not written: %v", err)
	}
	// second call must not clobber and should report so
	if done, ok := runWriteEnvrc()().(opDoneMsg); !ok || done.err != nil {
		t.Fatalf("second: ok=%v err=%v", ok, done.err)
	}
}

func TestRunDeleteProducesMsg(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles", "acme"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	old, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(old)

	msg := runDelete("acme")()
	if done, ok := msg.(opDoneMsg); !ok || done.err != nil {
		t.Fatalf("msg=%v ok=%v", msg, ok)
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); !os.IsNotExist(err) {
		t.Fatal("conf not removed")
	}
}
