package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mappingsPath(profilesDir string) string {
	return filepath.Join(profilesDir, "mappings")
}

func readIndexFile(t *testing.T, profilesDir string) string {
	t.Helper()
	b, err := os.ReadFile(mappingsPath(profilesDir))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMappingRecordReadRoundTrip(t *testing.T) {
	profilesDir := t.TempDir()
	a, b := t.TempDir(), t.TempDir()
	if err := RecordMapping(profilesDir, Mapping{Dir: a, Profile: "work", Source: "pointer"}); err != nil {
		t.Fatal(err)
	}
	if err := RecordMapping(profilesDir, Mapping{Dir: b, Profile: "personal", Source: "gitconfig"}); err != nil {
		t.Fatal(err)
	}
	got := ReadMappings(profilesDir)
	want := []Mapping{
		{Dir: a, Profile: "work", Source: "pointer"},
		{Dir: b, Profile: "personal", Source: "gitconfig"},
	}
	if len(got) != len(want) {
		t.Fatalf("want %d mappings, got %+v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mapping %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestMappingReadMissingFile(t *testing.T) {
	profilesDir := t.TempDir()
	if got := ReadMappings(profilesDir); got != nil {
		t.Fatalf("missing index: got %+v", got)
	}
	if _, err := os.Stat(mappingsPath(profilesDir)); !os.IsNotExist(err) {
		t.Fatal("read must not create the index file")
	}
}

func TestMappingReadPrunes(t *testing.T) {
	live := t.TempDir()
	gone := filepath.Join(t.TempDir(), "gone")
	tests := []struct {
		name    string
		lines   []string
		want    []Mapping
		rewrite bool
	}{
		{
			name:    "missing dir dropped and file rewritten",
			lines:   []string{gone + "\twork\tpointer", live + "\tpersonal\tgitconfig"},
			want:    []Mapping{{Dir: live, Profile: "personal", Source: "gitconfig"}},
			rewrite: true,
		},
		{
			name: "malformed lines skipped silently",
			lines: []string{
				"not-a-mapping",
				live + "\twork",
				"relative/dir\twork\tpointer",
				live + "\t\tpointer",
				live + "\twork\tpointer",
			},
			want:    []Mapping{{Dir: live, Profile: "work", Source: "pointer"}},
			rewrite: true,
		},
		{
			name:    "clean file untouched",
			lines:   []string{live + "\twork\tpointer"},
			want:    []Mapping{{Dir: live, Profile: "work", Source: "pointer"}},
			rewrite: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profilesDir := t.TempDir()
			original := strings.Join(tt.lines, "\n") + "\n"
			if err := os.WriteFile(mappingsPath(profilesDir), []byte(original), 0o644); err != nil {
				t.Fatal(err)
			}
			got := ReadMappings(profilesDir)
			if len(got) != len(tt.want) {
				t.Fatalf("want %d mappings, got %+v", len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("mapping %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
			after := readIndexFile(t, profilesDir)
			if tt.rewrite {
				var body strings.Builder
				for _, m := range tt.want {
					body.WriteString(m.Dir + "\t" + m.Profile + "\t" + m.Source + "\n")
				}
				if after != body.String() {
					t.Fatalf("pruned file = %q, want %q", after, body.String())
				}
			} else if after != original {
				t.Fatalf("clean file rewritten: %q", after)
			}
		})
	}
}

func TestMappingRecordReplacesNotDuplicates(t *testing.T) {
	profilesDir := t.TempDir()
	a, b, c := t.TempDir(), t.TempDir(), t.TempDir()
	for _, m := range []Mapping{
		{Dir: a, Profile: "work", Source: "pointer"},
		{Dir: b, Profile: "work", Source: "pointer"},
		{Dir: c, Profile: "personal", Source: "gitconfig"},
	} {
		if err := RecordMapping(profilesDir, m); err != nil {
			t.Fatal(err)
		}
	}
	if err := RecordMapping(profilesDir, Mapping{Dir: b, Profile: "personal", Source: "pointer"}); err != nil {
		t.Fatal(err)
	}
	got := ReadMappings(profilesDir)
	want := []Mapping{
		{Dir: a, Profile: "work", Source: "pointer"},
		{Dir: b, Profile: "personal", Source: "pointer"},
		{Dir: c, Profile: "personal", Source: "gitconfig"},
	}
	if len(got) != len(want) {
		t.Fatalf("want %d mappings, got %+v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mapping %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSchemeReadMappingsRevalidatesPointers(t *testing.T) {
	s := Scheme{Pointer: ".azprofile"}
	profilesDir := t.TempDir()
	same, changed, gone, git := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(same, ".azprofile"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Spec §9: pointer now names a different profile → replaced on read.
	if err := os.WriteFile(filepath.Join(changed, ".azprofile"), []byte("personal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// gone has no pointer file; git is gitconfig-sourced (dir-exists-only).
	lines := same + "\twork\tpointer\n" + changed + "\twork\tpointer\n" +
		gone + "\twork\tpointer\n" + git + "\twork\tgitconfig\n"
	if err := os.WriteFile(mappingsPath(profilesDir), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	got := s.ReadMappings(profilesDir)
	want := []Mapping{
		{Dir: same, Profile: "work", Source: "pointer"},
		{Dir: changed, Profile: "personal", Source: "pointer"},
		{Dir: git, Profile: "work", Source: "gitconfig"},
	}
	if len(got) != len(want) {
		t.Fatalf("want %d mappings, got %+v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mapping %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	after := readIndexFile(t, profilesDir)
	wantFile := same + "\twork\tpointer\n" + changed + "\tpersonal\tpointer\n" + git + "\twork\tgitconfig\n"
	if after != wantFile {
		t.Fatalf("healed file = %q, want %q", after, wantFile)
	}
}

func TestSchemeReadMappingsCleanFileUntouched(t *testing.T) {
	s := Scheme{Pointer: ".azprofile"}
	profilesDir := t.TempDir()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".azprofile"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := dir + "\twork\tpointer\n"
	if err := os.WriteFile(mappingsPath(profilesDir), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(mappingsPath(profilesDir), past, past); err != nil {
		t.Fatal(err)
	}
	got := s.ReadMappings(profilesDir)
	if len(got) != 1 || got[0] != (Mapping{Dir: dir, Profile: "work", Source: "pointer"}) {
		t.Fatalf("clean read = %+v", got)
	}
	fi, err := os.Stat(mappingsPath(profilesDir))
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().Equal(past) {
		t.Fatalf("clean file was rewritten (mtime %v)", fi.ModTime())
	}
}

func TestMappingRecordNoOpWhenUnchanged(t *testing.T) {
	profilesDir := t.TempDir()
	dir := t.TempDir()
	m := Mapping{Dir: dir, Profile: "work", Source: "pointer"}
	if err := RecordMapping(profilesDir, m); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(profilesDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(profilesDir, 0o755)
	if err := RecordMapping(profilesDir, m); err != nil {
		t.Fatalf("unchanged record must not write: %v", err)
	}
}
