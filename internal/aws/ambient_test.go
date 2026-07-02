package aws_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/provider"
)

func writeAwsConfig(t *testing.T, home, body string) string {
	t.Helper()
	dir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAmbientEnvProfileWinsOverDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AWS_CONFIG_FILE", "")
	t.Setenv("AWS_PROFILE", "acme-prod")
	writeAwsConfig(t, home,
		"[default]\nsso_account_id = 999999999999\nsso_role_name = Other\n\n"+
			"[profile acme-prod]\nsso_session = acme-prod\nsso_account_id = 123456789012\nsso_role_name = AdministratorAccess\n")

	a, err := aws.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "123456789012/AdministratorAccess" {
		t.Fatalf("Identity = %q", a.Identity)
	}
	if a.Source != "env:AWS_PROFILE" {
		t.Fatalf("Source = %q, want env:AWS_PROFILE", a.Source)
	}
}

func TestAmbientEnvProfileNameAloneWhenUnresolvable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AWS_CONFIG_FILE", "")
	t.Setenv("AWS_PROFILE", "mystery")

	a, err := aws.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "mystery" {
		t.Fatalf("Identity = %q, want the profile name alone", a.Identity)
	}
	if a.Source != "env:AWS_PROFILE" {
		t.Fatalf("Source = %q", a.Source)
	}
}

func TestAmbientFallsBackToDefaultProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AWS_CONFIG_FILE", "")
	t.Setenv("AWS_PROFILE", "")
	writeAwsConfig(t, home,
		"[default]\nsso_account_id = 123456789012\nsso_role_name = Dev\nregion = ap-southeast-2\n")

	a, err := aws.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "123456789012/Dev" {
		t.Fatalf("Identity = %q", a.Identity)
	}
	if a.Source != "file:~/.aws/config" {
		t.Fatalf("Source = %q", a.Source)
	}
}

func TestAmbientDefaultProfileNameAloneWithoutSSOKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AWS_CONFIG_FILE", "")
	t.Setenv("AWS_PROFILE", "")
	writeAwsConfig(t, home, "[default]\nregion = us-east-1\n")

	a, err := aws.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "default" {
		t.Fatalf("Identity = %q, want default", a.Identity)
	}
}

func TestAmbientHonorsAwsConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	os.WriteFile(path, []byte("[default]\nsso_account_id = 42\nsso_role_name = Ops\n"), 0o644)
	t.Setenv("AWS_CONFIG_FILE", path)
	t.Setenv("AWS_PROFILE", "")

	a, err := aws.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "42/Ops" {
		t.Fatalf("Identity = %q", a.Identity)
	}
	if a.Source != "file:$AWS_CONFIG_FILE" {
		t.Fatalf("Source = %q", a.Source)
	}
}

func TestAmbientZeroWithoutEnvOrDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AWS_CONFIG_FILE", "")
	t.Setenv("AWS_PROFILE", "")

	a, err := aws.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a != (provider.Ambient{}) {
		t.Fatalf("missing config: got %+v, want zero", a)
	}

	writeAwsConfig(t, home, "[profile named-only]\nsso_account_id = 1\n")
	a, err = aws.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a != (provider.Ambient{}) {
		t.Fatalf("no [default] stanza: got %+v, want zero", a)
	}
}
