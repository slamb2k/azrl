package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Scheme parameterizes the provider-agnostic profile mechanics so the same
// resolve/use/remove/list/label logic serves both Azure (.azprofile / AZ_*) and
// GitHub (.ghprofile / GH_*). Pointer is the repo pin filename; Reserved is the
// global-conf basename excluded from listings; DetailKey/LabelKey are the conf
// keys holding a profile's headline detail and optional display label; Prefix is
// the tool name used in error messages.
type Scheme struct {
	Pointer   string
	Reserved  string
	DetailKey string
	LabelKey  string
	Prefix    string
}

// Resolve returns arg when non-empty, otherwise the trimmed contents of the
// nearest Pointer file found walking up from dir.
func (s Scheme) Resolve(arg, dir string) (string, error) {
	if arg != "" {
		return arg, nil
	}
	d := dir
	for d != "" && d != string(filepath.Separator) {
		b, err := os.ReadFile(filepath.Join(d, s.Pointer))
		if err == nil {
			return strings.TrimSpace(string(b)), nil
		}
		d = filepath.Dir(d)
	}
	return "", fmt.Errorf("%s: no profile arg and no %s found from %s", s.Prefix, s.Pointer, dir)
}

// Locate walks up from dir to the nearest directory holding the Pointer file,
// returning that directory. ok is false when none is found.
func (s Scheme) Locate(dir string) (string, bool) {
	d := dir
	for d != "" && d != string(filepath.Separator) {
		if _, err := os.Stat(filepath.Join(d, s.Pointer)); err == nil {
			return d, true
		}
		d = filepath.Dir(d)
	}
	return "", false
}

// Use links pwd to an existing profile by writing pwd/<Pointer>, after verifying
// <confdir>/<name>.conf exists.
func (s Scheme) Use(name, confdir, pwd string) error {
	conf := filepath.Join(confdir, name+".conf")
	if _, err := os.Stat(conf); err != nil {
		return fmt.Errorf("%s: no such profile %q (missing %s)", s.Prefix, name, conf)
	}
	if err := os.WriteFile(filepath.Join(pwd, s.Pointer), []byte(name+"\n"), 0o644); err != nil {
		return err
	}
	return s.Touch(name, confdir, pwd)
}

// Touch bumps LAST_USED (now, RFC3339 UTC) and LAST_DIR (dir) in profile name's
// conf, adding either key if absent and preserving every other key and its order.
// When a Pointer file governing dir names this profile, it also records the
// pointer's directory in the mappings index (best-effort, never fails Touch).
func (s Scheme) Touch(name, confdir, dir string) error {
	path := filepath.Join(confdir, name+".conf")
	m, order, err := readOrderedKV(path)
	if err != nil {
		return err
	}
	set := func(k, v string) {
		if _, ok := m[k]; !ok {
			order = append(order, k)
		}
		m[k] = v
	}
	set("LAST_USED", time.Now().UTC().Format(time.RFC3339))
	set("LAST_DIR", dir)
	var b strings.Builder
	for _, k := range order {
		fmt.Fprintf(&b, "%s=%s\n", k, m[k])
	}
	if pdir, ok := s.Locate(dir); ok {
		if pb, perr := os.ReadFile(filepath.Join(pdir, s.Pointer)); perr == nil && strings.TrimSpace(string(pb)) == name {
			_ = RecordMapping(confdir, Mapping{Dir: pdir, Profile: name, Source: "pointer"})
		}
	}
	return writeAtomic(path, b.String())
}

// LastTouch reads LAST_USED and LAST_DIR back from profile name's conf, returning
// the zero time and "" when absent or unparseable.
func (s Scheme) LastTouch(name, confdir string) (time.Time, string) {
	m, _, err := readOrderedKV(filepath.Join(confdir, name+".conf"))
	if err != nil {
		return time.Time{}, ""
	}
	var t time.Time
	if v := m["LAST_USED"]; v != "" {
		if parsed, perr := time.Parse(time.RFC3339, v); perr == nil {
			t = parsed
		}
	}
	return t, m["LAST_DIR"]
}

// RemoveTargets returns the existing paths Remove would delete: the conf, the
// per-profile config dir, and pwd/<Pointer> only when it names this profile.
func (s Scheme) RemoveTargets(name, confdir, pwd string) []string {
	var targets []string
	conf := filepath.Join(confdir, name+".conf")
	if _, err := os.Stat(conf); err == nil {
		targets = append(targets, conf)
	}
	dir := filepath.Join(confdir, name)
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		targets = append(targets, dir)
	}
	ptr := filepath.Join(pwd, s.Pointer)
	if b, err := os.ReadFile(ptr); err == nil && strings.TrimSpace(string(b)) == name {
		targets = append(targets, ptr)
	}
	return targets
}

// Remove deletes the RemoveTargets and returns the list it removed.
func (s Scheme) Remove(name, confdir, pwd string) ([]string, error) {
	targets := s.RemoveTargets(name, confdir, pwd)
	for _, t := range targets {
		if err := os.RemoveAll(t); err != nil {
			return targets, err
		}
	}
	return targets, nil
}

// List returns every <name>.conf in confdir (except Reserved) with its Detail
// (from DetailKey) and Label (from LabelKey), sorted by name.
func (s Scheme) List(confdir string) ([]Listed, error) {
	entries, err := os.ReadDir(confdir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Listed
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".conf") {
			continue
		}
		name := strings.TrimSuffix(n, ".conf")
		if name == s.Reserved {
			continue
		}
		m, _, _ := readOrderedKV(filepath.Join(confdir, n))
		out = append(out, Listed{Name: name, Detail: m[s.DetailKey], Label: m[s.LabelKey]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SetLabel updates only the label key of profile name, preserving its other
// fields. An empty label reverts the display name to the slug.
func (s Scheme) SetLabel(name, confdir, label string) error {
	return s.SetKey(name, confdir, s.LabelKey, label)
}

// SetKey updates a single key of profile name's conf, preserving the other
// keys and their order (the key is appended when absent).
func (s Scheme) SetKey(name, confdir, key, value string) error {
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("profile: key %s value must be single-line", key)
	}
	path := filepath.Join(confdir, name+".conf")
	m, order, err := readOrderedKV(path)
	if err != nil {
		return err
	}
	if _, ok := m[key]; !ok {
		order = append(order, key)
	}
	m[key] = value
	var b strings.Builder
	for _, k := range order {
		fmt.Fprintf(&b, "%s=%s\n", k, m[k])
	}
	return writeAtomic(path, b.String())
}

// GetKey returns one key's value from the conf; "" when the key or the conf
// is missing (best-effort, display-only callers).
func (s Scheme) GetKey(name, confdir, key string) string {
	m, _, err := readOrderedKV(filepath.Join(confdir, name+".conf"))
	if err != nil {
		return ""
	}
	return m[key]
}

// readOrderedKV parses a KEY=value conf into a map plus the key order as first
// seen. Blank lines and lines without '=' are skipped.
func readOrderedKV(path string) (map[string]string, []string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	m := map[string]string{}
	var order []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if _, seen := m[k]; !seen {
			order = append(order, k)
		}
		m[k] = strings.TrimSpace(v)
	}
	return m, order, nil
}

// Unlink removes the calling directory's own pointer file and its mapping
// row, returning the profile it pointed at. It never reaches into parents:
// a directory governed from above is refused with the governing path — links
// are removed where they live.
func (s Scheme) Unlink(confdir, pwd string) (string, error) {
	ptr := filepath.Join(pwd, s.Pointer)
	b, err := os.ReadFile(ptr)
	if err != nil {
		if pdir, ok := s.Locate(pwd); ok {
			name, _ := s.Resolve("", pwd)
			return "", fmt.Errorf("%s: this directory is governed by %s (profile %s) — run unlink there",
				s.Prefix, filepath.Join(pdir, s.Pointer), name)
		}
		return "", fmt.Errorf("%s: nothing linked in %s", s.Prefix, pwd)
	}
	name := strings.TrimSpace(string(b))
	if err := os.Remove(ptr); err != nil {
		return "", err
	}
	_ = RemoveMapping(confdir, pwd, "pointer")
	return name, nil
}

// LinkedDirs lists the directories whose pointer-sourced mappings name the
// profile — the blast radius of deleting or re-signing it.
func (s Scheme) LinkedDirs(confdir, name string) []string {
	var dirs []string
	for _, m := range s.ReadMappings(confdir) {
		if m.Profile == name && m.Source == "pointer" {
			dirs = append(dirs, m.Dir)
		}
	}
	return dirs
}

// UnlinkAll removes every linked directory's pointer file and mapping row.
// On a mid-loop error it returns only the dirs actually processed, so
// callers don't report success for dirs left untouched.
func (s Scheme) UnlinkAll(confdir, name string) ([]string, error) {
	dirs := s.LinkedDirs(confdir, name)
	var done []string
	for _, d := range dirs {
		if err := os.Remove(filepath.Join(d, s.Pointer)); err != nil && !os.IsNotExist(err) {
			return done, err
		}
		_ = RemoveMapping(confdir, d, "pointer")
		done = append(done, d)
	}
	return done, nil
}

// ReplaceLinks repoints every linked directory at another profile — an edge
// rewrite only; provider-native extras (git credential setup, config sync)
// happen on the next use/login as usual. On a mid-loop error it returns only
// the dirs actually processed.
func (s Scheme) ReplaceLinks(confdir, oldName, newName string) ([]string, error) {
	if newName == oldName {
		return nil, fmt.Errorf("%s: cannot replace links with the profile being removed", s.Prefix)
	}
	if _, err := os.Stat(filepath.Join(confdir, newName+".conf")); err != nil {
		return nil, fmt.Errorf("%s: no such profile %q to replace links with", s.Prefix, newName)
	}
	dirs := s.LinkedDirs(confdir, oldName)
	var done []string
	for _, d := range dirs {
		if err := os.WriteFile(filepath.Join(d, s.Pointer), []byte(newName+"\n"), 0o644); err != nil {
			return done, err
		}
		_ = RecordMapping(confdir, Mapping{Dir: d, Profile: newName, Source: "pointer"})
		done = append(done, d)
	}
	return done, nil
}

// writeAtomic writes body to path via a temp file + rename.
func writeAtomic(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	return os.Rename(tmp.Name(), path)
}
