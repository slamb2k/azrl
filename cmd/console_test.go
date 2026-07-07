package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/spf13/cobra"
)

// seedConsoleHome mirrors seedShellHome's provider confs for URL building.
func seedConsoleHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLOUDSDK_CONFIG", "")
	t.Setenv("AZRL_BROWSER_CMD", "")
	write := func(rel, body string) {
		p := filepath.Join(home, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".azure-profiles/work.conf", "AZ_TENANT=acme.com\nAZ_BROWSER_CMD=chrome --profile-directory=Work\n")
	write(".azure-profiles/guest.conf", "AZ_TENANT=11111111-2222-3333-4444-555555555555\n")
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

// writeGlobalConf writes ~/.azure-profiles/azrl.conf under the HOME seeded by
// seedConsoleHome, for tests that need config.LoadGlobal to succeed.
func writeGlobalConf(t *testing.T, body string) {
	t.Helper()
	home := os.Getenv("HOME")
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	if err := os.WriteFile(filepath.Join(home, ".azure-profiles", "azrl.conf"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunConsoleOpensViaBridge(t *testing.T) {
	seedConsoleHome(t)
	writeGlobalConf(t, "BROWSER_CMD=wslview\n")

	var opened []string
	orig := consoleOpen
	consoleOpen = func(g config.Global, u string) error {
		opened = append(opened, g.BrowserCmd+" | "+u)
		return nil
	}
	defer func() { consoleOpen = orig }()

	var out strings.Builder
	if err := runConsole("azure", "work", &out); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != "chrome --profile-directory=Work | https://portal.azure.com/#@acme.com" {
		t.Fatalf("profile browser mapping must override the global: %v", opened)
	}
	if !strings.Contains(out.String(), "opening azure console") {
		t.Fatalf("success line missing: %q", out.String())
	}
}

func TestRunConsoleFallsBackToPrintingURL(t *testing.T) {
	seedConsoleHome(t)

	// (a) no global config at all — print the URL, no error.
	var out strings.Builder
	if err := runConsole("github", "oss", &out); err != nil {
		t.Fatalf("missing global config must not error: %v", err)
	}
	if !strings.Contains(out.String(), "https://github.com") {
		t.Fatalf("fallback must print the URL: %q", out.String())
	}

	// (b) launch failure — print the URL, no error.
	writeGlobalConf(t, "BROWSER_CMD=wslview\n")
	orig := consoleOpen
	consoleOpen = func(config.Global, string) error { return fmt.Errorf("ssh unreachable") }
	defer func() { consoleOpen = orig }()
	out.Reset()
	if err := runConsole("github", "oss", &out); err != nil {
		t.Fatalf("launch failure must not error: %v", err)
	}
	if !strings.Contains(out.String(), "https://github.com") {
		t.Fatalf("fallback must print the URL: %q", out.String())
	}
}

func TestRunConsoleProfileDataErrorsSurface(t *testing.T) {
	seedConsoleHome(t)
	var out strings.Builder
	if err := runConsole("aws", "hollow", &out); err == nil {
		t.Fatal("missing start URL is a profile-data error and must surface")
	}
}

func TestConsoleVerbRegisteredOnAllSurfaces(t *testing.T) {
	find := func(cmds []*cobra.Command) bool {
		for _, c := range cmds {
			if strings.HasPrefix(c.Use, "console") {
				return true
			}
		}
		return false
	}
	if !find(RootCmd.Commands()) || !find(githubSubcommands()) || !find(awsSubcommands()) || !find(gcpSubcommands()) {
		t.Fatal("console verb missing from a surface")
	}
}
