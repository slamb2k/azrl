package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedBrowserMapEnv wires azrl.conf, one gcp profile conf, and an ssh shim
// that prints a POSIX-probe hit for an Edge Local State.
func seedBrowserMapEnv(t *testing.T, sshScript string) (confPath string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "azrl.conf"),
		[]byte("LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm\n"), 0o644)
	gp := filepath.Join(home, ".gcp-profiles")
	os.MkdirAll(gp, 0o755)
	confPath = filepath.Join(gp, "work.conf")
	os.WriteFile(confPath,
		[]byte("GCP_PROJECT=acme-prod\nGCP_EXPECT_ACCOUNT=simon@acme.com\n"), 0o644)

	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/usr/bin/env bash\n"+sshScript), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return confPath
}

const twoProfileProbe = `cat <<'EOF'
===AZRL /home/u/.config/microsoft-edge/Local State
{"profile":{"info_cache":{"Default":{"name":"Personal","user_name":"me@gmail.com"},"Profile 2":{"name":"Work","user_name":"simon@acme.com"}}}}
EOF
`

func TestBrowserMapPicksDiscoveredProfile(t *testing.T) {
	confPath := seedBrowserMapEnv(t, twoProfileProbe)
	RootCmd.SetIn(strings.NewReader("1\n"))
	out, err := execRoot(t, "gcp", "browser", "work")
	if err != nil {
		t.Fatalf("browser map: %v (out=%q)", err, out)
	}
	// GCP_EXPECT_ACCOUNT matches "Profile 2" (Work), so identity sorting puts
	// it first — option 1.
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), `GCP_BROWSER_CMD=microsoft-edge --profile-directory="Profile 2"`) {
		t.Fatalf("conf missing mapped command:\n%s", b)
	}
	if !strings.Contains(string(b), "GCP_BROWSER_LABEL=Edge — Work") {
		t.Fatalf("conf missing label:\n%s", b)
	}
	if !strings.Contains(string(b), "GCP_PROJECT=acme-prod") {
		t.Fatalf("existing keys must be preserved:\n%s", b)
	}
}

func TestBrowserMapManualFallbackWhenUnreachable(t *testing.T) {
	confPath := seedBrowserMapEnv(t, "exit 1\n")
	RootCmd.SetIn(strings.NewReader("m\nmy-browser --foo\n"))
	if _, err := execRoot(t, "gcp", "browser", "work"); err != nil {
		t.Fatalf("manual fallback: %v", err)
	}
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), "GCP_BROWSER_CMD=my-browser --foo") {
		t.Fatalf("manual command not written:\n%s", b)
	}
}

func TestBrowserMapClear(t *testing.T) {
	confPath := seedBrowserMapEnv(t, "exit 1\n")
	os.WriteFile(confPath,
		[]byte("GCP_PROJECT=acme-prod\nGCP_BROWSER_CMD=old\nGCP_BROWSER_LABEL=Old\n"), 0o644)
	RootCmd.SetIn(strings.NewReader("0\n"))
	if _, err := execRoot(t, "gcp", "browser", "work"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), "GCP_BROWSER_CMD=\n") || !strings.Contains(string(b), "GCP_BROWSER_LABEL=\n") {
		t.Fatalf("mapping not cleared:\n%s", b)
	}
}

func TestBrowserMapUnknownProfileErrors(t *testing.T) {
	seedBrowserMapEnv(t, "exit 1\n")
	if _, err := execRoot(t, "gcp", "browser", "nope"); err == nil {
		t.Fatal("unknown profile must error")
	}
}
