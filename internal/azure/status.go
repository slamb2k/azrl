package azure

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// Status returns a disk-only snapshot of profile name from its isolated
// AZURE_CONFIG_DIR (<confdir>/<name>) and conf file. It never spawns az.
func (Provider) Status(name, confdir string) (provider.Status, error) {
	isolated := filepath.Join(confdir, name)
	last, dir := profile.AzureScheme().LastTouch(name, confdir)
	last = provider.LatestMtime(last,
		filepath.Join(isolated, "msal_token_cache.json"),
		filepath.Join(isolated, "azureProfile.json"))
	return provider.Status{
		ProfileName: name,
		Identity:    azureIdentity(isolated),
		Directory:   dir,
		Expiry:      azureExpiry(isolated),
		Drifted:     provider.Drifted(profile.AzureScheme(), "AZURE_CONFIG_DIR", name, isolated),
		LastUsed:    last,
	}, nil
}

// azureIdentity reads the default subscription's signed-in user from
// azureProfile.json, qualified by its tenant — a bare UPN is not an Azure
// identity: the same user legitimately exists across tenants (B2B guests),
// so the tenant is part of who you are. Blank on any error.
func azureIdentity(confdir string) string {
	b, err := os.ReadFile(filepath.Join(confdir, "azureProfile.json"))
	if err != nil {
		return ""
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var p struct {
		Subscriptions []struct {
			User struct {
				Name string `json:"name"`
			} `json:"user"`
			IsDefault           bool   `json:"isDefault"`
			TenantID            string `json:"tenantId"`
			TenantDefaultDomain string `json:"tenantDefaultDomain"`
		} `json:"subscriptions"`
	}
	if json.Unmarshal(b, &p) != nil {
		return ""
	}
	for _, s := range p.Subscriptions {
		if s.IsDefault {
			return QualifiedIdentity(s.User.Name, s.TenantDefaultDomain, s.TenantID)
		}
	}
	return ""
}

// QualifiedIdentity composes "user · tenant" — the tenant being the default
// domain when known, else the domain a saved profile conf declares for the
// tenant GUID (guest/B2B tenants have no resolvable tenantDefaultDomain, but
// their conf carries AZ_TENANT + AZ_TENANT_ID), else a shortened GUID. Both
// the profile Status and ambient readers, and the TUI's az-account-show
// comparisons, use this one composition so identity matching stays
// tenant-aware and consistent.
func QualifiedIdentity(user, domain, tenantID string) string {
	tenant := domain
	if tenant == "" {
		tenant = tenantDomainFromConfs(tenantID)
	}
	if tenant == "" {
		tenant = tenantID
		if len(tenant) > 8 {
			tenant = tenant[:8]
		}
	}
	switch {
	case user == "" && tenant == "":
		return ""
	case user == "":
		return tenant
	case tenant == "":
		return user
	}
	return user + " · " + tenant
}

// tenantDomainFromConfs resolves a tenant GUID to the human domain a saved
// profile declares for it (AZ_TENANT, matched by AZ_TENANT_ID). Disk-only;
// "" when no conf knows the tenant.
func tenantDomainFromConfs(tenantID string) string {
	if tenantID == "" {
		return ""
	}
	listed, err := profile.List(config.ProfilesDir())
	if err != nil {
		return ""
	}
	for _, l := range listed {
		if c, err := profile.LoadConf(l.Name, config.ProfilesDir()); err == nil {
			if c.TenantID == tenantID && c.Tenant != "" && c.Tenant != c.TenantID {
				return c.Tenant
			}
		}
	}
	return ""
}

// azureExpiry reads the latest access-token expiry from the MSAL cache; nil on
// any error or when no parseable expires_on is present.
func azureExpiry(confdir string) *time.Time {
	b, err := os.ReadFile(filepath.Join(confdir, "msal_token_cache.json"))
	if err != nil {
		return nil
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var c struct {
		AccessToken map[string]struct {
			ExpiresOn string `json:"expires_on"`
		} `json:"AccessToken"`
	}
	if json.Unmarshal(b, &c) != nil {
		return nil
	}
	var max int64
	for _, at := range c.AccessToken {
		n, err := strconv.ParseInt(at.ExpiresOn, 10, 64)
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	if max == 0 {
		return nil
	}
	t := time.Unix(max, 0)
	return &t
}
