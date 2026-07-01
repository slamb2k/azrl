package provider

import (
	"os"
	"time"
)

// LatestMtime returns the newest of base and the modtimes of the given files.
// Missing or unreadable files are ignored. Providers use it to fold a token
// cache file's mtime into LastUsed so external CLI usage re-sorts the dashboard.
// It only stats files, so it never makes a network call.
func LatestMtime(base time.Time, paths ...string) time.Time {
	latest := base
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if mt := fi.ModTime(); mt.After(latest) {
			latest = mt
		}
	}
	return latest
}

// ExistingDirs filters paths to those that exist and are directories, deduping
// while preserving order. It is best-effort: unreadable or missing paths are
// silently skipped. Providers use it to implement WatchDirs.
func ExistingDirs(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	var out []string
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// ChildDirs returns dir plus its immediate subdirectories that exist. It is a
// convenience for providers whose profiles dir holds per-profile isolated
// config dirs. Best-effort: an unreadable dir yields just dir (if it exists).
func ChildDirs(dir string) []string {
	dirs := []string{dir}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ExistingDirs(dirs)
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, dir+string(os.PathSeparator)+e.Name())
		}
	}
	return ExistingDirs(dirs)
}
