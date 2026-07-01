package aws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncConfigSharedIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := t.TempDir()
	c := Conf{SSOStartURL: "https://acme.awsapps.com/start", SSORegion: "us-east-1", AccountID: "123456789012", RoleName: "AdminAccess"}

	if err := SyncConfig("work", confdir, c); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".aws", "config")
	b, _ := os.ReadFile(path)
	out := string(b)
	for _, want := range []string{
		"[sso-session work]", "sso_start_url = https://acme.awsapps.com/start", "sso_region = us-east-1",
		"[profile work]", "sso_session = work", "sso_account_id = 123456789012", "sso_role_name = AdminAccess",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	// Re-running must not duplicate the stanzas.
	if err := SyncConfig("work", confdir, c); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if n := strings.Count(string(b), "[profile work]"); n != 1 {
		t.Fatalf("[profile work] appears %d times, want 1", n)
	}
}

func TestSyncConfigNeverOverwritesExistingProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := t.TempDir()
	path := filepath.Join(home, ".aws", "config")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("[profile work]\nregion = eu-west-1\n# hand tuned\n"), 0o644)

	c := Conf{SSOStartURL: "https://acme.awsapps.com/start", SSORegion: "us-east-1", AccountID: "1", RoleName: "Admin"}
	if err := SyncConfig("work", confdir, c); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	out := string(b)
	if !strings.Contains(out, "region = eu-west-1") || !strings.Contains(out, "# hand tuned") {
		t.Fatalf("existing profile block was overwritten:\n%s", out)
	}
	if strings.Contains(out, "sso_account_id") {
		t.Fatalf("should not have rewritten the existing [profile work]:\n%s", out)
	}
}

func TestSyncConfigPreservesExistingFileMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := t.TempDir()
	path := filepath.Join(home, ".aws", "config")
	os.MkdirAll(filepath.Dir(path), 0o755)
	// Pre-existing config with the usual 0644 mode and an unrelated profile so the
	// sync actually appends (and rewrites) the file.
	os.WriteFile(path, []byte("[profile other]\nregion = eu-west-1\n"), 0o644)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	c := Conf{SSOStartURL: "https://acme.awsapps.com/start", SSORegion: "us-east-1", AccountID: "1", RoleName: "Admin"}
	if err := SyncConfig("work", confdir, c); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Fatalf("config mode = %o after sync, want 0644 (temp+rename downgraded it)", got)
	}
}

func TestSyncConfigIsolateWritesToConfdir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := t.TempDir()
	c := Conf{SSOStartURL: "https://acme.awsapps.com/start", SSORegion: "us-east-1", AccountID: "1", RoleName: "Admin", Isolate: true}
	if err := SyncConfig("work", confdir, c); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(confdir, "work", "config")); err != nil {
		t.Fatalf("isolated config not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".aws", "config")); !os.IsNotExist(err) {
		t.Fatal("isolated sync must not touch the shared ~/.aws/config")
	}
}
