package github

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// initGitRepo creates a real git repo in a temp dir (real git per the spec's
// git-config read-back test strategy).
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := GitCmd(dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init failed: %v: %s", err, out)
	}
	return dir
}

func setCredentialUser(t *testing.T, dir, host, user string) {
	t.Helper()
	key := fmt.Sprintf("credential.https://%s.username", host)
	if err := GitCmd(dir, "config", "--local", key, user).Run(); err != nil {
		t.Fatal(err)
	}
}

func writeGhConf(t *testing.T, confdir, name, host, user string) {
	t.Helper()
	body := fmt.Sprintf("GH_HOST=%s\nGH_USER=%s\n", host, user)
	if err := os.WriteFile(filepath.Join(confdir, name+".conf"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePointer(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".ghprofile"), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveDirGitConfigWinsOverPointer(t *testing.T) {
	confdir := t.TempDir()
	writeGhConf(t, confdir, "work", "github.com", "user-a")
	writeGhConf(t, confdir, "personal", "github.com", "user-b")
	repo := initGitRepo(t)
	setCredentialUser(t, repo, "github.com", "user-a")
	writePointer(t, repo, "personal")

	r := ResolveDir(repo, confdir)
	if r.Profile != "work" || r.Source != SourceGitConfig {
		t.Fatalf("Resolution = %+v, want work via %s", r, SourceGitConfig)
	}
	if r.Unmanaged != "" {
		t.Fatalf("Unmanaged = %q, want empty", r.Unmanaged)
	}
	if r.Conflict == nil {
		t.Fatal("Conflict not reported for disagreeing .ghprofile")
	}
	want := Conflict{GitConfigProfile: "work", GitConfigUser: "user-a@github.com", PointerProfile: "personal"}
	if *r.Conflict != want {
		t.Fatalf("Conflict = %+v, want %+v", *r.Conflict, want)
	}
}

func TestResolveDirGitConfigAgreesWithPointer(t *testing.T) {
	confdir := t.TempDir()
	writeGhConf(t, confdir, "work", "github.com", "user-a")
	repo := initGitRepo(t)
	setCredentialUser(t, repo, "github.com", "user-a")
	writePointer(t, repo, "work")

	r := ResolveDir(repo, confdir)
	if r.Profile != "work" || r.Source != SourceGitConfig || r.Conflict != nil || r.Unmanaged != "" {
		t.Fatalf("Resolution = %+v, want work via %s with no conflict", r, SourceGitConfig)
	}
}

func TestResolveDirUnmanagedUsername(t *testing.T) {
	confdir := t.TempDir()
	writeGhConf(t, confdir, "work", "github.com", "user-a")
	repo := initGitRepo(t)
	setCredentialUser(t, repo, "github.com", "stranger")

	r := ResolveDir(repo, confdir)
	if r.Profile != "" || r.Source != "" {
		t.Fatalf("Resolution = %+v, want no profile", r)
	}
	if r.Unmanaged != "stranger@github.com" {
		t.Fatalf("Unmanaged = %q, want stranger@github.com", r.Unmanaged)
	}
}

func TestResolveDirUnmanagedFallsBackToPointer(t *testing.T) {
	confdir := t.TempDir()
	writeGhConf(t, confdir, "work", "github.com", "user-a")
	repo := initGitRepo(t)
	setCredentialUser(t, repo, "github.com", "stranger")
	writePointer(t, repo, "work")

	r := ResolveDir(repo, confdir)
	if r.Profile != "work" || r.Source != SourcePointer {
		t.Fatalf("Resolution = %+v, want work via %s", r, SourcePointer)
	}
	if r.Unmanaged != "stranger@github.com" {
		t.Fatalf("Unmanaged = %q, want stranger@github.com", r.Unmanaged)
	}
}

func TestResolveDirNoRepoPointerFallback(t *testing.T) {
	confdir := t.TempDir()
	writeGhConf(t, confdir, "work", "github.com", "user-a")
	root := t.TempDir()
	writePointer(t, root, "work")
	sub := filepath.Join(root, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	r := ResolveDir(sub, confdir)
	if r.Profile != "work" || r.Source != SourcePointer || r.Conflict != nil || r.Unmanaged != "" {
		t.Fatalf("Resolution = %+v, want work via %s", r, SourcePointer)
	}
}

func TestResolveDirNothing(t *testing.T) {
	r := ResolveDir(t.TempDir(), t.TempDir())
	if r != (Resolution{}) {
		t.Fatalf("Resolution = %+v, want zero", r)
	}
}

func TestProviderResolveNativeFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	confdir := filepath.Join(home, ".github-profiles")
	if err := os.MkdirAll(confdir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGhConf(t, confdir, "work", "github.com", "user-a")
	writeGhConf(t, confdir, "personal", "github.com", "user-b")
	repo := initGitRepo(t)
	setCredentialUser(t, repo, "github.com", "user-a")
	writePointer(t, repo, "personal")

	got, err := Provider{}.Resolve("", repo)
	if err != nil || got != "work" {
		t.Fatalf("Resolve = %q, %v; want work", got, err)
	}
}
