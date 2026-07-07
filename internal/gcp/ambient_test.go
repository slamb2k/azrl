package gcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/slamb2k/azrl/internal/gcp"
	"github.com/slamb2k/azrl/internal/provider"
)

func writeGcloudConfig(t *testing.T, dir, name, account string) {
	t.Helper()
	confs := filepath.Join(dir, "configurations")
	if err := os.MkdirAll(confs, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[core]\naccount = " + account + "\nproject = proj-1\n"
	if err := os.WriteFile(filepath.Join(confs, "config_"+name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAmbientEnvConfigNameWinsOverActiveConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLOUDSDK_CONFIG", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "work")
	dir := filepath.Join(home, ".config", "gcloud")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "active_config"), []byte("personal\n"), 0o644)
	writeGcloudConfig(t, dir, "work", "simon@work.example")
	writeGcloudConfig(t, dir, "personal", "simon@gmail.com")

	a, err := gcp.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "simon@work.example" {
		t.Fatalf("Identity = %q", a.Identity)
	}
	if a.Source != "env:CLOUDSDK_ACTIVE_CONFIG_NAME" {
		t.Fatalf("Source = %q", a.Source)
	}
}

func TestAmbientReadsActiveConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLOUDSDK_CONFIG", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	dir := filepath.Join(home, ".config", "gcloud")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "active_config"), []byte("personal\n"), 0o644)
	writeGcloudConfig(t, dir, "personal", "simon@gmail.com")

	a, err := gcp.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "simon@gmail.com" {
		t.Fatalf("Identity = %q", a.Identity)
	}
	if a.Source != "file:~/.config/gcloud/active_config" {
		t.Fatalf("Source = %q", a.Source)
	}
}

func TestAmbientHonorsCloudsdkConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", dir)
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	os.WriteFile(filepath.Join(dir, "active_config"), []byte("iso\n"), 0o644)
	writeGcloudConfig(t, dir, "iso", "svc@proj.iam.example")

	a, err := gcp.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a.Identity != "svc@proj.iam.example" {
		t.Fatalf("Identity = %q", a.Identity)
	}
	if a.Source != "file:$CLOUDSDK_CONFIG/active_config" {
		t.Fatalf("Source = %q", a.Source)
	}
}

func TestAmbientZeroOnMissingState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLOUDSDK_CONFIG", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")

	a, err := gcp.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a != (provider.Ambient{}) {
		t.Fatalf("missing gcloud dir: got %+v, want zero", a)
	}

	// active_config names a configuration whose file is absent.
	dir := filepath.Join(home, ".config", "gcloud")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "active_config"), []byte("ghost\n"), 0o644)
	a, err = gcp.NewProvider().Ambient()
	if err != nil {
		t.Fatal(err)
	}
	if a != (provider.Ambient{}) {
		t.Fatalf("missing configuration file: got %+v, want zero", a)
	}
}

func TestCaptureDefaultsReadsActiveConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLOUDSDK_CONFIG", dir)
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	os.WriteFile(filepath.Join(dir, "active_config"), []byte("work\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "configurations"), 0o755)
	os.WriteFile(filepath.Join(dir, "configurations", "config_work"), []byte(`[core]
account = dev@example.com
project = acme-prod

[compute]
region = australiaeast
`), 0o644)

	c := gcp.CaptureDefaults()
	if c.ConfigName != "work" || c.Project != "acme-prod" || c.Region != "australiaeast" {
		t.Fatalf("ambient config not derived: %+v", c)
	}
}

func TestCaptureDefaultsZeroOnMissingState(t *testing.T) {
	t.Setenv("CLOUDSDK_CONFIG", t.TempDir())
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	if c := gcp.CaptureDefaults(); c != (gcp.Conf{}) {
		t.Fatalf("expected zero Conf, got %+v", c)
	}
}
