package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slamb2k/azrl/internal/provider"
)

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init failed: %v: %s", err, out)
	}
}

func setCredentialUser(t *testing.T, dir, host, user string) {
	t.Helper()
	key := fmt.Sprintf("credential.https://%s.username", host)
	if err := exec.Command("git", "-C", dir, "config", "--local", key, user).Run(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildOverviewGithubConflictAndUnmanaged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	gh := filepath.Join(home, ".github-profiles")
	os.MkdirAll(gh, 0o755)
	os.WriteFile(filepath.Join(gh, "work.conf"), []byte("GH_HOST=github.com\nGH_USER=user-a\n"), 0o644)
	os.WriteFile(filepath.Join(gh, "personal.conf"), []byte("GH_HOST=github.com\nGH_USER=user-b\n"), 0o644)

	// A repo whose git config (user-a → work) disagrees with its .ghprofile
	// (personal): git config wins, the row carries the conflict.
	repo := filepath.Join(home, "repo")
	initGitRepo(t, repo)
	setCredentialUser(t, repo, "github.com", "user-a")
	os.WriteFile(filepath.Join(repo, ".ghprofile"), []byte("personal\n"), 0o644)

	// A repo whose git config names an identity matching no profile: unmanaged.
	stray := filepath.Join(home, "stray")
	initGitRepo(t, stray)
	setCredentialUser(t, stray, "github.com", "ghost")

	os.WriteFile(filepath.Join(gh, "mappings"),
		[]byte(repo+"\twork\tgitconfig\n"+stray+"\twork\tgitconfig\n"), 0o644)

	ov := BuildOverview(provider.All(), "")

	var conflict, unmanaged *MappingRow
	for i, r := range ov.Mappings {
		if r.Dir == repo {
			conflict = &ov.Mappings[i]
		}
		if r.Dir == stray {
			unmanaged = &ov.Mappings[i]
		}
	}
	if conflict == nil || conflict.Profile != "work" || conflict.Source != "gitconfig" {
		t.Fatalf("conflict row = %+v", conflict)
	}
	if conflict.Conflict == nil || conflict.Conflict.PointerProfile != "personal" {
		t.Fatalf("conflict detail = %+v", conflict.Conflict)
	}
	if unmanaged == nil || unmanaged.Unmanaged != "ghost@github.com" || unmanaged.Profile != "" {
		t.Fatalf("unmanaged row = %+v", unmanaged)
	}

	// Both sides of the conflict count as mapped: neither is unmapped (AC-011).
	for _, u := range ov.Unmapped {
		if u.Provider == "github" && (u.Status.ProfileName == "work" || u.Status.ProfileName == "personal") {
			t.Fatalf("mapped profile %q listed as unmapped", u.Status.ProfileName)
		}
	}

	// The unmanaged row is adoptable: it yields an item carrying its dir.
	found := false
	for _, it := range overviewItems(ov) {
		if it.adoptDir == stray && it.provider == "github" {
			found = true
		}
	}
	if !found {
		t.Fatal("unmanaged mapping produced no adoptable item")
	}
}

func TestBuildOverviewScopeNearestAncestorWins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "a.conf"), []byte("AZ_TENANT=a.com\n"), 0o644)
	os.WriteFile(filepath.Join(az, "b.conf"), []byte("AZ_TENANT=b.com\n"), 0o644)

	parent := filepath.Join(home, "work")
	child := filepath.Join(parent, "proj")
	cwd := filepath.Join(child, "deep")
	other := filepath.Join(home, "other")
	os.MkdirAll(cwd, 0o755)
	os.MkdirAll(other, 0o755)
	os.WriteFile(filepath.Join(parent, ".azprofile"), []byte("a\n"), 0o644)
	os.WriteFile(filepath.Join(child, ".azprofile"), []byte("b\n"), 0o644)
	os.WriteFile(filepath.Join(other, ".azprofile"), []byte("a\n"), 0o644)
	os.WriteFile(filepath.Join(az, "mappings"),
		[]byte(parent+"\ta\tpointer\n"+child+"\tb\tpointer\n"+other+"\ta\tpointer\n"), 0o644)

	scopes := func(cwd string) map[string]string {
		out := map[string]string{}
		for _, r := range BuildOverview(provider.All(), cwd).Mappings {
			out[r.Dir] = r.Scope
		}
		return out
	}

	// Two ancestors on the chain: only the nearest gets ↑; off-chain rows are neutral.
	got := scopes(cwd)
	if got[child] != ScopeAncestor || got[parent] != ScopeNone || got[other] != ScopeNone {
		t.Fatalf("scopes from %s = %v", cwd, got)
	}

	// From the mapping's own dir it is ● and the ancestor stays neutral.
	got = scopes(child)
	if got[child] != ScopeCwd || got[parent] != ScopeNone {
		t.Fatalf("scopes from %s = %v", child, got)
	}

	// No cwd → no markers at all.
	got = scopes("")
	for d, s := range got {
		if s != ScopeNone {
			t.Fatalf("scope for %s without a cwd = %s", d, s)
		}
	}
}

func TestBuildOverviewSelfHealsHandMadePointer(t *testing.T) {
	// AC-007: a hand-written .azprofile in a dir unknown to the index joins the
	// index (and the view) after any resolution — no Touch/use/login involved.
	seedDashHome(t)
	home := os.Getenv("HOME")
	work := filepath.Join(home, "work")
	os.MkdirAll(work, 0o755)
	os.WriteFile(filepath.Join(work, ".azprofile"), []byte("acme\n"), 0o644)

	ov := BuildOverview(provider.All(), work)
	found := false
	for _, r := range ov.Mappings {
		if r.Provider == "azure" && r.Dir == work && r.Profile == "acme" && r.Source == "pointer" {
			found = true
		}
	}
	if !found {
		t.Fatalf("hand-made pointer not self-healed into mappings: %+v", ov.Mappings)
	}
	b, err := os.ReadFile(filepath.Join(home, ".azure-profiles", "mappings"))
	if err != nil || !strings.Contains(string(b), work+"\tacme\tpointer") {
		t.Fatalf("index not updated (err=%v):\n%s", err, b)
	}
}

func TestBuildOverviewSelfHealsGitConfigMapping(t *testing.T) {
	// AC-007 for GitHub's native mechanism: a repo-local credential username
	// resolving to a managed profile joins the index on read, recorded against
	// the repo root even when resolved from a subdirectory.
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	gh := filepath.Join(home, ".github-profiles")
	os.MkdirAll(gh, 0o755)
	os.WriteFile(filepath.Join(gh, "work.conf"), []byte("GH_HOST=github.com\nGH_USER=user-a\n"), 0o644)

	repo := filepath.Join(home, "repo")
	initGitRepo(t, repo)
	setCredentialUser(t, repo, "github.com", "user-a")
	sub := filepath.Join(repo, "sub")
	os.MkdirAll(sub, 0o755)

	ov := BuildOverview(provider.All(), sub)
	found := false
	for _, r := range ov.Mappings {
		if r.Provider == "github" && r.Profile == "work" && r.Source == "gitconfig" {
			found = true
		}
	}
	if !found {
		t.Fatalf("gitconfig mapping not self-healed: %+v", ov.Mappings)
	}
	b, err := os.ReadFile(filepath.Join(gh, "mappings"))
	if err != nil || !strings.Contains(string(b), "\twork\tgitconfig") {
		t.Fatalf("index not updated (err=%v):\n%s", err, b)
	}
	// Recorded at the repo root, not the subdirectory.
	if strings.Contains(string(b), sub+"\t") {
		t.Fatalf("gitconfig mapping recorded against the subdir:\n%s", b)
	}
}

func TestBuildOverviewStalePointerEntryHealsOnRead(t *testing.T) {
	// Spec §9 / AC-006: an index entry whose dir exists but whose pointer now
	// names a different profile is replaced on read — not duplicated.
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "a.conf"), []byte("AZ_TENANT=a.com\n"), 0o644)
	os.WriteFile(filepath.Join(az, "b.conf"), []byte("AZ_TENANT=b.com\n"), 0o644)

	proj := filepath.Join(home, "proj")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, ".azprofile"), []byte("b\n"), 0o644)
	os.WriteFile(filepath.Join(az, "mappings"), []byte(proj+"\ta\tpointer\n"), 0o644)

	var rows []MappingRow
	for _, r := range BuildOverview(provider.All(), "").Mappings {
		if r.Provider == "azure" && r.Dir == proj {
			rows = append(rows, r)
		}
	}
	if len(rows) != 1 || rows[0].Profile != "b" {
		t.Fatalf("stale pointer entry not replaced in place: %+v", rows)
	}
	b, err := os.ReadFile(filepath.Join(az, "mappings"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), proj+"\tb\tpointer\n"; got != want {
		t.Fatalf("healed index = %q, want %q", got, want)
	}
}

func TestBuildOverviewMappedProfilesLeaveUnmapped(t *testing.T) {
	seedDashHome(t)
	home := os.Getenv("HOME")
	work := filepath.Join(home, "work")
	os.MkdirAll(work, 0o755)
	os.WriteFile(filepath.Join(work, ".azprofile"), []byte("acme\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "mappings"),
		[]byte(work+"\tacme\tpointer\n"), 0o644)

	ov := BuildOverview(provider.All(), work)
	if len(ov.Mappings) != 1 || ov.Mappings[0].Profile != "acme" || ov.Mappings[0].Scope != ScopeCwd {
		t.Fatalf("mappings = %+v", ov.Mappings)
	}
	if ov.Mappings[0].Provider != "azure" || ov.Mappings[0].Pointer != ".azprofile" {
		t.Fatalf("mapping row provider/pointer = %+v", ov.Mappings[0])
	}
	for _, u := range ov.Unmapped {
		if u.Status.ProfileName == "acme" {
			t.Fatalf("mapped acme still unmapped: %+v", ov.Unmapped)
		}
	}
	// The GitHub profile has no mapping, so it stays visible with its identity.
	found := false
	for _, u := range ov.Unmapped {
		if u.Provider == "github" && u.Status.ProfileName == "work" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unmapped github profile missing: %+v", ov.Unmapped)
	}
}

func TestBuildOverviewHealsStaleGitConfigEntry(t *testing.T) {
	// A gitconfig-sourced index entry whose repo now names a different managed
	// user is replaced in place from the same resolution that renders the row.
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	gh := filepath.Join(home, ".github-profiles")
	os.MkdirAll(gh, 0o755)
	os.WriteFile(filepath.Join(gh, "old.conf"), []byte("GH_HOST=github.com\nGH_USER=user-old\n"), 0o644)
	os.WriteFile(filepath.Join(gh, "fresh.conf"), []byte("GH_HOST=github.com\nGH_USER=user-fresh\n"), 0o644)

	repo := filepath.Join(home, "repo")
	initGitRepo(t, repo)
	setCredentialUser(t, repo, "github.com", "user-fresh")
	os.WriteFile(filepath.Join(gh, "mappings"), []byte(repo+"\told\tgitconfig\n"), 0o644)

	ov := BuildOverview(provider.All(), home)
	found := false
	for _, r := range ov.Mappings {
		if r.Provider == "github" && r.Dir == repo {
			if r.Profile != "fresh" || r.Source != "gitconfig" {
				t.Fatalf("row = %+v, want fresh/gitconfig", r)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no github row for %s: %+v", repo, ov.Mappings)
	}
	b, _ := os.ReadFile(filepath.Join(gh, "mappings"))
	if !strings.Contains(string(b), repo+"\tfresh\tgitconfig") || strings.Contains(string(b), "\told\t") {
		t.Fatalf("index not healed:\n%s", b)
	}
}

func TestBuildOverviewDropsDeadGitConfigEntry(t *testing.T) {
	// A gitconfig-sourced index entry whose repo no longer carries a credential
	// username (and has no pointer) is dropped from the index on read.
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	gh := filepath.Join(home, ".github-profiles")
	os.MkdirAll(gh, 0o755)
	os.WriteFile(filepath.Join(gh, "work.conf"), []byte("GH_HOST=github.com\nGH_USER=user-a\n"), 0o644)

	repo := filepath.Join(home, "repo")
	initGitRepo(t, repo)
	os.WriteFile(filepath.Join(gh, "mappings"), []byte(repo+"\twork\tgitconfig\n"), 0o644)

	ov := BuildOverview(provider.All(), home)
	for _, r := range ov.Mappings {
		if r.Provider == "github" && r.Dir == repo {
			t.Fatalf("dead gitconfig entry still rendered: %+v", r)
		}
	}
	b, _ := os.ReadFile(filepath.Join(gh, "mappings"))
	if strings.Contains(string(b), repo) {
		t.Fatalf("dead entry not dropped:\n%s", b)
	}
}
