package gcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfWriteAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.conf")
	c := Conf{
		ConfigName:    "work",
		Project:       "acme-prod",
		Region:        "us-central1",
		ExpectAccount: "simon@acme.com",
		Label:         "Acme Prod",
		Isolate:       true,
		BrowserCmd:    "chrome-work",
		BrowserLabel:  "Edge — Work",
	}
	if err := c.Write(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConf("work", dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != c {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, c)
	}
}

func TestLoadConfRequiresProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.conf"), []byte("GCP_CONFIG_NAME=work\n"), 0o644)
	if _, err := LoadConf("bad", dir); err == nil {
		t.Fatal("expected error when GCP_PROJECT is unset")
	}
}

func TestSetIsolatePreservesOrderAndOtherKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.conf")
	os.WriteFile(path, []byte("GCP_PROJECT=acme-prod\nLAST_USED=2026-06-01T10:00:00Z\nLAST_DIR=/work/repo\n"), 0o644)
	if err := SetIsolate(dir, "work", true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	out := string(b)
	for _, want := range []string{"GCP_PROJECT=acme-prod", "LAST_USED=2026-06-01T10:00:00Z", "LAST_DIR=/work/repo", "GCP_ISOLATE=true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("SetIsolate dropped %q:\n%s", want, out)
		}
	}
	// Updating an existing key must not duplicate it.
	if err := SetIsolate(dir, "work", false); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if n := strings.Count(string(b), "GCP_ISOLATE="); n != 1 {
		t.Fatalf("GCP_ISOLATE appears %d times, want 1:\n%s", n, b)
	}
}
