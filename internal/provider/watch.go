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
