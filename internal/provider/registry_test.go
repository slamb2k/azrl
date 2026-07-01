package provider_test

import (
	"testing"
	"time"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// stubProvider is a minimal Provider used to exercise the registry in isolation.
type stubProvider struct{ name string }

func (s stubProvider) Name() string                                       { return s.name }
func (s stubProvider) Title() string                                      { return s.name }
func (s stubProvider) ProfilesDir() string                                { return "" }
func (s stubProvider) Scheme() profile.Scheme                             { return profile.Scheme{} }
func (s stubProvider) ListProfiles(string) ([]profile.Listed, error)      { return nil, nil }
func (s stubProvider) Resolve(arg, dir string) (string, error)            { return arg, nil }
func (s stubProvider) Use(name, confdir, pwd string) error                { return nil }
func (s stubProvider) Remove(name, confdir, pwd string) ([]string, error) { return nil, nil }
func (s stubProvider) SetLabel(name, confdir, label string) error         { return nil }
func (s stubProvider) Status(name, confdir string) (provider.Status, error) {
	return provider.Status{ProfileName: name, LastUsed: time.Now()}, nil
}

func TestRegistrySortsByNameAndDedupes(t *testing.T) {
	provider.Register(stubProvider{name: "zeta"})
	provider.Register(stubProvider{name: "alpha"})
	provider.Register(stubProvider{name: "zeta"}) // dedupe by name

	all := provider.All()
	var names []string
	seen := map[string]int{}
	for _, p := range all {
		names = append(names, p.Name())
		seen[p.Name()]++
	}
	if seen["zeta"] != 1 || seen["alpha"] != 1 {
		t.Fatalf("dedupe failed: %v", names)
	}
	// Assert alpha precedes zeta (sorted order).
	var ai, zi = -1, -1
	for i, n := range names {
		if n == "alpha" {
			ai = i
		}
		if n == "zeta" {
			zi = i
		}
	}
	if ai < 0 || zi < 0 || ai > zi {
		t.Fatalf("not sorted by name: %v", names)
	}
}
