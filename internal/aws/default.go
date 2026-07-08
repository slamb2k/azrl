package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NativeConfigPath is the config file the plain aws CLI reads:
// $AWS_CONFIG_FILE when set, else ~/.aws/config.
func NativeConfigPath() string {
	if p := os.Getenv("AWS_CONFIG_FILE"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "config")
}

// RewriteDefaultSection replaces the [default] section of an aws config file
// body with stanza (which must include its own "[default]" header), appending
// it when no such section exists. Every other section is preserved verbatim.
func RewriteDefaultSection(existing, stanza string) string {
	lines := strings.Split(existing, "\n")
	var out []string
	inDefault := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			inDefault = t == "[default]"
			if inDefault {
				continue
			}
		}
		if !inDefault {
			out = append(out, l)
		}
	}
	body := strings.TrimRight(strings.Join(out, "\n"), "\n")
	if body != "" {
		body += "\n\n"
	}
	return body + strings.TrimRight(stanza, "\n") + "\n"
}

// DefaultStanza renders the [default] profile for c using the legacy inline
// SSO keys — no sso-session indirection, so it cannot collide with named
// sessions azrl or the user already maintain.
func DefaultStanza(c Conf) string {
	return fmt.Sprintf("[default]\nsso_start_url = %s\nsso_region = %s\nsso_account_id = %s\nsso_role_name = %s\nregion = %s\n",
		c.SSOStartURL, c.SSORegion, c.AccountID, c.RoleName, c.SSORegion)
}

// SetDefaultProfile points the native aws CLI's [default] profile at c's SSO
// coordinates, preserving every other section of the config file. This is the
// one deliberate native-state mutation in azrl — the user-invoked `default`
// verb (see docs/ambient-identity-model.md, 2026-07-08 amendment).
func SetDefaultProfile(c Conf) (string, error) {
	if c.SSOStartURL == "" || c.SSORegion == "" || c.AccountID == "" || c.RoleName == "" {
		return "", fmt.Errorf("aws: profile is missing SSO fields (start URL, region, account, role) — capture or edit the conf first")
	}
	path := NativeConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	body := RewriteDefaultSection(string(b), DefaultStanza(c))
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return "", err
	}
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return path, os.Rename(tmp.Name(), path)
}
