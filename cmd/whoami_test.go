package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedWhoamiHome is seedStatusHome plus a clean AZRL_BROWSER_CMD.
func seedWhoamiHome(t *testing.T) string {
	t.Helper()
	seedStatusHome(t)
	t.Setenv("AZRL_BROWSER_CMD", "")
	return os.Getenv("HOME")
}

func mapAcme(t *testing.T, home, dir string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, ".azprofile"), []byte("acme\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "mappings"),
		[]byte(dir+"\tacme\tpointer\n"), 0o644)
}

func TestWhoamiPointerGovernsCwd(t *testing.T) {
	home := seedWhoamiHome(t)
	work := filepath.Join(home, "work")
	mapAcme(t, home, work)
	t.Chdir(work)

	out := runRoot(t, "whoami")
	for _, want := range []string{"azure", "acme", "u@acme.com", "via .azprofile", "Edge — Work (profile)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("whoami missing %q:\n%s", want, out)
		}
	}
}

func TestWhoamiAncestorGovernsSubdir(t *testing.T) {
	home := seedWhoamiHome(t)
	work := filepath.Join(home, "work")
	mapAcme(t, home, work)
	sub := filepath.Join(work, "sub")
	os.MkdirAll(sub, 0o755)
	t.Chdir(sub)

	out := runRoot(t, "whoami")
	if !strings.Contains(out, "via ancestor "+work) || !strings.Contains(out, "acme") {
		t.Fatalf("ancestor row missing:\n%s", out)
	}
}

func TestWhoamiShellOverrideWins(t *testing.T) {
	home := seedWhoamiHome(t)
	work := filepath.Join(home, "work")
	mapAcme(t, home, work)
	t.Chdir(work)
	t.Setenv("AZRL_PROFILE", "azure:other")

	out := runRoot(t, "whoami")
	if !strings.Contains(out, "shell override") || !strings.Contains(out, "other") {
		t.Fatalf("shell override should outrank the pointer:\n%s", out)
	}
	if strings.Contains(out, "via .azprofile") {
		t.Fatalf("pointer row should be superseded for azure:\n%s", out)
	}
}

func TestWhoamiAmbientFallback(t *testing.T) {
	home := seedWhoamiHome(t)
	os.MkdirAll(filepath.Join(home, ".azure"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure", "azureProfile.json"),
		[]byte(`{"subscriptions":[{"user":{"name":"u@acme.com"},"isDefault":true,"tenantId":"g1"}]}`), 0o644)
	t.Chdir(home)

	out := runRoot(t, "whoami")
	if !strings.Contains(out, "ambient") || !strings.Contains(out, "u@acme.com") {
		t.Fatalf("ambient fallback missing:\n%s", out)
	}
}

func TestWhoamiBrowserEnvThenGlobal(t *testing.T) {
	home := seedWhoamiHome(t)
	t.Chdir(home)

	// github's work profile has no browser assignment: env override wins.
	t.Setenv("AZRL_BROWSER_CMD", "felix")
	out := runRoot(t, "whoami")
	if !strings.Contains(out, "felix (env)") {
		t.Fatalf("env browser missing:\n%s", out)
	}

	// No env: the global BROWSER_CMD is what a bridged sign-in would run.
	t.Setenv("AZRL_BROWSER_CMD", "")
	os.WriteFile(filepath.Join(home, ".azure-profiles", "azrl.conf"),
		[]byte("BROWSER_CMD=wslview\n"), 0o644)
	out = runRoot(t, "whoami")
	if !strings.Contains(out, "wslview (global)") {
		t.Fatalf("global browser missing:\n%s", out)
	}
}

func TestWhoamiJSON(t *testing.T) {
	home := seedWhoamiHome(t)
	work := filepath.Join(home, "work")
	mapAcme(t, home, work)
	t.Chdir(work)

	out := runRoot(t, "whoami", "--json")
	whoamiJSON = false
	var rep whoamiReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if rep.Dir != work {
		t.Fatalf("dir = %q, want %q", rep.Dir, work)
	}
	var az *whoamiRow
	for i, r := range rep.Providers {
		if r.Provider == "azure" {
			az = &rep.Providers[i]
		}
	}
	if az == nil {
		t.Fatalf("no azure row: %+v", rep.Providers)
	}
	if az.Profile != "acme" || az.Via != "pointer" || az.Dir != work || az.Pointer != ".azprofile" ||
		az.Browser != "chrome-work" || az.BrowserLabel != "Edge — Work" || az.BrowserSource != "profile" {
		t.Fatalf("azure row = %+v", az)
	}
}

func TestStatusStubPointsAtReplacements(t *testing.T) {
	seedWhoamiHome(t)
	RootCmd.SetArgs([]string{"status", "--json"})
	err := RootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "whoami") || !strings.Contains(err.Error(), "profiles") {
		t.Fatalf("status stub should point at whoami/profiles, got %v", err)
	}
}
