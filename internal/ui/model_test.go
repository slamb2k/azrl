package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)

	// dir linked via .azprofile
	linked := t.TempDir()
	os.WriteFile(filepath.Join(linked, ".azprofile"), []byte("acme\n"), 0o644)
	if got := contextLine(linked); !strings.Contains(got, "This dir") || !strings.Contains(got, "acme") {
		t.Fatalf("linked: %q", got)
	}

	// dir whose basename matches an existing conf -> offer link
	matchDir := filepath.Join(t.TempDir(), "acme")
	os.MkdirAll(matchDir, 0o755)
	if got := contextLine(matchDir); !strings.Contains(strings.ToLower(got), "link") {
		t.Fatalf("match: %q", got)
	}

	// unknown dir -> offer create
	if got := contextLine(filepath.Join(t.TempDir(), "brand-new")); !strings.Contains(strings.ToLower(got), "create") {
		t.Fatalf("unknown: %q", got)
	}
}

func TestModelViewRenders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	m := NewModel()
	m.width, m.height = 80, 24
	v := m.View()
	if !strings.Contains(v, "Azure Remote Login") {
		t.Fatalf("view missing banner:\n%s", v)
	}
}
