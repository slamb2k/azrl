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

// SetLabel updates only LabelKey in profile name's conf, preserving every other
// key and their order. An empty label reverts the display name to the slug.
func (s Scheme) SetLabel(name, confdir, label string) error {
	path := filepath.Join(confdir, name+".conf")
	m, order, err := readOrderedKV(path)
	if err != nil {
		return err
	}
	if _, ok := m[s.LabelKey]; !ok {
		order = append(order, s.LabelKey)
	}
	m[s.LabelKey] = label
	var b strings.Builder
	for _, k := range order {
		fmt.Fprintf(&b, "%s=%s\n", k, m[k])
	}
	return writeAtomic(path, b.String())
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
