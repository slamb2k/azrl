package github_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/github"
)

func TestStatusReadsIdentityFromHostsYml(t *testing.T) {
	confdir := t.TempDir()
	iso := filepath.Join(confdir, "work")
	os.MkdirAll(iso, 0o755)
	os.WriteFile(filepath.Join(confdir, "work.conf"),
		[]byte("GH_HOST=github.com\nLAST_USED=2026-06-01T10:00:00Z\nLAST_DIR=/work/repo\n"), 0o644)
	os.WriteFile(filepath.Join(iso, "hosts.yml"),
		[]byte("github.com:\n    user: octocat\n    oauth_token: gho_x\n"), 0o644)

	st, err := github.NewProvider().Status("work", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Identity != "octocat@github.com" {
		t.Fatalf("Identity = %q", st.Identity)
	}
	if st.Directory != "/work/repo" {
		t.Fatalf("Directory = %q", st.Directory)
	}
	if st.Expiry != nil {
		t.Fatalf("Expiry should be nil, got %v", st.Expiry)
	}
	if st.LastUsed.IsZero() {
		t.Fatal("LastUsed not read")
	}
}

func TestStatusDrift(t *testing.T) {
	confdir := t.TempDir()
	iso := filepath.Join(confdir, "work")
	os.MkdirAll(iso, 0o755)
	os.WriteFile(filepath.Join(confdir, "work.conf"), []byte("GH_HOST=github.com\n"), 0o644)

	cases := []struct {
		name    string
		pin     string // .ghprofile contents; "" means no pointer file
		ambient string // GH_CONFIG_DIR; "" means unset
		want    bool
	}{
		{"ambient unset while pinned drifts", "work", "", true},
		{"ambient equals isolated is clean", "work", iso, false},
		{"ambient other dir drifts", "work", filepath.Join(confdir, "other"), true},
		{"cwd pins a different profile is clean", "elsewhere", "", false},
		{"cwd not pinned is clean", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pwd := t.TempDir()
			if c.pin != "" {
				os.WriteFile(filepath.Join(pwd, ".ghprofile"), []byte(c.pin+"\n"), 0o644)
			}
			t.Chdir(pwd)
			t.Setenv("GH_CONFIG_DIR", c.ambient)
			st, err := github.NewProvider().Status("work", confdir)
			if err != nil {
				t.Fatal(err)
			}
			if st.Drifted != c.want {
				t.Fatalf("Drifted = %v, want %v", st.Drifted, c.want)
			}
		})
	}
}

func TestStatusBlankOnMissingHosts(t *testing.T) {
	confdir := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "work.conf"), []byte("GH_HOST=github.com\n"), 0o644)
	st, err := github.NewProvider().Status("work", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Identity != "" {
		t.Fatalf("expected blank identity, got %q", st.Identity)
	}
}
