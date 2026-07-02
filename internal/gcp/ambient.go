package gcp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/provider"
)

// Ambient returns the identity gcloud itself would use right now: the
// configuration named by CLOUDSDK_ACTIVE_CONFIG_NAME when set, else by
// ${CLOUDSDK_CONFIG:-~/.config/gcloud}/active_config, with the identity read
// from that configuration's [core] account. Disk-only and best-effort: it
// never spawns gcloud, and missing or unparseable state yields the zero value.
func (Provider) Ambient() (provider.Ambient, error) {
	dir := os.Getenv("CLOUDSDK_CONFIG")
	activeSource := "file:$CLOUDSDK_CONFIG/active_config"
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return provider.Ambient{}, nil
		}
		dir = filepath.Join(home, ".config", "gcloud")
		activeSource = "file:~/.config/gcloud/active_config"
	}
	name := os.Getenv("CLOUDSDK_ACTIVE_CONFIG_NAME")
	source := "env:CLOUDSDK_ACTIVE_CONFIG_NAME"
	if name == "" {
		b, err := os.ReadFile(filepath.Join(dir, "active_config"))
		if err != nil {
			return provider.Ambient{}, nil
		}
		name = strings.TrimSpace(string(b))
		source = activeSource
	}
	if name == "" {
		return provider.Ambient{}, nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "configurations", "config_"+name))
	if err != nil {
		return provider.Ambient{}, nil
	}
	account := iniValue(string(b), "core", "account")
	if account == "" {
		return provider.Ambient{}, nil
	}
	return provider.Ambient{Identity: account, Source: source}, nil
}
