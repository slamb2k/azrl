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
	if strings.Contains(s, "setup-git") || strings.Contains(s, "--global") {
		t.Fatalf("must never touch global git config: %s", s)
	}
	if !strings.Contains(s, "config --local --replace-all credential.https://github.com.helper") ||
		!strings.Contains(s, "config --local --add credential.https://github.com.helper !GH_CONFIG_DIR='"+filepath.Join(profilesDir, "work")+"'") {
		t.Fatalf("repo-local helper not pinned to the profile dir: %s", s)
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

// fakeGhOnly shims gh but leaves git real so rev-parse resolves the repo root.
func fakeGhOnly(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/usr/bin/env bash\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSetupRepoRecordsRepoRootMappingFromSubdir(t *testing.T) {
	fakeGhOnly(t)
	root := initGitRepo(t)
	sub := filepath.Join(root, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	profilesDir := t.TempDir()
	c := Conf{Host: "github.com", User: "octocat", Protocol: "https"}
	if err := SetupRepo(profilesDir, "work", sub, c); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	got := profile.ReadMappings(profilesDir)
	if len(got) != 1 || got[0].Dir != want || got[0].Profile != "work" || got[0].Source != "gitconfig" {
		t.Fatalf("mappings = %+v, want Dir=%s Profile=work Source=gitconfig", got, want)
	}
}

// TestSetupRepoLeavesGlobalGitConfigAlone runs real git with an isolated
// HOME: an existing global helper (e.g. GCM) must survive untouched, and the
// repo gets its own reset + gh helper pinned to the profile's config dir.
func TestSetupRepoLeavesGlobalGitConfigAlone(t *testing.T) {
	fakeGhOnly(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	global := filepath.Join(home, ".gitconfig")
	before := "[credential \"https://github.com\"]\n\thelper = manager\n"
	if err := os.WriteFile(global, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	root := initGitRepo(t)
	profilesDir := t.TempDir()
	c := Conf{Host: "github.com", User: "octocat", Protocol: "https"}
	if err := SetupRepo(profilesDir, "work", root, c); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(global); string(after) != before {
		t.Fatalf("global git config changed:\n%s", after)
	}
	out, err := GitCmd(root, "config", "--local", "--get-all", "credential.https://github.com.helper").Output()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 2 || lines[0] != "" || !strings.HasPrefix(lines[1], "!GH_CONFIG_DIR='"+filepath.Join(profilesDir, "work")+"' ") {
		t.Fatalf("local helpers = %q", lines)
	}
	// Idempotent: a second run leaves exactly the same two entries.
	if err := SetupRepo(profilesDir, "work", root, c); err != nil {
		t.Fatal(err)
	}
	out2, _ := GitCmd(root, "config", "--local", "--get-all", "credential.https://github.com.helper").Output()
	if string(out2) != string(out) {
		t.Fatalf("not idempotent: %q then %q", out, out2)
	}
}
