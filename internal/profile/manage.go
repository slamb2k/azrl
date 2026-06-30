package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Use links pwd to an existing profile by writing pwd/.azprofile, after
// verifying <confdir>/<name>.conf exists.
func Use(name, confdir, pwd string) error {
	conf := filepath.Join(confdir, name+".conf")
	if _, err := os.Stat(conf); err != nil {
		return fmt.Errorf("azrl: no such profile %q (missing %s)", name, conf)
	}
	return os.WriteFile(filepath.Join(pwd, ".azprofile"), []byte(name+"\n"), 0o644)
}

// RemoveTargets returns the existing paths that Remove would delete: the conf,
// the AZURE_CONFIG_DIR, and pwd/.azprofile only when it names this profile.
func RemoveTargets(name, confdir, pwd string) []string {
	var targets []string
	conf := filepath.Join(confdir, name+".conf")
	if _, err := os.Stat(conf); err == nil {
		targets = append(targets, conf)
	}
	dir := filepath.Join(confdir, name)
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		targets = append(targets, dir)
	}
	az := filepath.Join(pwd, ".azprofile")
	if b, err := os.ReadFile(az); err == nil && strings.TrimSpace(string(b)) == name {
		targets = append(targets, az)
	}
	return targets
}

// Remove deletes the RemoveTargets and returns the list it removed.
func Remove(name, confdir, pwd string) ([]string, error) {
	targets := RemoveTargets(name, confdir, pwd)
	for _, t := range targets {
		if err := os.RemoveAll(t); err != nil {
			return targets, err
		}
	}
	return targets, nil
}
