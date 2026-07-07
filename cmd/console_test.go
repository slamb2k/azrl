package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedConsoleHome mirrors seedShellHome's provider confs for URL building.
func seedConsoleHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	write := func(rel, body string) {
		p := filepath.Join(home, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".azure-profiles/work.conf", "AZ_TENANT=acme.com\nAZ_BROWSER_CMD=chrome --profile-directory=Work\n")
	write(".azure-profiles/guest.conf", "AZ_TENANT=\nAZ_TENANT_ID=11111111-2222-3333-4444-555555555555\n")
	write(".github-profiles/oss.conf", "GH_HOST=github.com\n")
	write(".aws-profiles/prod.conf", "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	write(".aws-profiles/hollow.conf", "AWS_SSO_REGION=us-east-1\n")
	write(".gcp-profiles/lab.conf", "GCP_PROJECT=acme-lab\nGCP_CONFIG_NAME=labcfg\n")
	return home
}

func TestConsoleURLPerProvider(t *testing.T) {
	home := seedConsoleHome(t)

	u, browser, err := consoleURL("azure", "work")
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://portal.azure.com/#@acme.com" {
		t.Fatalf("azure url = %q", u)
	}
	if browser != "chrome --profile-directory=Work" {
		t.Fatalf("azure browser mapping = %q", browser)
	}

	u, _, err = consoleURL("azure", "guest")
	if err != nil || u != "https://portal.azure.com/#@11111111-2222-3333-4444-555555555555" {
		t.Fatalf("azure GUID fallback = %q, %v", u, err)
	}

	u, _, err = consoleURL("github", "oss")
	if err != nil || u != "https://github.com" {
		t.Fatalf("github url = %q, %v", u, err)
	}

	u, _, err = consoleURL("aws", "prod")
	if err != nil || u != "https://acme.awsapps.com/start" {
		t.Fatalf("aws url = %q, %v", u, err)
	}

	// gcp: no signed-in account on disk ⇒ authuser omitted.
	u, _, err = consoleURL("gcp", "lab")
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://console.cloud.google.com/?project=acme-lab" {
		t.Fatalf("gcp url without identity = %q", u)
	}

	// gcp with a live account in the named configuration ⇒ authuser added.
	gcDir := filepath.Join(home, ".config", "gcloud")
	os.MkdirAll(filepath.Join(gcDir, "configurations"), 0o755)
	os.WriteFile(filepath.Join(gcDir, "configurations", "config_labcfg"),
		[]byte("[core]\naccount = dev@example.com\nproject = acme-lab\n"), 0o644)
	u, _, err = consoleURL("gcp", "lab")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "project=acme-lab") || !strings.Contains(u, "authuser=dev%40example.com") {
		t.Fatalf("gcp url with identity = %q", u)
	}
}

func TestConsoleURLErrorsOnMissingData(t *testing.T) {
	seedConsoleHome(t)
	if _, _, err := consoleURL("aws", "hollow"); err == nil {
		t.Fatal("aws profile without a start URL must error")
	}
	if _, _, err := consoleURL("azure", "nope"); err == nil {
		t.Fatal("unknown profile must error")
	}
	if _, _, err := consoleURL("mars", "x"); err == nil {
		t.Fatal("unknown provider must error")
	}
}
