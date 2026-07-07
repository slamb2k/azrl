package aws

import (
	"os"
	"strings"

	"github.com/slamb2k/azrl/internal/provider"
)

// Ambient returns the identity aws itself would use right now: the AWS_PROFILE
// env var when set, else the [default] profile in
// ${AWS_CONFIG_FILE:-~/.aws/config}; the winning profile's stanza enriches the
// identity with its SSO account/role when resolvable, else the profile name
// stands alone. Disk-only and best-effort: it never spawns aws, and missing or
// unparseable state yields the zero value.
func (Provider) Ambient() (provider.Ambient, error) {
	path, base, ok := provider.EnvOrHome("AWS_CONFIG_FILE", ".aws", "config")
	if !ok {
		return provider.Ambient{}, nil
	}
	source := "file:" + base
	if p := os.Getenv("AWS_PROFILE"); p != "" {
		return provider.Ambient{
			Identity: ambientIdentity(configSection(path, "profile "+p), p),
			Source:   "env:AWS_PROFILE",
		}, nil
	}
	sec := configSection(path, "default")
	if sec == nil {
		return provider.Ambient{}, nil
	}
	return provider.Ambient{Identity: ambientIdentity(sec, "default"), Source: source}, nil
}

// ambientIdentity renders the identity behind a config-file stanza: the
// stanza's SSO account/role when resolvable, else the profile name alone.
func ambientIdentity(sec map[string]string, name string) string {
	if sec["sso_account_id"] == "" && sec["sso_role_name"] == "" {
		return name
	}
	return sec["sso_account_id"] + "/" + sec["sso_role_name"]
}

// configSection returns the key = value pairs under [section] in an aws config
// file; nil when the file or the section is missing.
func configSection(path, section string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out map[string]string
	cur := ""
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			cur = strings.TrimSpace(line[1 : len(line)-1])
			if cur == section && out == nil {
				out = map[string]string{}
			}
			continue
		}
		if cur != section {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

// CaptureDefaults returns the SSO fields behind the ambient aws identity —
// the stanza AWS_PROFILE names, else [default] — resolving an sso_session
// indirection when present. It backs flag-less `azrl aws capture`, so adopting
// the ambient identity records a profile that can actually sign in.
// Best-effort and disk-only: missing or unparseable state yields zero values.
func CaptureDefaults() Conf {
	path, _, ok := provider.EnvOrHome("AWS_CONFIG_FILE", ".aws", "config")
	if !ok {
		return Conf{}
	}
	section := "default"
	if p := os.Getenv("AWS_PROFILE"); p != "" {
		section = "profile " + p
	}
	sec := configSection(path, section)
	if sec == nil {
		return Conf{}
	}
	c := Conf{
		SSOStartURL: sec["sso_start_url"], SSORegion: sec["sso_region"],
		AccountID: sec["sso_account_id"], RoleName: sec["sso_role_name"],
	}
	if s := sec["sso_session"]; s != "" {
		if ses := configSection(path, "sso-session "+s); ses != nil {
			if c.SSOStartURL == "" {
				c.SSOStartURL = ses["sso_start_url"]
			}
			if c.SSORegion == "" {
				c.SSORegion = ses["sso_region"]
			}
		}
	}
	return c
}
