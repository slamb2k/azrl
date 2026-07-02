package provider_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/slamb2k/azrl/internal/provider"
)

func TestMatchProfileMapsIdentityToProfile(t *testing.T) {
	statuses := []provider.Status{
		{ProfileName: "work", Identity: "simon@contoso.com"},
		{ProfileName: "personal", Identity: "simon@gmail.com"},
	}
	if got := provider.MatchProfile(statuses, "simon@gmail.com"); got != "personal" {
		t.Fatalf("MatchProfile = %q, want personal", got)
	}
}

func TestMatchProfileUnmanagedWhenNoMatch(t *testing.T) {
	statuses := []provider.Status{
		{ProfileName: "work", Identity: "simon@contoso.com"},
	}
	if got := provider.MatchProfile(statuses, "stranger@example.com"); got != "" {
		t.Fatalf("MatchProfile = %q, want \"\"", got)
	}
}

func TestMatchProfileBlankIdentityIsUnmanaged(t *testing.T) {
	statuses := []provider.Status{
		{ProfileName: "work", Identity: ""},
	}
	if got := provider.MatchProfile(statuses, ""); got != "" {
		t.Fatalf("MatchProfile = %q, want \"\"", got)
	}
}

func TestMatchProfileMostRecentlyUsedWinsOnDuplicates(t *testing.T) {
	older := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	statuses := []provider.Status{
		{ProfileName: "old", Identity: "simon@contoso.com", LastUsed: older},
		{ProfileName: "fresh", Identity: "simon@contoso.com", LastUsed: newer},
	}
	if got := provider.MatchProfile(statuses, "simon@contoso.com"); got != "fresh" {
		t.Fatalf("MatchProfile = %q, want fresh (most-recently-used)", got)
	}
}

func TestEnvOrHomeEnvWins(t *testing.T) {
	t.Setenv("AZRL_TEST_DIR", "/opt/native")
	path, base, ok := provider.EnvOrHome("AZRL_TEST_DIR", ".config", "native")
	if !ok || path != "/opt/native" || base != "$AZRL_TEST_DIR" {
		t.Fatalf("EnvOrHome = %q, %q, %v; want /opt/native, $AZRL_TEST_DIR, true", path, base, ok)
	}
}

func TestEnvOrHomeFallsBackToHome(t *testing.T) {
	t.Setenv("AZRL_TEST_DIR", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, base, ok := provider.EnvOrHome("AZRL_TEST_DIR", ".config", "native")
	if !ok || path != filepath.Join(home, ".config", "native") || base != "~/.config/native" {
		t.Fatalf("EnvOrHome = %q, %q, %v; want %s, ~/.config/native, true", path, base, ok, filepath.Join(home, ".config", "native"))
	}
}
