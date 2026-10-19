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
	// capture is metadata-only on every provider: it never claims the cwd.
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); !os.IsNotExist(err) {
		t.Fatalf("capture must not write .azprofile (err=%v)", err)
	}
}

// TestCaptureUpdatesInPlace proves re-capturing an existing profile refreshes
// the session-derived fields while keeping the user-set label and browser.
func TestCaptureUpdatesInPlace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"),
		[]byte("AZ_TENANT=old.tenant\nAZ_LABEL=Acme Prod\nAZ_BROWSER_CMD=edge\nAZ_BROWSER_LABEL=Edge\n"), 0o644)
	shimAzCapture(t)
	chdir(t, t.TempDir())
	out, err := execRoot(t, "capture", "acme")
	if err != nil {
		t.Fatalf("re-capture should update in place: %v (out=%q)", err, out)
	}
	b, _ := os.ReadFile(filepath.Join(home, ".azure-profiles", "acme.conf"))
	for _, want := range []string{"AZ_TENANT=acme.onmicrosoft.com", "AZ_LABEL=Acme Prod", "AZ_BROWSER_CMD=edge", "AZ_BROWSER_LABEL=Edge"} {
		if !contains(string(b), want) {
			t.Fatalf("missing %q after update:\n%s", want, b)
		}
	}
	if !contains(out, "updated") {
		t.Fatalf("expected an update notice:\n%s", out)
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
