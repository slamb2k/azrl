package azure

import (
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/provider"
)

// Ambient returns the identity az itself would use right now, read from
// ${AZURE_CONFIG_DIR:-~/.azure}/azureProfile.json (default subscription's
// user, falling back to its tenant). Disk-only and best-effort: it never
// spawns az, and missing or unparseable state yields the zero value.
func (Provider) Ambient() (provider.Ambient, error) {
	dir := os.Getenv("AZURE_CONFIG_DIR")
	source := "file:$AZURE_CONFIG_DIR/azureProfile.json"
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return provider.Ambient{}, nil
		}
		dir = filepath.Join(home, ".azure")
		source = "file:~/.azure/azureProfile.json"
	}
	id := azureIdentity(dir)
	if id == "" {
		return provider.Ambient{}, nil
	}
	return provider.Ambient{Identity: id, Source: source}, nil
}
