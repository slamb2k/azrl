package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCLI installs an executable named bin on PATH that appends its argv to
// logfile and exits with the code produced by script (default 0).
func fakeCLI(t *testing.T, dir, bin, logfile, extra string) {
	t.Helper()
	body := "#!/bin/sh\necho \"$@\" >> " + logfile + "\n" + extra
	if err := os.WriteFile(filepath.Join(dir, bin), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func defaultTestHome(t *testing.T) (home, bin, log string) {
	t.Helper()
	home = t.TempDir()
	bin = t.TempDir()
	log = filepath.Join(t.TempDir(), "argv.log")
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return
}

func TestDefaultAzureTargetsProfileTenant(t *testing.T) {
	home, bin, log := defaultTestHome(t)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "work.conf"),
		[]byte("AZ_TENANT=acme.com\nAZ_TENANT_ID=1111-2222\n"), 0o644)
	fakeCLI(t, bin, "az", log, "")
	var out bytes.Buffer
	if err := runDefault("azure", "work", strings.NewReader(""), &out); err != nil {
		t.Fatalf("runDefault: %v\n%s", err, out.String())
	}
	b, _ := os.ReadFile(log)
	if got := strings.TrimSpace(string(b)); got != "login --tenant 1111-2222" {
		t.Fatalf("az argv = %q (tenant GUID should win over the domain)", got)
	}
}

func TestDefaultGcpPassesExpectedAccount(t *testing.T) {
	home, bin, log := defaultTestHome(t)
	os.MkdirAll(filepath.Join(home, ".gcp-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".gcp-profiles", "home.conf"),
		[]byte("GCP_PROJECT=demo\nGCP_EXPECT_ACCOUNT=me@gmail.com\n"), 0o644)
	fakeCLI(t, bin, "gcloud", log, "")
	var out bytes.Buffer
	if err := runDefault("gcp", "home", strings.NewReader(""), &out); err != nil {
		t.Fatalf("runDefault: %v\n%s", err, out.String())
	}
	b, _ := os.ReadFile(log)
	if got := strings.TrimSpace(string(b)); got != "auth login me@gmail.com" {
		t.Fatalf("gcloud argv = %q", got)
	}
}

func TestDefaultGithubSwitchThenLoginFallback(t *testing.T) {
	home, bin, log := defaultTestHome(t)
	os.MkdirAll(filepath.Join(home, ".github-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".github-profiles", "oss.conf"),
		[]byte("GH_HOST=github.com\nGH_USER=octocat\n"), 0o644)
	// switch succeeds: no login attempted.
	fakeCLI(t, bin, "gh", log, "")
	var out bytes.Buffer
	if err := runDefault("github", "oss", strings.NewReader(""), &out); err != nil {
		t.Fatalf("runDefault: %v", err)
	}
	b, _ := os.ReadFile(log)
	if got := strings.TrimSpace(string(b)); got != "auth switch --hostname github.com --user octocat" {
		t.Fatalf("gh argv = %q", got)
	}

	// switch fails (account unknown to native gh): bridged web login follows.
	os.Remove(log)
	fakeCLI(t, bin, "gh", log, `case "$2" in switch) exit 1;; esac`+"\n")
	out.Reset()
	if err := runDefault("github", "oss", strings.NewReader(""), &out); err != nil {
		t.Fatalf("runDefault fallback: %v", err)
	}
	b, _ = os.ReadFile(log)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || lines[1] != "auth login --hostname github.com --web" {
		t.Fatalf("expected switch then web login, got %q", lines)
	}
	if !strings.Contains(out.String(), "starting a web login") {
		t.Fatalf("fallback should announce itself: %q", out.String())
	}
}

func TestDefaultAwsWritesDefaultStanzaThenSignsIn(t *testing.T) {
	home, bin, log := defaultTestHome(t)
	cfg := filepath.Join(t.TempDir(), "config")
	t.Setenv("AWS_CONFIG_FILE", cfg)
	os.MkdirAll(filepath.Join(home, ".aws-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".aws-profiles", "prod.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_SSO_REGION=ap-southeast-2\nAWS_ACCOUNT_ID=111122223333\nAWS_ROLE_NAME=Dev\n"), 0o644)
	fakeCLI(t, bin, "aws", log, "")
	var out bytes.Buffer
	if err := runDefault("aws", "prod", strings.NewReader(""), &out); err != nil {
		t.Fatalf("runDefault: %v\n%s", err, out.String())
	}
	b, _ := os.ReadFile(cfg)
	if !strings.Contains(string(b), "[default]") || !strings.Contains(string(b), "sso_role_name = Dev") {
		t.Fatalf("[default] stanza not written:\n%s", b)
	}
	lg, _ := os.ReadFile(log)
	if got := strings.TrimSpace(string(lg)); got != "sso login" {
		t.Fatalf("aws argv = %q", got)
	}
}

func TestDefaultPicker(t *testing.T) {
	home, bin, log := defaultTestHome(t)
	_ = bin
	_ = log
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "alpha.conf"), []byte("AZ_TENANT=a.com\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "beta.conf"), []byte("AZ_TENANT=b.com\n"), 0o644)
	var out bytes.Buffer
	name, err := pickDefaultProfile("azure", "", strings.NewReader("2\n"), &out)
	if err != nil || name != "beta" {
		t.Fatalf("pick 2 = %q, %v", name, err)
	}
	if !strings.Contains(out.String(), "1) alpha") || !strings.Contains(out.String(), "2) beta") {
		t.Fatalf("picker should list numbered profiles:\n%s", out.String())
	}
	if _, err := pickDefaultProfile("azure", "", strings.NewReader("9\n"), &out); err == nil {
		t.Fatal("out-of-range pick should error")
	}
	// A lone profile is picked automatically.
	os.Remove(filepath.Join(home, ".azure-profiles", "beta.conf"))
	out.Reset()
	name, err = pickDefaultProfile("azure", "", strings.NewReader(""), &out)
	if err != nil || name != "alpha" {
		t.Fatalf("lone profile = %q, %v", name, err)
	}
}
