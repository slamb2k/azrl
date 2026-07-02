package provider

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvOrHome resolves the location a native CLI reads: envVar's value when
// set, else rel joined under the user's home directory. base is the matching
// display form ("$ENVVAR" or "~/rel/…") for Ambient Source labels; ok is
// false only when envVar is unset and the home directory is unknown.
func EnvOrHome(envVar string, rel ...string) (path, base string, ok bool) {
	if v := os.Getenv(envVar); v != "" {
		return v, "$" + envVar, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", false
	}
	return filepath.Join(append([]string{home}, rel...)...), "~/" + strings.Join(rel, "/"), true
}

// MatchProfile reverse-maps an ambient identity to the managed profile whose
// disk-only Status identity matches, given the provider's precomputed profile
// statuses. When several saved profiles share the identity the most-recently-
// used wins; "" means the identity is unmanaged.
func MatchProfile(statuses []Status, identity string) string {
	if identity == "" {
		return ""
	}
	var best Status
	name := ""
	for _, st := range statuses {
		if st.Identity != identity {
			continue
		}
		if name == "" || st.LastUsed.After(best.LastUsed) {
			name, best = st.ProfileName, st
		}
	}
	return name
}
