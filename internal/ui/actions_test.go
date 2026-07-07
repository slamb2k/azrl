package ui

import (
	"os"
	"path/filepath"
	"testing"
)

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
