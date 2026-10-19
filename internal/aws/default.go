package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/config"
)

// NativeConfigPath is the config file the plain aws CLI reads by default:
// $AWS_CONFIG_FILE when set — unless it points into azrl's own profiles dir
// (an isolated profile's file, exported by direnv or `azrl shell`), which is
// not the native store — else ~/.aws/config.
func NativeConfigPath() string {
	if p := os.Getenv("AWS_CONFIG_FILE"); p != "" && !config.Within(config.AwsProfilesDir(), p) {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".aws", "config")
}

// ssoOwnedKeys are the [default] keys DefaultStanza writes; their old values
// are replaced.
var ssoOwnedKeys = map[string]bool{
	"sso_start_url": true, "sso_region": true, "sso_account_id": true, "sso_role_name": true,
}

// conflictingKeys select a different credential source than the inline SSO
// stanza and would win over it, so they are dropped from [default].
var conflictingKeys = map[string]bool{
	"sso_session": true, "role_arn": true, "source_profile": true, "credential_source": true,
	"credential_process": true, "web_identity_token_file": true, "role_session_name": true,
	"aws_access_key_id": true, "aws_secret_access_key": true, "aws_session_token": true,
}

// RewriteDefaultSection points the [default] section of an aws config body at
// the SSO keys in stanza (which must include its own "[default]" header),
// appending the section when absent. Within [default] the user's other
// settings survive (output, cli_pager, nested s3 blocks, comments, and their
// own region); only the SSO keys are replaced and keys that would select a
// competing credential source are dropped. Every other section is untouched.
func RewriteDefaultSection(existing, stanza string) string {
	var want []string // stanza body lines, in order
	wantKeys := map[string]bool{}
	for _, l := range strings.Split(stanza, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "[") {
			continue
		}
		want = append(want, t)
		wantKeys[iniKey(t)] = true
	}
	lines := strings.Split(strings.TrimRight(existing, "\n"), "\n")
	if existing == "" {
		lines = nil
	}
	var out, kept []string
	found, inDefault, dropping := false, false, false
	flush := func() {
		if !inDefault {
			return
		}
		body := append([]string{}, kept...)
		for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
			body = body[:len(body)-1]
		}
		for _, w := range want {
			if k := iniKey(w); k == "region" && hasKey(body, "region") {
				continue // the user's own region choice wins
			}
			body = append(body, w)
		}
		// Keep a blank separator before the next section if one followed.
		for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
			body = body[:len(body)-1]
		}
		out = append(out, body...)
		out = append(out, "")
		kept = nil
	}
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") {
			flush()
			inDefault = t == "[default]"
			dropping = false
			if inDefault {
				found = true
			}
			out = append(out, l)
			continue
		}
		if !inDefault {
			out = append(out, l)
			continue
		}
		indented := l != "" && (l[0] == ' ' || l[0] == '\t')
		if indented && t != "" {
			if !dropping {
				kept = append(kept, l)
			}
			continue // continuation of a nested key (e.g. s3 = …)
		}
		k := iniKey(t)
		dropping = k != "" && (ssoOwnedKeys[k] || conflictingKeys[k] || (wantKeys[k] && k != "region"))
		if !dropping {
			kept = append(kept, l)
		}
	}
	flush()
	body := strings.TrimRight(strings.Join(out, "\n"), "\n")
	if !found {
		if body != "" {
			body += "\n\n"
		}
		body += strings.TrimRight(stanza, "\n")
	}
	return body + "\n"
}

// iniKey returns the lower-cased key of a "key = value" line ("" for blank
// lines and comments).
func iniKey(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || t[0] == '#' || t[0] == ';' {
		return ""
	}
	k, _, ok := strings.Cut(t, "=")
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(k))
}

func hasKey(lines []string, key string) bool {
	for _, l := range lines {
		if l != "" && l[0] != ' ' && l[0] != '\t' && iniKey(l) == key {
			return true
		}
	}
	return false
}

// DefaultStanza renders the [default] profile for c using the legacy inline
// SSO keys — no sso-session indirection, so it cannot collide with named
// sessions azrl or the user already maintain. region is only a fallback: an
// existing [default] region is kept.
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
	// Write through a symlinked config (dotfile managers) to its target, so
	// the link survives instead of being replaced by a regular file.
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	mode := os.FileMode(0o600)
	b, err := os.ReadFile(target)
	switch {
	case err == nil:
		if fi, serr := os.Stat(target); serr == nil {
			mode = fi.Mode().Perm()
		}
		// One-step undo: the config exactly as it was before this call.
		if werr := os.WriteFile(target+".azrl.bak", b, mode); werr != nil {
			return "", fmt.Errorf("aws: backing up %s: %w", target, werr)
		}
	case !os.IsNotExist(err):
		return "", err
	}
	body := RewriteDefaultSection(string(b), DefaultStanza(c))
	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".*")
	if err != nil {
		return "", err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
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
	return path, os.Rename(tmp.Name(), target)
}
