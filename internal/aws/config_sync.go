package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// configPath returns the ~/.aws/config path SyncConfig writes, or the isolated
// <confdir>/<name>/config when the profile is isolated.
func configPath(name, confdir string, isolate bool) string {
	if isolate {
		return filepath.Join(confdir, name, "config")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "config")
}

// SyncConfig idempotently ensures the [sso-session <name>] and [profile <name>]
// stanzas exist in the target aws config file. It never overwrites an existing
// [profile <name>] block, so hand-tuned settings survive; only missing stanzas
// are appended.
func SyncConfig(name, confdir string, c Conf) error {
	path := configPath(name, confdir, c.Isolate)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	existing := string(b)

	var add strings.Builder
	if !strings.Contains(existing, fmt.Sprintf("[sso-session %s]", name)) {
		fmt.Fprintf(&add, "[sso-session %s]\nsso_start_url = %s\nsso_region = %s\nsso_registration_scopes = sso:account:access\n\n",
			name, c.SSOStartURL, c.SSORegion)
	}
	if !strings.Contains(existing, fmt.Sprintf("[profile %s]", name)) {
		fmt.Fprintf(&add, "[profile %s]\nsso_session = %s\nsso_account_id = %s\nsso_role_name = %s\nregion = %s\n\n",
			name, name, c.AccountID, c.RoleName, c.SSORegion)
	}
	if add.Len() == 0 {
		return nil
	}

	body := existing
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += add.String()
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	// Preserve an existing config's permissions across the temp+rename (a fresh
	// CreateTemp is 0600); a brand-new file keeps that sane default.
	if fi, serr := os.Stat(path); serr == nil {
		if cerr := os.Chmod(tmp.Name(), fi.Mode().Perm()); cerr != nil {
			os.Remove(tmp.Name())
			return cerr
		}
	}
	return os.Rename(tmp.Name(), path)
}
