package azure

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func runAz(args ...string) ([]byte, error) {
	return exec.Command("az", args...).Output()
}

// CleanSlate reaps orphaned az-login processes (warning about live ones),
// logs out, clears accounts, and removes the scoped MSAL caches in cfgDir.
// The az errors are intentionally ignored (a fresh box has nothing to
// clear); only filesystem errors surface — and those are ignored too since
// the files may legitimately be absent.
func CleanSlate(cfgDir string, out io.Writer) error {
	SweepOrphanedLogins(out)
	_ = exec.Command("az", "logout").Run()
	_ = exec.Command("az", "account", "clear").Run()
	os.Remove(filepath.Join(cfgDir, "msal_token_cache.json"))
	os.Remove(filepath.Join(cfgDir, "service_principal_entries.json"))
	return nil
}

// AccountShow returns `az account show -o json`.
func AccountShow() ([]byte, error) {
	return runAz("account", "show", "-o", "json")
}

// AccountShowIn reports the signed-in account for a specific AZURE_CONFIG_DIR
// (the profile's isolated token dir), rather than the ambient ~/.azure session.
// An empty configDir falls back to the inherited environment.
func AccountShowIn(configDir string) ([]byte, error) {
	cmd := exec.Command("az", "account", "show", "-o", "json")
	if configDir != "" {
		cmd.Env = append(os.Environ(), "AZURE_CONFIG_DIR="+configDir)
	}
	return cmd.Output()
}

// SetSubscription selects the given subscription.
func SetSubscription(sub string) error {
	return exec.Command("az", "account", "set", "--subscription", sub).Run()
}

// Domains returns the Graph /v1.0/domains JSON, or {} on error.
func Domains() []byte {
	out, err := runAz("rest", "--url", "https://graph.microsoft.com/v1.0/domains", "-o", "json")
	if err != nil {
		return []byte("{}")
	}
	return out
}
