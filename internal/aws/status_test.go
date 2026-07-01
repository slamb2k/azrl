package aws_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slamb2k/azrl/internal/aws"
)

func TestStatusReadsIdentityExpiryAndLastUsed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(confdir, 0o755)
	os.WriteFile(filepath.Join(confdir, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_ACCOUNT_ID=123456789012\nAWS_ROLE_NAME=AdminAccess\nLAST_USED=2026-06-01T10:00:00Z\nLAST_DIR=/work/repo\n"), 0o644)

	cache := filepath.Join(home, ".aws", "sso", "cache")
	os.MkdirAll(cache, 0o755)
	exp := time.Now().Add(42 * time.Minute).UTC().Truncate(time.Second)
	os.WriteFile(filepath.Join(cache, "abc.json"),
		[]byte(`{"startUrl":"https://acme.awsapps.com/start","expiresAt":"`+exp.Format(time.RFC3339)+`"}`), 0o644)
	// A cache entry for a different portal must be ignored.
	os.WriteFile(filepath.Join(cache, "other.json"),
		[]byte(`{"startUrl":"https://other.awsapps.com/start","expiresAt":"2099-01-01T00:00:00Z"}`), 0o644)

	st, err := aws.NewProvider().Status("work", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Identity != "123456789012/AdminAccess" {
		t.Fatalf("Identity = %q", st.Identity)
	}
	if st.Directory != "/work/repo" {
		t.Fatalf("Directory = %q", st.Directory)
	}
	if st.LastUsed.IsZero() {
		t.Fatal("LastUsed not read")
	}
	if st.Expiry == nil || !st.Expiry.Equal(exp) {
		t.Fatalf("Expiry = %v, want %v", st.Expiry, exp)
	}
}

func TestStatusBlankOnMissingCacheAndFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(confdir, 0o755)
	os.WriteFile(filepath.Join(confdir, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)
	st, err := aws.NewProvider().Status("work", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Identity != "" || st.Expiry != nil {
		t.Fatalf("expected blank status, got %+v", st)
	}
}

func TestStatusDriftShared(t *testing.T) {
	confdir := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "work.conf"), []byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	cases := []struct {
		name    string
		pin     string // .awsprofile contents; "" means no pointer file
		ambient string // AWS_PROFILE; "" means unset
		want    bool
	}{
		{"ambient unset while pinned drifts", "work", "", true},
		{"ambient equals name is clean", "work", "work", false},
		{"ambient other profile drifts", "work", "other", true},
		{"cwd pins a different profile is clean", "elsewhere", "", false},
		{"cwd not pinned is clean", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pwd := t.TempDir()
			if c.pin != "" {
				os.WriteFile(filepath.Join(pwd, ".awsprofile"), []byte(c.pin+"\n"), 0o644)
			}
			t.Chdir(pwd)
			t.Setenv("AWS_PROFILE", c.ambient)
			st, err := aws.NewProvider().Status("work", confdir)
			if err != nil {
				t.Fatal(err)
			}
			if st.Drifted != c.want {
				t.Fatalf("Drifted = %v, want %v", st.Drifted, c.want)
			}
		})
	}
}

func TestStatusDriftIsolate(t *testing.T) {
	confdir := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_ISOLATE=true\n"), 0o644)
	cfg := filepath.Join(confdir, "work", "config")
	creds := filepath.Join(confdir, "work", "credentials")

	cases := []struct {
		name  string
		pin   string
		cfg   string
		creds string
		want  bool
	}{
		{"both unset while pinned drifts", "work", "", "", true},
		{"both point at isolated files is clean", "work", cfg, creds, false},
		{"only config set drifts", "work", cfg, "", true},
		{"config points elsewhere drifts", "work", "/tmp/other", creds, true},
		{"cwd not pinned is clean", "", cfg, creds, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pwd := t.TempDir()
			if c.pin != "" {
				os.WriteFile(filepath.Join(pwd, ".awsprofile"), []byte(c.pin+"\n"), 0o644)
			}
			t.Chdir(pwd)
			t.Setenv("AWS_CONFIG_FILE", c.cfg)
			t.Setenv("AWS_SHARED_CREDENTIALS_FILE", c.creds)
			st, err := aws.NewProvider().Status("work", confdir)
			if err != nil {
				t.Fatal(err)
			}
			if st.Drifted != c.want {
				t.Fatalf("Drifted = %v, want %v", st.Drifted, c.want)
			}
		})
	}
}
