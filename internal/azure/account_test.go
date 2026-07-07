package azure

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// shimAz writes a fake `az` onto PATH that logs its args and echoes a fixed
// JSON for `account show`.
func shimAz(t *testing.T, logPath string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/usr/bin/env bash\necho \"$*\" >> \"" + logPath + "\"\n" +
		"case \"$*\" in *\"account show\"*) echo '{\"tenantId\":\"g\",\"name\":\"s\",\"user\":{\"name\":\"u@x\"}}';; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "az"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCleanSlate(t *testing.T) {
	log := filepath.Join(t.TempDir(), "az.log")
	shimAz(t, log)
	oldFS := procFS
	procFS = t.TempDir()
	t.Cleanup(func() { procFS = oldFS })
	cfg := t.TempDir()
	os.WriteFile(filepath.Join(cfg, "msal_token_cache.json"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(cfg, "service_principal_entries.json"), []byte("x"), 0o644)
	if err := CleanSlate(cfg, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "msal_token_cache.json")); !os.IsNotExist(err) {
		t.Fatal("token cache not removed")
	}
	b, _ := os.ReadFile(log)
	if !contains(string(b), "logout") || !contains(string(b), "account clear") {
		t.Fatalf("missing logout/clear: %s", b)
	}
}

func TestAccountShow(t *testing.T) {
	shimAz(t, filepath.Join(t.TempDir(), "az.log"))
	out, err := AccountShow()
	if err != nil || !contains(string(out), "tenantId") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
