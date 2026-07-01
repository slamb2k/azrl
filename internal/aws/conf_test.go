package aws

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
		SSOStartURL:   "https://acme.awsapps.com/start",
		SSORegion:     "us-east-1",
		AccountID:     "123456789012",
		RoleName:      "AdminAccess",
		ExpectAccount: "123456789012",
		ExpectARN:     "AWSReservedSSO_AdminAccess",
		Label:         "Acme Prod",
		Isolate:       true,
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

func TestLoadConfRequiresStartURL(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.conf"), []byte("AWS_SSO_REGION=us-east-1\n"), 0o644)
	if _, err := LoadConf("bad", dir); err == nil {
		t.Fatal("expected error when AWS_SSO_START_URL is unset")
	}
}

func TestSetConfKeyPreservesOrderAndOtherKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.conf")
	os.WriteFile(path, []byte("AWS_SSO_START_URL=https://x/start\nLAST_USED=2026-06-01T10:00:00Z\nLAST_DIR=/work/repo\n"), 0o644)
	if err := setConfKey(path, "AWS_ISOLATE", "true"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	out := string(b)
	for _, want := range []string{"AWS_SSO_START_URL=https://x/start", "LAST_USED=2026-06-01T10:00:00Z", "LAST_DIR=/work/repo", "AWS_ISOLATE=true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("setConfKey dropped %q:\n%s", want, out)
		}
	}
	// Updating an existing key must not duplicate it.
	if err := setConfKey(path, "AWS_ISOLATE", "false"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if n := strings.Count(string(b), "AWS_ISOLATE="); n != 1 {
		t.Fatalf("AWS_ISOLATE appears %d times, want 1:\n%s", n, b)
	}
}
