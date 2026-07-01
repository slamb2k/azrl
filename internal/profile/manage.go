package profile

// Use links pwd to an existing profile by writing pwd/.azprofile, after
// verifying <confdir>/<name>.conf exists.
func Use(name, confdir, pwd string) error {
	return azureScheme.Use(name, confdir, pwd)
}

// RemoveTargets returns the existing paths that Remove would delete: the conf,
// the AZURE_CONFIG_DIR, and pwd/.azprofile only when it names this profile.
func RemoveTargets(name, confdir, pwd string) []string {
	return azureScheme.RemoveTargets(name, confdir, pwd)
}

// Remove deletes the RemoveTargets and returns the list it removed.
func Remove(name, confdir, pwd string) ([]string, error) {
	return azureScheme.Remove(name, confdir, pwd)
}
