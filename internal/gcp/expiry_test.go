package gcp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// copyFixtureDB copies the committed access_tokens.db fixture into dir,
// standing in for a real gcloud config dir.
func copyFixtureDB(t *testing.T, dir string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "access_tokens.db"))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, "access_tokens.db"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGcpExpiryReadsPastExpiry(t *testing.T) {
	dir := t.TempDir()
	copyFixtureDB(t, dir)
	got := gcpExpiry(dir, "expired@acme.com")
	want := time.Date(1970, 1, 2, 3, 4, 5, 0, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Fatalf("gcpExpiry = %v, want %v", got, want)
	}
}

func TestGcpExpiryParsesFractionalSeconds(t *testing.T) {
	dir := t.TempDir()
	copyFixtureDB(t, dir)
	got := gcpExpiry(dir, "valid@acme.com")
	want := time.Date(2999, 1, 1, 0, 0, 0, 123456000, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Fatalf("gcpExpiry = %v, want %v", got, want)
	}
}

func TestGcpExpiryNilCases(t *testing.T) {
	dir := t.TempDir()
	copyFixtureDB(t, dir)
	cases := map[string]struct{ dir, account string }{
		"unknown account": {dir, "nobody@acme.com"},
		"NULL expiry":     {dir, "nullexp@acme.com"},
		"blank account":   {dir, ""},
		"absent file":     {t.TempDir(), "expired@acme.com"},
	}
	for name, c := range cases {
		if got := gcpExpiry(c.dir, c.account); got != nil {
			t.Fatalf("%s: gcpExpiry = %v, want nil", name, got)
		}
	}
}

func TestGcpExpiryCorruptFileIsNil(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "access_tokens.db"), []byte("not a sqlite file"), 0o644)
	if got := gcpExpiry(dir, "expired@acme.com"); got != nil {
		t.Fatalf("gcpExpiry on corrupt db = %v, want nil", got)
	}
}
