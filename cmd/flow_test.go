package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// shimAzCapture provides an `az` whose `account show`/domains return fixed JSON.
func shimAzCapture(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/usr/bin/env bash\ncase \"$*\" in\n" +
		"  *\"account show\"*) echo '{\"tenantId\":\"guid-1\",\"id\":\"sub-1\",\"name\":\"Sub\",\"user\":{\"name\":\"u@acme.onmicrosoft.com\"}}';;\n" +
		"  *\"rest\"*\"domains\"*) echo '{\"value\":[{\"id\":\"acme.onmicrosoft.com\",\"isDefault\":true}]}';;\n" +
		"  *) echo '{}';;\nesac\n"
	os.WriteFile(filepath.Join(bin, "az"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCaptureCmd(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	shimAzCapture(t)
	chdir(t, work)
	RootCmd.SetArgs([]string{"capture", "acme"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".azure-profiles", "acme.conf"))
	if !contains(string(b), "AZ_TENANT=acme.onmicrosoft.com") || !contains(string(b), "AZ_TENANT_ID=guid-1") {
		t.Fatalf("conf=%s", b)
	}
	az, _ := os.ReadFile(filepath.Join(work, ".azprofile"))
	if string(az) != "acme\n" {
		t.Fatalf("azprofile=%q", string(az))
	}
}

func TestCaptureRefusesClobber(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=keep.me\n"), 0o644)
	shimAzCapture(t)
	chdir(t, t.TempDir())
	RootCmd.SetArgs([]string{"capture", "acme"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatal("expected clobber refusal")
	}
	b, _ := os.ReadFile(filepath.Join(home, ".azure-profiles", "acme.conf"))
	if !contains(string(b), "keep.me") {
		t.Fatalf("conf overwritten: %s", b)
	}
}

// contains/indexOf are defined in cmd test files via the azure package style;
// redefine here for the cmd package tests.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return len(sub) == 0
}
