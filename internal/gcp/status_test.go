package gcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/gcp"
)

func writeConfigINI(t *testing.T, gcloudDir, configName, account string) {
	t.Helper()
	dir := filepath.Join(gcloudDir, "configurations")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "config_"+configName),
		[]byte("[core]\naccount = "+account+"\nproject = acme-prod\n"), 0o644)
}

func TestStatusReadsIdentityAndLastUsedExpiryNil(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := filepath.Join(home, ".gcp-profiles")
	os.MkdirAll(confdir, 0o755)
	os.WriteFile(filepath.Join(confdir, "work.conf"),
		[]byte("GCP_CONFIG_NAME=work\nGCP_PROJECT=acme-prod\nLAST_USED=2026-06-01T10:00:00Z\nLAST_DIR=/work/repo\n"), 0o644)

	writeConfigINI(t, filepath.Join(home, ".config", "gcloud"), "work", "simon@acme.com")

	st, err := gcp.NewProvider().Status("work", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Identity != "simon@acme.com" {
		t.Fatalf("Identity = %q", st.Identity)
	}
	if st.Directory != "/work/repo" {
		t.Fatalf("Directory = %q", st.Directory)
	}
	if st.LastUsed.IsZero() {
		t.Fatal("LastUsed not read")
	}
	if st.Expiry != nil {
		t.Fatalf("Expiry must be nil in v1, got %v", st.Expiry)
	}
}

func TestStatusBlankOnMissingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := filepath.Join(home, ".gcp-profiles")
	os.MkdirAll(confdir, 0o755)
	os.WriteFile(filepath.Join(confdir, "work.conf"), []byte("GCP_PROJECT=acme-prod\n"), 0o644)
	st, err := gcp.NewProvider().Status("work", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Identity != "" || st.Expiry != nil {
		t.Fatalf("expected blank status, got %+v", st)
	}
}

func TestStatusDriftDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := filepath.Join(home, ".gcp-profiles")
	os.MkdirAll(confdir, 0o755)
	os.WriteFile(filepath.Join(confdir, "work.conf"), []byte("GCP_CONFIG_NAME=work\nGCP_PROJECT=acme-prod\n"), 0o644)

	cases := []struct {
		name    string
		pin     string // .gcpprofile contents; "" means no pointer file
		ambient string // CLOUDSDK_ACTIVE_CONFIG_NAME; "" means unset (→ active_config file "default")
		want    bool
	}{
		{"ambient unset while pinned drifts", "work", "", true},
		{"ambient equals config name is clean", "work", "work", false},
		{"ambient other config drifts", "work", "other", true},
		{"cwd pins a different profile is clean", "elsewhere", "", false},
		{"cwd not pinned is clean", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pwd := t.TempDir()
			if c.pin != "" {
				os.WriteFile(filepath.Join(pwd, ".gcpprofile"), []byte(c.pin+"\n"), 0o644)
			}
			t.Chdir(pwd)
			t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", c.ambient)
			st, err := gcp.NewProvider().Status("work", confdir)
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
		[]byte("GCP_CONFIG_NAME=work\nGCP_PROJECT=acme-prod\nGCP_ISOLATE=true\n"), 0o644)
	isoDir := filepath.Join(confdir, "work")

	cases := []struct {
		name   string
		pin    string
		cfgEnv string
		want   bool
	}{
		{"unset while pinned drifts", "work", "", true},
		{"points at isolated dir is clean", "work", isoDir, false},
		{"points elsewhere drifts", "work", "/tmp/other", true},
		{"cwd not pinned is clean", "", isoDir, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pwd := t.TempDir()
			if c.pin != "" {
				os.WriteFile(filepath.Join(pwd, ".gcpprofile"), []byte(c.pin+"\n"), 0o644)
			}
			t.Chdir(pwd)
			t.Setenv("CLOUDSDK_CONFIG", c.cfgEnv)
			st, err := gcp.NewProvider().Status("work", confdir)
			if err != nil {
				t.Fatal(err)
			}
			if st.Drifted != c.want {
				t.Fatalf("Drifted = %v, want %v", st.Drifted, c.want)
			}
		})
	}
}
