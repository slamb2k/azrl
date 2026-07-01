package azure_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/slamb2k/azrl/internal/azure"
)

func TestStatusReadsIdentityExpiryAndLastUsed(t *testing.T) {
	confdir := t.TempDir()
	iso := filepath.Join(confdir, "acme")
	os.MkdirAll(iso, 0o755)
	os.WriteFile(filepath.Join(confdir, "acme.conf"),
		[]byte("AZ_TENANT=acme.com\nLAST_USED=2026-06-01T10:00:00Z\nLAST_DIR=/work/acme\n"), 0o644)
	os.WriteFile(filepath.Join(iso, "azureProfile.json"),
		[]byte(`{"subscriptions":[{"user":{"name":"u@acme.com"},"isDefault":true,"tenantId":"guid-1"}]}`), 0o644)
	exp := time.Now().Add(42 * time.Minute).Unix()
	os.WriteFile(filepath.Join(iso, "msal_token_cache.json"),
		[]byte(`{"AccessToken":{"k":{"expires_on":"`+strconv.FormatInt(exp, 10)+`"}}}`), 0o644)

	st, err := azure.NewProvider().Status("acme", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Identity != "u@acme.com" {
		t.Fatalf("Identity = %q", st.Identity)
	}
	if st.Directory != "/work/acme" {
		t.Fatalf("Directory = %q", st.Directory)
	}
	if st.LastUsed.IsZero() {
		t.Fatal("LastUsed not read")
	}
	if st.Expiry == nil || st.Expiry.Unix() != exp {
		t.Fatalf("Expiry = %v", st.Expiry)
	}
}

func TestStatusReadsIdentityWithUTF8BOM(t *testing.T) {
	confdir := t.TempDir()
	iso := filepath.Join(confdir, "acme")
	os.MkdirAll(iso, 0o755)
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	// Azure CLI writes azureProfile.json with a leading UTF-8 BOM (EF BB BF).
	os.WriteFile(filepath.Join(iso, "azureProfile.json"),
		[]byte("\xef\xbb\xbf"+`{"subscriptions":[{"user":{"name":"u@acme.com"},"isDefault":true,"tenantId":"guid-1"}]}`), 0o644)

	st, err := azure.NewProvider().Status("acme", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Identity != "u@acme.com" {
		t.Fatalf("Identity = %q, want u@acme.com (BOM not stripped?)", st.Identity)
	}
}

func TestStatusDrift(t *testing.T) {
	confdir := t.TempDir()
	iso := filepath.Join(confdir, "acme")
	os.MkdirAll(iso, 0o755)
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)

	cases := []struct {
		name    string
		pin     string // .azprofile contents; "" means no pointer file
		ambient string // AZURE_CONFIG_DIR; "" means unset
		want    bool
	}{
		{"ambient unset while pinned drifts", "acme", "", true},
		{"ambient equals isolated is clean", "acme", iso, false},
		{"ambient other dir drifts", "acme", filepath.Join(confdir, "other"), true},
		{"cwd pins a different profile is clean", "elsewhere", "", false},
		{"cwd not pinned is clean", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pwd := t.TempDir()
			if c.pin != "" {
				os.WriteFile(filepath.Join(pwd, ".azprofile"), []byte(c.pin+"\n"), 0o644)
			}
			t.Chdir(pwd)
			t.Setenv("AZURE_CONFIG_DIR", c.ambient)
			st, err := azure.NewProvider().Status("acme", confdir)
			if err != nil {
				t.Fatal(err)
			}
			if st.Drifted != c.want {
				t.Fatalf("Drifted = %v, want %v", st.Drifted, c.want)
			}
		})
	}
}

func TestStatusLastUsedReflectsCacheMtime(t *testing.T) {
	confdir := t.TempDir()
	iso := filepath.Join(confdir, "acme")
	os.MkdirAll(iso, 0o755)
	// LAST_USED in the conf is older than the token cache's mtime: external `az`
	// usage refreshed the MSAL cache without going through azrl.
	os.WriteFile(filepath.Join(confdir, "acme.conf"),
		[]byte("AZ_TENANT=acme.com\nLAST_USED=2026-06-01T10:00:00Z\n"), 0o644)
	cache := filepath.Join(iso, "msal_token_cache.json")
	os.WriteFile(cache, []byte(`{"AccessToken":{}}`), 0o644)
	newer := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(cache, newer, newer); err != nil {
		t.Fatal(err)
	}

	st, err := azure.NewProvider().Status("acme", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.LastUsed.Equal(newer) {
		t.Fatalf("LastUsed = %v, want cache mtime %v", st.LastUsed, newer)
	}
}

func TestStatusBlankOnMissingCache(t *testing.T) {
	confdir := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	st, err := azure.NewProvider().Status("acme", confdir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Identity != "" || st.Expiry != nil {
		t.Fatalf("expected blank status, got %+v", st)
	}
}
