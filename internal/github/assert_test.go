package github

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeGhAPI installs a gh shim that prints the given login as `gh api user`
// JSON and records the GH_CONFIG_DIR it saw.
func fakeGhAPI(t *testing.T, login string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "gh.log")
	script := "#!/usr/bin/env bash\n" +
		"echo \"GH_CONFIG_DIR=$GH_CONFIG_DIR ARGS=$*\" >> \"" + log + "\"\n" +
		"printf '{\"login\":\"" + login + "\"}'\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestAssertAccountMatches(t *testing.T) {
	log := fakeGhAPI(t, "octocat")
	profilesDir := t.TempDir()
	c := Conf{Host: "github.com", User: "octocat"}
	if err := AssertAccount(profilesDir, "work", c); err != nil {
		t.Fatalf("expected match: %v", err)
	}
	b, _ := os.ReadFile(log)
	if want := "GH_CONFIG_DIR=" + filepath.Join(profilesDir, "work"); !contains(string(b), want) {
		t.Fatalf("api not scoped to config dir; got %s", b)
	}
}

func TestAssertAccountMismatch(t *testing.T) {
	fakeGhAPI(t, "someone-else")
	c := Conf{Host: "github.com", User: "octocat"}
	if err := AssertAccount(t.TempDir(), "work", c); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestAssertAccountNoExpectedUserSkipsCheck(t *testing.T) {
	fakeGhAPI(t, "whoever")
	c := Conf{Host: "github.com"} // no GH_USER
	if err := AssertAccount(t.TempDir(), "work", c); err != nil {
		t.Fatalf("no expected user should pass: %v", err)
	}
}

func TestWhoAmIReturnsLogin(t *testing.T) {
	fakeGhAPI(t, "octocat")
	login, err := WhoAmI(t.TempDir(), "work", "github.com")
	if err != nil || login != "octocat" {
		t.Fatalf("WhoAmI=%q err=%v", login, err)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
