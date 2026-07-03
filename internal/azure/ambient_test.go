package azure_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/provider"
)

func TestAmbientReadsDefaultSubscriptionUserWithBOM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AZURE_CONFIG_DIR", "")
	az := filepath.Join(home, ".azure")
	os.MkdirAll(az, 0o755)
	// Azure CLI writes azureProfile.json with a leading UTF-8 BOM (EF BB BF).
	os.WriteFile(filepath.Join(az, "azureProfile.json"),
		[]byte("\xef\xbb\xbf"+`{"subscriptions":[{"user":{"name":"simon@contoso.com"},"isDefault":true,"tenantId":"guid-1"}]}`), 0o644)

	a, err := azure.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "simon@contoso.com · guid-1" {
		t.Fatalf("Identity = %q", a.Identity)
	}
	if a.Source != "file:~/.azure/azureProfile.json" {
		t.Fatalf("Source = %q", a.Source)
	}
}

func TestAmbientHonorsAzureConfigDirAndTenantFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AZURE_CONFIG_DIR", dir)
	os.WriteFile(filepath.Join(dir, "azureProfile.json"),
		[]byte(`{"subscriptions":[{"isDefault":true,"tenantId":"guid-2"}]}`), 0o644)

	a, err := azure.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "guid-2" {
		t.Fatalf("Identity = %q, want tenantId fallback guid-2", a.Identity)
	}
	if a.Source != "file:$AZURE_CONFIG_DIR/azureProfile.json" {
		t.Fatalf("Source = %q", a.Source)
	}
}

func TestAmbientZeroOnMissingOrUnparseableProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AZURE_CONFIG_DIR", "")

	a, err := azure.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a != (provider.Ambient{}) {
		t.Fatalf("missing profile: got %+v, want zero", a)
	}

	az := filepath.Join(home, ".azure")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "azureProfile.json"), []byte("not json"), 0o644)
	a, err = azure.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a != (provider.Ambient{}) {
		t.Fatalf("unparseable profile: got %+v, want zero", a)
	}
}
