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

func TestOfferEnvrcWritesBesideAzprofile(t *testing.T) {
	t.Setenv("PATH", "") // no direnv on PATH — skip the allow side effect
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".azprofile"), []byte("acme\n"), 0o644)
	sub := filepath.Join(root, "src")
	os.MkdirAll(sub, 0o755)

	var out strings.Builder
	offerEnvrc(sub, &out, strings.NewReader("y\n"))

	if _, err := os.Stat(filepath.Join(root, ".envrc")); err != nil {
		t.Fatalf(".envrc not written beside .azprofile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, ".envrc")); err == nil {
		t.Fatal(".envrc wrongly written in the subdir")
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
