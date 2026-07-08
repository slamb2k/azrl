package aws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteDefaultSection(t *testing.T) {
	existing := "[profile work]\nsso_session = work\n\n[default]\nregion = old\nsso_start_url = old\n\n[sso-session work]\nsso_start_url = https://work\n"
	got := RewriteDefaultSection(existing, "[default]\nregion = new\n")
	if strings.Contains(got, "old") {
		t.Fatalf("old [default] body should be gone:\n%s", got)
	}
	for _, keep := range []string{"[profile work]", "[sso-session work]", "https://work", "region = new"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("missing %q:\n%s", keep, got)
		}
	}
	// No existing [default]: appended.
	got = RewriteDefaultSection("[profile work]\nsso_session = work\n", "[default]\nregion = new\n")
	if !strings.Contains(got, "[profile work]") || !strings.HasSuffix(got, "[default]\nregion = new\n") {
		t.Fatalf("stanza should append after existing sections:\n%s", got)
	}
	// Empty file: just the stanza.
	if got := RewriteDefaultSection("", "[default]\nregion = new\n"); got != "[default]\nregion = new\n" {
		t.Fatalf("empty file: %q", got)
	}
}

func TestSetDefaultProfile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	t.Setenv("AWS_CONFIG_FILE", cfg)
	os.WriteFile(cfg, []byte("[profile work]\nsso_session = work\n"), 0o644)
	c := Conf{SSOStartURL: "https://acme.awsapps.com/start", SSORegion: "ap-southeast-2", AccountID: "111122223333", RoleName: "Dev"}
	path, err := SetDefaultProfile(c)
	if err != nil || path != cfg {
		t.Fatalf("SetDefaultProfile: %v (path %q)", err, path)
	}
	b, _ := os.ReadFile(cfg)
	for _, want := range []string{"[profile work]", "[default]", "sso_start_url = https://acme.awsapps.com/start", "sso_account_id = 111122223333", "sso_role_name = Dev"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("config missing %q:\n%s", want, b)
		}
	}
	// Missing SSO fields refuse before touching anything.
	if _, err := SetDefaultProfile(Conf{SSOStartURL: "x"}); err == nil {
		t.Fatal("incomplete conf should refuse")
	}
}
