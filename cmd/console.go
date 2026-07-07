package cmd

import (
	"fmt"
	"net/url"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/gcp"
	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/profile"
)

// consoleURL builds the provider's web-console deep link for a profile from
// data already in its conf (plus, for gcp, the disk-only signed-in account),
// and returns the profile's mapped browser command ("" when unmapped).
func consoleURL(providerName, name string) (string, string, error) {
	switch providerName {
	case "azure":
		c, err := profile.LoadConf(name, config.ProfilesDir())
		if err != nil {
			return "", "", fmt.Errorf("azrl: unknown azure profile %q: %w", name, err)
		}
		tenant := c.Tenant
		if tenant == "" {
			tenant = c.TenantID
		}
		if tenant == "" {
			return "", "", fmt.Errorf("azrl: profile %q has no AZ_TENANT or AZ_TENANT_ID — nothing to open", name)
		}
		return "https://portal.azure.com/#@" + tenant, c.BrowserCmd, nil
	case "github":
		c, err := github.LoadConf(name, config.GithubProfilesDir())
		if err != nil {
			return "", "", fmt.Errorf("azrl: unknown github profile %q: %w", name, err)
		}
		if c.Host == "" {
			return "", "", fmt.Errorf("azrl: profile %q has no GH_HOST — nothing to open", name)
		}
		return "https://" + c.Host, c.BrowserCmd, nil
	case "aws":
		c, err := aws.LoadConf(name, config.AwsProfilesDir())
		if err != nil {
			return "", "", fmt.Errorf("azrl: unknown aws profile %q: %w", name, err)
		}
		if c.SSOStartURL == "" {
			return "", "", fmt.Errorf("azrl: profile %q has no AWS_SSO_START_URL — nothing to open", name)
		}
		return c.SSOStartURL, c.BrowserCmd, nil
	case "gcp":
		dir := config.GcpProfilesDir()
		c, err := gcp.LoadConf(name, dir)
		if err != nil {
			return "", "", fmt.Errorf("azrl: unknown gcp profile %q: %w", name, err)
		}
		if c.Project == "" {
			return "", "", fmt.Errorf("azrl: profile %q has no GCP_PROJECT — nothing to open", name)
		}
		u := "https://console.cloud.google.com/?project=" + url.QueryEscape(c.Project)
		if st, err := gcp.NewProvider().Status(name, dir); err == nil && st.Identity != "" {
			u += "&authuser=" + url.QueryEscape(st.Identity)
		}
		return u, c.BrowserCmd, nil
	default:
		return "", "", fmt.Errorf("azrl: unknown provider %q", providerName)
	}
}
