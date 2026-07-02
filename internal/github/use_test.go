package github

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slamb2k/azrl/internal/profile"
)

// fakeGhGit installs gh and git shims that log their args + GH_CONFIG_DIR.
func fakeGhGit(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "cmds.log")
	for _, name := range []string{"gh", "git"} {
		script := "#!/usr/bin/env bash\n" +
			"echo \"" + name + " GH_CONFIG_DIR=$GH_CONFIG_DIR ARGS=$*\" >> \"" + log + "\"\n" +
			"exit 0\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestSetupRepoWiresCredentialHelperAndUsername(t *testing.T) {
	log := fakeGhGit(t)
	profilesDir := t.TempDir()
	pwd := t.TempDir()
	c := Conf{Host: "github.com", User: "octocat", Protocol: "https"}
	if err := SetupRepo(profilesDir, "work", pwd, c); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(log)
	s := string(out)
	if !strings.Contains(s, "gh GH_CONFIG_DIR="+filepath.Join(profilesDir, "work")) ||
		!strings.Contains(s, "auth setup-git") {
		t.Fatalf("setup-git not scoped/invoked: %s", s)
	}
	if !strings.Contains(s, "credential.https://github.com.username octocat") {
		t.Fatalf("credential username not set: %s", s)
	}
	if !strings.Contains(s, "-C "+pwd) {
		t.Fatalf("git not run in pwd: %s", s)
	}
	got := profile.ReadMappings(profilesDir)
	want := profile.Mapping{Dir: pwd, Profile: "work", Source: "gitconfig"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("mappings = %+v, want [%+v]", got, want)
	}
}

func TestSetupRepoSkipsUsernameWhenNoUser(t *testing.T) {
	log := fakeGhGit(t)
	c := Conf{Host: "github.com", Protocol: "https"}
	if err := SetupRepo(t.TempDir(), "work", t.TempDir(), c); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(log)
	if strings.Contains(string(out), "credential.https://github.com.username") {
		t.Fatalf("should not set username without GH_USER: %s", out)
	}
}
