// Package profile holds azrl's pure profile logic: resolution, conf I/O, and
// name handling. No process execution lives here.
package profile

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	portRe = regexp.MustCompile(`localhost:(\d+)`)
	junkRe = regexp.MustCompile(`[^a-z0-9._-]+`)
	edgeRe = regexp.MustCompile(`^-+|-+$`)
)

// ExtractPort returns the callback port from an OAuth redirect URL, decoding the
// common %3A/%2F encodings first. Returns "" when no localhost:<port> is found.
func ExtractPort(url string) string {
	d := strings.ReplaceAll(url, "%3A", ":")
	d = strings.ReplaceAll(d, "%2F", "/")
	m := portRe.FindStringSubmatch(d)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// SanitizeName lowercases, collapses non [a-z0-9._-] runs to '-', and trims
// leading/trailing '-'.
func SanitizeName(raw string) string {
	s := strings.ToLower(raw)
	s = junkRe.ReplaceAllString(s, "-")
	s = edgeRe.ReplaceAllString(s, "")
	return s
}

// DefaultName returns arg verbatim when non-empty, else the sanitized basename
// of dir.
func DefaultName(arg, dir string) string {
	if arg != "" {
		return arg
	}
	return SanitizeName(filepath.Base(dir))
}
