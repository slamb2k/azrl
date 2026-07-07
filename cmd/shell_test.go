package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
