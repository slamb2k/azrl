package azure

import (
	"github.com/slamb2k/azrl/internal/provider"
)

// Ambient returns the identity az itself would use right now, read from
// ${AZURE_CONFIG_DIR:-~/.azure}/azureProfile.json (default subscription's
// user, falling back to its tenant). Disk-only and best-effort: it never
// spawns az, and missing or unparseable state yields the zero value.
func (Provider) Ambient() (provider.Ambient, error) {
	dir, base, ok := provider.EnvOrHome("AZURE_CONFIG_DIR", ".azure")
	if !ok {
		return provider.Ambient{}, nil
	}
	source := "file:" + base + "/azureProfile.json"
	id := azureIdentity(dir)
	if id == "" {
		return provider.Ambient{}, nil
	}
	return provider.Ambient{Identity: id, Source: source}, nil
}
