package provider

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
