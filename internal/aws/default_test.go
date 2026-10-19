package aws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteDefaultSection(t *testing.T) {
	stanza := "[default]\nsso_start_url = https://new\nsso_region = us-east-1\nsso_account_id = 111\nsso_role_name = Dev\nregion = us-east-1\n"
	existing := "[profile work]\nsso_session = work\n\n" +
		"[default]\n# my defaults\nregion = ap-southeast-2\noutput = json\nsso_start_url = https://old\n" +
		"role_arn = arn:aws:iam::1:role/x\nsource_profile = work\ns3 =\n    max_concurrent_requests = 20\n" +
		"credential_process = /bin/creds\n    --flag\ncli_pager =\n\n" +
		"[sso-session work]\nsso_start_url = https://work\n"
	got := RewriteDefaultSection(existing, stanza)
	for _, gone := range []string{"https://old", "role_arn", "source_profile", "credential_process", "--flag"} {
		if strings.Contains(got, gone) {
			t.Fatalf("%q should be dropped from [default]:\n%s", gone, got)
		}
	}
	for _, keep := range []string{
		"[profile work]\nsso_session = work", "[sso-session work]\nsso_start_url = https://work",
		"# my defaults", "region = ap-southeast-2", "output = json", "s3 =\n    max_concurrent_requests = 20", "cli_pager =",
		"sso_start_url = https://new", "sso_account_id = 111", "sso_role_name = Dev",
	} {
		if !strings.Contains(got, keep) {
			t.Fatalf("missing %q:\n%s", keep, got)
		}
	}
	if strings.Contains(got, "\nregion = us-east-1") {
		t.Fatalf("the user's own [default] region must win:\n%s", got)
	}
	if strings.Count(got, "[default]") != 1 {
		t.Fatalf("exactly one [default]:\n%s", got)
	}
	// [default] without a region gets the SSO region as a fallback.
	if got := RewriteDefaultSection("[default]\noutput = json\n", stanza); !strings.Contains(got, "\nregion = us-east-1") || !strings.Contains(got, "output = json") {
		t.Fatalf("fallback region: %q", got)
	}
	// Idempotent.
	once := RewriteDefaultSection(existing, stanza)
	if twice := RewriteDefaultSection(once, stanza); twice != once {
		t.Fatalf("not idempotent:\n%s\n---\n%s", once, twice)
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

// TestSetDefaultProfileKeepsModeSymlinkAndBackup proves the rewrite writes
// through a symlinked config, keeps its permissions, and saves the original.
func TestSetDefaultProfileKeepsModeSymlinkAndBackup(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles-aws-config")
	orig := "[default]\noutput = text\n"
	os.WriteFile(real, []byte(orig), 0o640)
	link := filepath.Join(dir, "config")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	t.Setenv("AWS_CONFIG_FILE", link)
	c := Conf{SSOStartURL: "https://acme.awsapps.com/start", SSORegion: "ap-southeast-2", AccountID: "111122223333", RoleName: "Dev"}
	if _, err := SetDefaultProfile(c); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink must survive the rewrite")
	}
	fi, _ := os.Stat(real)
	if fi.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, want 0640", fi.Mode().Perm())
	}
	b, _ := os.ReadFile(real)
	if !strings.Contains(string(b), "output = text") || !strings.Contains(string(b), "sso_account_id = 111122223333") {
		t.Fatalf("target not merged:\n%s", b)
	}
	if bak, _ := os.ReadFile(real + ".azrl.bak"); string(bak) != orig {
		t.Fatalf("backup = %q, want the original", bak)
	}
}

// TestNativeConfigPathIgnoresIsolatedProfileFile proves an AWS_CONFIG_FILE
// exported for an isolated profile (inside azrl's profiles dir) is not
// mistaken for the native config.
func TestNativeConfigPathIgnoresIsolatedProfileFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(home, ".aws-profiles", "work", "config"))
	if got, want := NativeConfigPath(), filepath.Join(home, ".aws", "config"); got != want {
		t.Fatalf("NativeConfigPath = %q, want %q", got, want)
	}
	custom := filepath.Join(home, "my-aws-config")
	t.Setenv("AWS_CONFIG_FILE", custom)
	if got := NativeConfigPath(); got != custom {
		t.Fatalf("a user's own AWS_CONFIG_FILE must still be honoured, got %q", got)
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
