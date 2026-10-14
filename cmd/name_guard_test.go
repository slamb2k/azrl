package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGroupRmDotDotRefuses is the regression for `azrl gh|aws|gcp rm ..`
// deleting $HOME without a prompt.
func TestGroupRmDotDotRefuses(t *testing.T) {
	for _, group := range []string{"gh", "aws", "gcp"} {
		for _, name := range []string{"..", "."} {
			home := seedGhHome(t)
			if err := runRootErr(t, group, "rm", name); err == nil {
				t.Fatalf("%s rm %s must refuse", group, name)
			}
			if _, err := os.Stat(filepath.Join(home, ".github-profiles", "work.conf")); err != nil {
				t.Fatalf("%s rm %s deleted data outside its profile: %v", group, name, err)
			}
		}
	}
}

func TestAzureRmDotDotRefuses(t *testing.T) {
	resetRmFlags(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	if err := runRootErr(t, "rm", "..", "-y"); err == nil {
		t.Fatal("rm .. must refuse")
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); err != nil {
		t.Fatal("home must survive rm ..")
	}
}

func TestCaptureRejectsTraversalName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	err := runRootErr(t, "capture", "../../x")
	if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("capture ../../x must be rejected, got %v", err)
	}
}

// TestConsoleBareRejectsHostilePointer proves a repo-shipped .azprofile that
// traverses out of the profiles dir is refused, not loaded.
func TestConsoleBareRejectsHostilePointer(t *testing.T) {
	home := seedConsoleHome(t)
	t.Setenv("AZURE_CONFIG_DIR", "")
	t.Setenv("AZRL_PROFILE", "")
	repo := filepath.Join(home, "repo")
	os.MkdirAll(repo, 0o755)
	os.WriteFile(filepath.Join(repo, ".azprofile"), []byte("../repo/evil\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "evil.conf"), []byte("AZ_TENANT=evil.com\nAZ_BROWSER_CMD=touch pwned\n"), 0o644)
	t.Chdir(repo)
	err := runRootErr(t, "console")
	if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("hostile pointer must be refused, got %v", err)
	}
	if err := runRootErr(t, "console", "../repo/evil"); err == nil {
		t.Fatal("explicit traversal name must be refused")
	}
}
