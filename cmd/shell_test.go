package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// seedShellHome creates a temp HOME with one conf per provider, exercising
// both plain and isolate variants. Returns the HOME path.
func seedShellHome(t *testing.T) string {
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
	write(".github-profiles/oss.conf", "GH_HOST=github.com\n")
	write(".aws-profiles/prod.conf", "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_SSO_REGION=us-east-1\n")
	write(".aws-profiles/sealed.conf", "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_ISOLATE=true\n")
	write(".gcp-profiles/lab.conf", "GCP_PROJECT=acme-lab\nGCP_CONFIG_NAME=labcfg\n")
	write(".gcp-profiles/vault.conf", "GCP_PROJECT=acme-vault\nGCP_ISOLATE=true\n")
	return home
}

func envHas(t *testing.T, env []string, want string) {
	t.Helper()
	for _, e := range env {
		if e == want {
			return
		}
	}
	t.Fatalf("env missing %q:\n%s", want, strings.Join(env, "\n"))
}

func envLacksKey(t *testing.T, env []string, key string) {
	t.Helper()
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			t.Fatalf("env must not carry %s: %q", key, e)
		}
	}
}

func TestShellEnvPerProvider(t *testing.T) {
	home := seedShellHome(t)

	env, err := shellEnv("azure", "work")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "AZURE_CONFIG_DIR="+filepath.Join(home, ".azure-profiles", "work"))
	envHas(t, env, "AZRL_PROFILE=azure:work")
	envHas(t, env, "AZRL_BROWSER_CMD=chrome --profile-directory=Work")

	env, err = shellEnv("github", "oss")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "GH_CONFIG_DIR="+filepath.Join(home, ".github-profiles", "oss"))
	envHas(t, env, "AZRL_PROFILE=github:oss")
	envLacksKey(t, env, "AZRL_BROWSER_CMD") // no mapping on this profile

	env, err = shellEnv("aws", "prod")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "AWS_PROFILE=prod")
	envHas(t, env, "AZRL_PROFILE=aws:prod")
	envLacksKey(t, env, "AWS_CONFIG_FILE")

	env, err = shellEnv("aws", "sealed")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "AWS_CONFIG_FILE="+filepath.Join(home, ".aws-profiles", "sealed", "config"))
	envHas(t, env, "AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(home, ".aws-profiles", "sealed", "credentials"))
	envLacksKey(t, env, "AWS_PROFILE")

	env, err = shellEnv("gcp", "lab")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "CLOUDSDK_ACTIVE_CONFIG_NAME=labcfg")
	envLacksKey(t, env, "CLOUDSDK_CONFIG")

	env, err = shellEnv("gcp", "vault")
	if err != nil {
		t.Fatal(err)
	}
	envHas(t, env, "CLOUDSDK_CONFIG="+filepath.Join(home, ".gcp-profiles", "vault"))
	envHas(t, env, "CLOUDSDK_ACTIVE_CONFIG_NAME=vault") // config name defaults to profile name
}

func TestShellEnvUnknownProfileErrors(t *testing.T) {
	seedShellHome(t)
	if _, err := shellEnv("azure", "nope"); err == nil {
		t.Fatal("unknown profile should error")
	}
	if _, err := shellEnv("aws", "nope"); err == nil {
		t.Fatal("unknown aws profile should error")
	}
}

// fakeShell installs a fake $SHELL that logs selected env vars, and returns
// the log path. exitCode is the fake shell's exit status.
func fakeShell(t *testing.T, exitCode int) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "shell.log")
	script := "#!/bin/sh\n" +
		"{ echo \"AZRL_PROFILE=$AZRL_PROFILE\"; echo \"AZURE_CONFIG_DIR=$AZURE_CONFIG_DIR\"; " +
		"echo \"AWS_PROFILE=$AWS_PROFILE\"; echo \"AZRL_BROWSER_CMD=$AZRL_BROWSER_CMD\"; " +
		"echo \"AWS_CONFIG_FILE=$AWS_CONFIG_FILE\"; } >> \"" + log + "\"\n" +
		fmt.Sprintf("exit %d\n", exitCode)
	sh := filepath.Join(bin, "fakeshell")
	if err := os.WriteFile(sh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", sh)
	return log
}

// liveAzureSession marks the azure profile's isolated dir as signed in so
// runShell skips the login-first path (azure Status reads azureProfile.json).
func liveAzureSession(t *testing.T, home, name string) {
	t.Helper()
	p := filepath.Join(home, ".azure-profiles", name, "azureProfile.json")
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p,
		[]byte(`{"subscriptions":[{"user":{"name":"u@acme.com"},"isDefault":true,"tenantId":"g1"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunShellExecsShellWithEnvAndBanner(t *testing.T) {
	home := seedShellHome(t)
	liveAzureSession(t, home, "work")
	log := fakeShell(t, 0)
	t.Setenv("AZRL_PROFILE", "")

	var out strings.Builder
	if err := runShell("azure", "work", &out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	got := string(b)
	for _, want := range []string{
		"AZRL_PROFILE=azure:work",
		"AZURE_CONFIG_DIR=" + filepath.Join(home, ".azure-profiles", "work"),
		"AZRL_BROWSER_CMD=chrome --profile-directory=Work",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("subshell env missing %q; got:\n%s", want, got)
		}
	}
	if !strings.Contains(out.String(), "azrl: shell as work (azure) — 'exit' returns") {
		t.Fatalf("banner missing: %q", out.String())
	}
}

func TestRunShellSignsInFirstWhenSessionDead(t *testing.T) {
	seedShellHome(t) // no azureProfile.json → session dead
	fakeShell(t, 0)
	t.Setenv("AZRL_PROFILE", "")

	var calls [][]string
	orig := shellLoginRunner
	shellLoginRunner = func(providerName, name string) error {
		calls = append(calls, []string{providerName, name})
		return nil
	}
	defer func() { shellLoginRunner = orig }()

	var out strings.Builder
	if err := runShell("azure", "work", &out); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0][0] != "azure" || calls[0][1] != "work" {
		t.Fatalf("expected one login call for azure:work, got %v", calls)
	}
}

func TestRunShellAbortsWhenLoginFails(t *testing.T) {
	seedShellHome(t)
	log := fakeShell(t, 0)
	t.Setenv("AZRL_PROFILE", "")

	orig := shellLoginRunner
	shellLoginRunner = func(string, string) error { return fmt.Errorf("boom") }
	defer func() { shellLoginRunner = orig }()

	var out strings.Builder
	if err := runShell("azure", "work", &out); err == nil {
		t.Fatal("login failure must abort the shell")
	}
	if b, _ := os.ReadFile(log); len(b) != 0 {
		t.Fatalf("no subshell may start after a failed login; log: %s", b)
	}
}

func TestRunShellWarnsOnNesting(t *testing.T) {
	home := seedShellHome(t)
	liveAzureSession(t, home, "work")
	fakeShell(t, 0)
	t.Setenv("AZRL_PROFILE", "gcp:lab")

	var out strings.Builder
	if err := runShell("azure", "work", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already inside an azrl shell (gcp:lab)") {
		t.Fatalf("nesting warning missing: %q", out.String())
	}
}

func TestRunShellPassesExitStatusThrough(t *testing.T) {
	home := seedShellHome(t)
	liveAzureSession(t, home, "work")
	fakeShell(t, 3)
	t.Setenv("AZRL_PROFILE", "")

	code := -1
	orig := shellExit
	shellExit = func(c int) { code = c }
	defer func() { shellExit = orig }()

	var out strings.Builder
	if err := runShell("azure", "work", &out); err != nil {
		t.Fatal(err)
	}
	if code != 3 {
		t.Fatalf("exit status not passed through: got %d, want 3", code)
	}
}

// TestRunShellScrubsStaleEnvFromOuterAzrlShell guards the "innermost wins"
// contract: a key an outer azrl shell set but the inner profile does not set
// must not survive into the inner subshell (append-only os.Environ dedup
// otherwise lets the outer value win).
func TestRunShellScrubsStaleEnvFromOuterAzrlShell(t *testing.T) {
	seedShellHome(t)
	log := fakeShell(t, 0)
	t.Setenv("AZRL_PROFILE", "")

	orig := shellLoginRunner
	shellLoginRunner = func(string, string) error { return nil }
	defer func() { shellLoginRunner = orig }()

	// (a) outer aws isolate profile leaves AWS_CONFIG_FILE behind; entering a
	// non-isolate aws profile must not inherit it.
	t.Setenv("AWS_CONFIG_FILE", "/outer/sealed/config")
	var out strings.Builder
	if err := runShell("aws", "prod", &out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	got := string(b)
	if !strings.Contains(got, "AWS_CONFIG_FILE=\n") {
		t.Fatalf("stale AWS_CONFIG_FILE leaked into aws:prod subshell:\n%s", got)
	}
	if !strings.Contains(got, "AWS_PROFILE=prod\n") {
		t.Fatalf("expected AWS_PROFILE=prod, got:\n%s", got)
	}

	// (b) outer profile with a browser mapping leaves AZRL_BROWSER_CMD
	// behind; entering a github profile with no mapping must not inherit it.
	os.Remove(log)
	t.Setenv("AWS_CONFIG_FILE", "")
	t.Setenv("AZRL_BROWSER_CMD", "outer-browser")
	var out2 strings.Builder
	if err := runShell("github", "oss", &out2); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(log)
	got = string(b)
	if !strings.Contains(got, "AZRL_BROWSER_CMD=\n") {
		t.Fatalf("stale AZRL_BROWSER_CMD leaked into github:oss subshell:\n%s", got)
	}
}

func TestShellVerbRegisteredOnAllSurfaces(t *testing.T) {
	find := func(cmds []*cobra.Command) bool {
		for _, c := range cmds {
			if strings.HasPrefix(c.Use, "shell") {
				return true
			}
		}
		return false
	}
	if !find(RootCmd.Commands()) {
		t.Fatal("azrl shell not registered")
	}
	if !find(githubSubcommands()) {
		t.Fatal("gh shell not registered")
	}
	if !find(awsSubcommands()) {
		t.Fatal("aws shell not registered")
	}
	if !find(gcpSubcommands()) {
		t.Fatal("gcp shell not registered")
	}
}
