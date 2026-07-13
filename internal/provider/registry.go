package provider

import "sort"

var registry []Provider

// Register adds p to the central provider list, replacing any existing provider
// with the same Name (dedupe). Providers self-register from their package init.
func Register(p Provider) {
	for i, existing := range registry {
		if existing.Name() == p.Name() {
			registry[i] = p
			return
		}
	}
	registry = append(registry, p)
}

// All returns the registered providers sorted by Name for deterministic order.
func All() []Provider {
	out := make([]Provider, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// ProviderStatus groups one provider with the disk-only snapshots of its
// profiles, in provider ListProfiles order (the caller sorts if it needs to).
type ProviderStatus struct {
	Name     string
	Title    string
	Statuses []Status
}

// Collect gathers each provider's profiles into disk-only Status snapshots for
// the cross-provider dashboard and `azrl profiles`. A provider whose ListProfiles
// fails is skipped; a per-profile Status error is surfaced on that profile's own
// snapshot rather than aborting the whole scan. It never makes a network call.
func Collect(provs []Provider) []ProviderStatus {
	var out []ProviderStatus
	for _, p := range provs {
		confdir := p.ProfilesDir()
		listed, err := p.ListProfiles(confdir)
		if err != nil {
			continue
		}
		ps := ProviderStatus{Name: p.Name(), Title: p.Title()}
		for _, l := range listed {
			st, err := p.Status(l.Name, confdir)
			if err != nil {
				st = Status{ProfileName: l.Name, Identity: "⚠ error"}
			}
			ps.Statuses = append(ps.Statuses, st)
		}
		out = append(out, ps)
	}
	return out
}
