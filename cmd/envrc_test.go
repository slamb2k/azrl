package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfferEnvrcYesWrites(t *testing.T) {
	dir := t.TempDir()
	var out strings.Builder
	offerEnvrc(dir, &out, strings.NewReader("y\n"))
	if _, err := os.Stat(filepath.Join(dir, ".envrc")); err != nil {
		t.Fatalf(".envrc not written on yes: %v\nout=%s", err, out.String())
	}
}

func TestOfferEnvrcNoDeclines(t *testing.T) {
	dir := t.TempDir()
	var out strings.Builder
	offerEnvrc(dir, &out, strings.NewReader("n\n"))
	if _, err := os.Stat(filepath.Join(dir, ".envrc")); err == nil {
		t.Fatal(".envrc written despite no")
	}
}

func TestOfferEnvrcNonTTYDeclines(t *testing.T) {
	dir := t.TempDir()
	var out strings.Builder
	offerEnvrc(dir, &out, strings.NewReader("")) // closed/empty stdin
	if _, err := os.Stat(filepath.Join(dir, ".envrc")); err == nil {
		t.Fatal(".envrc written on empty stdin")
	}
}

func TestOfferEnvrcSkipsWhenPresent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".envrc"), []byte("# mine\n"), 0o644)
	var out strings.Builder
	offerEnvrc(dir, &out, strings.NewReader("y\n"))
	if out.Len() != 0 {
		t.Fatalf("should not prompt when .envrc exists: %q", out.String())
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".envrc"))
	if string(b) != "# mine\n" {
		t.Fatalf("existing .envrc clobbered: %q", string(b))
	}
}
