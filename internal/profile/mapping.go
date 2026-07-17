package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Mapping is one directory→profile association recorded in a provider's
// mappings index file. Dir is absolute; Source is "pointer" or "gitconfig".
type Mapping struct {
	Dir     string
	Profile string
	Source  string
}

// ReadMappings returns the entries of <profilesDir>/mappings — one
// tab-separated <abs-dir>\t<profile>\t<source> per line — in file order.
// Malformed lines and entries whose directory no longer exists are dropped;
// when that pruned anything the file is rewritten atomically (best-effort).
func ReadMappings(profilesDir string) []Mapping {
	return readMappings(profilesDir, nil)
}

// ReadMappings is the Scheme-aware read: on top of ReadMappings' pruning it
// re-verifies every pointer-sourced entry against the pointer file itself —
// a missing (or empty) pointer drops the entry, and a pointer now naming a
// different profile replaces the entry in place (self-heal, not duplicated).
// gitconfig-sourced entries keep the dir-exists-only check; re-resolving them
// is the github.ResolveDir consumers' job (which heal the index in turn). Any
// change is persisted atomically.
func (s Scheme) ReadMappings(profilesDir string) []Mapping {
	return readMappings(profilesDir, func(m Mapping) (Mapping, bool) {
		if m.Source != "pointer" {
			return m, true
		}
		b, err := os.ReadFile(filepath.Join(m.Dir, s.Pointer))
		if err != nil {
			return Mapping{}, false
		}
		name := strings.TrimSpace(string(b))
		if name == "" {
			return Mapping{}, false
		}
		m.Profile = name
		return m, true
	})
}

// readMappings is the shared read path: prune malformed lines and missing
// dirs, then apply revalidate (when non-nil) to heal or drop each survivor.
// Whenever anything changed the file is rewritten atomically (best-effort).
func readMappings(profilesDir string, revalidate func(Mapping) (Mapping, bool)) []Mapping {
	path := filepath.Join(profilesDir, "mappings")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var kept []Mapping
	changed := false
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m, ok := parseMapping(line)
		if !ok {
			changed = true
			continue
		}
		if fi, err := os.Stat(m.Dir); err != nil || !fi.IsDir() {
			changed = true
			continue
		}
		if revalidate != nil {
			healed, keep := revalidate(m)
			if !keep {
				changed = true
				continue
			}
			if healed != m {
				changed = true
				m = healed
			}
		}
		kept = append(kept, m)
	}
	if changed {
		var body strings.Builder
		for _, m := range kept {
			fmt.Fprintf(&body, "%s\t%s\t%s\n", m.Dir, m.Profile, m.Source)
		}
		WriteAtomic(path, body.String())
	}
	return kept
}

// RecordMapping appends m to <profilesDir>/mappings, replacing in place any
// existing entry with the same Dir and Source rather than duplicating it. It
// is a no-op (no write) when that entry already records the same profile;
// otherwise the rewrite is atomic and preserves every other line and order.
func RecordMapping(profilesDir string, m Mapping) error {
	path := filepath.Join(profilesDir, "mappings")
	var lines []string
	if b, err := os.ReadFile(path); err == nil {
		s := strings.TrimRight(string(b), "\n")
		if s != "" {
			lines = strings.Split(s, "\n")
		}
	}
	entry := m.Dir + "\t" + m.Profile + "\t" + m.Source
	replaced := false
	for i, line := range lines {
		e, ok := parseMapping(line)
		if !ok || e.Dir != m.Dir || e.Source != m.Source {
			continue
		}
		if e.Profile == m.Profile {
			return nil
		}
		lines[i] = entry
		replaced = true
		break
	}
	if !replaced {
		lines = append(lines, entry)
	}
	return WriteAtomic(path, strings.Join(lines, "\n")+"\n")
}

// RemoveMapping drops the entry with the given Dir and Source from
// <profilesDir>/mappings. It is a no-op (no write) when no such entry exists;
// otherwise the rewrite is atomic and preserves every other line and order.
func RemoveMapping(profilesDir, dir, source string) error {
	path := filepath.Join(profilesDir, "mappings")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	var kept []string
	removed := false
	for _, line := range strings.Split(s, "\n") {
		if e, ok := parseMapping(line); ok && e.Dir == dir && e.Source == source {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	if !removed {
		return nil
	}
	body := ""
	if len(kept) > 0 {
		body = strings.Join(kept, "\n") + "\n"
	}
	return WriteAtomic(path, body)
}

// parseMapping parses one index line; ok is false when the line is malformed
// (wrong field count, an empty field, or a non-absolute directory).
func parseMapping(line string) (Mapping, bool) {
	f := strings.Split(line, "\t")
	if len(f) != 3 {
		return Mapping{}, false
	}
	m := Mapping{Dir: f[0], Profile: f[1], Source: f[2]}
	if m.Dir == "" || m.Profile == "" || m.Source == "" || !filepath.IsAbs(m.Dir) {
		return Mapping{}, false
	}
	return m, true
}
