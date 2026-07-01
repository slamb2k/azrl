package azure

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

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

// azureIdentity reads the default subscription's signed-in user (falling back to
// its tenant) from azureProfile.json; blank on any error.
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
			IsDefault bool   `json:"isDefault"`
			TenantID  string `json:"tenantId"`
		} `json:"subscriptions"`
	}
	if json.Unmarshal(b, &p) != nil {
		return ""
	}
	for _, s := range p.Subscriptions {
		if s.IsDefault {
			if s.User.Name != "" {
				return s.User.Name
			}
			return s.TenantID
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
