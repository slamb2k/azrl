package profile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/config"
)

// AccountJSON mirrors the fields azrl reads from `az account show -o json`.
type AccountJSON struct {
	TenantID            string `json:"tenantId"`
	TenantDefaultDomain string `json:"tenantDefaultDomain"`
	ID                  string `json:"id"`
	Name                string `json:"name"`
	User                struct {
		Name string `json:"name"`
	} `json:"user"`
}

// Domain is one entry from the Graph /domains response.
type Domain struct {
	ID        string `json:"id"`
	IsDefault bool   `json:"isDefault"`
}

// DomainsJSON is the Graph /v1.0/domains response.
type DomainsJSON struct {
	Value []Domain `json:"value"`
}

// Conf holds a per-profile configuration. Label is an optional human-facing
// display name; the profile's identity remains its slug (the .conf filename),
// so the label can be changed at will without moving files or breaking any
// .azprofile pointers.
type Conf struct {
	Tenant     string
	TenantID   string
	DefaultSub string
	ExpectUser string
	Label      string
}

// azureScheme parameterizes the shared profile mechanics for Azure: .azprofile
// pins, AZ_TENANT as the headline detail, AZ_LABEL as the display label, and the
// reserved global-conf basename azrl.
var azureScheme = Scheme{
	Pointer:   ".azprofile",
	Reserved:  "azrl",
	DetailKey: "AZ_TENANT",
	LabelKey:  "AZ_LABEL",
	Prefix:    "azrl",
}

// AzureScheme returns the Scheme carrying azrl's Azure profile mechanics, for
// providers and callers that drive the generic Scheme methods directly.
func AzureScheme() Scheme { return azureScheme }

// Resolve returns arg when non-empty, otherwise the trimmed contents of the
// nearest .azprofile found walking up from dir.
func Resolve(arg, dir string) (string, error) {
	return azureScheme.Resolve(arg, dir)
}

// LocateAzprofile walks up from dir to the nearest directory that holds an
// .azprofile, returning that directory. ok is false when none is found. This is
// the directory an .envrc must live in, since its `cat .azprofile` is resolved
// relative to the .envrc's own location.
func LocateAzprofile(dir string) (string, bool) {
	return azureScheme.Locate(dir)
}

// LoadConf reads <confdir>/<name>.conf and requires AZ_TENANT.
func LoadConf(name, confdir string) (Conf, error) {
	var c Conf
	path := filepath.Join(confdir, name+".conf")
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("azrl: missing config %s: %w", path, err)
	}
	defer f.Close()
	m, err := config.ParseKV(f)
	if err != nil {
		return c, err
	}
	c = Conf{Tenant: m["AZ_TENANT"], TenantID: m["AZ_TENANT_ID"], DefaultSub: m["AZ_DEFAULT_SUB"], ExpectUser: m["AZ_EXPECT_USER"], Label: m["AZ_LABEL"]}
	if c.Tenant == "" {
		return c, fmt.Errorf("azrl: AZ_TENANT not set in %s", path)
	}
	return c, nil
}

// BuildConf derives a Conf from an account and the domains list. Tenant prefers
// the verified default domain, falling back to the tenant GUID (guest/B2B).
func BuildConf(acct AccountJSON, doms DomainsJSON) Conf {
	tenant := acct.TenantID
	for _, d := range doms.Value {
		if d.IsDefault {
			tenant = d.ID
			break
		}
	}
	return Conf{Tenant: tenant, TenantID: acct.TenantID, DefaultSub: acct.ID, ExpectUser: acct.User.Name}
}

// Write atomically writes the conf in the canonical KEY=value format.
func (c Conf) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("AZ_TENANT=%s\nAZ_TENANT_ID=%s\nAZ_DEFAULT_SUB=%s\nAZ_EXPECT_USER=%s\nAZ_LABEL=%s\n",
		c.Tenant, c.TenantID, c.DefaultSub, c.ExpectUser, c.Label)
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
	return os.Rename(tmp.Name(), path)
}

// SetLabel updates only the AZ_LABEL of profile name, preserving its other
// fields. An empty label reverts the display name to the slug.
func SetLabel(name, confdir, label string) error {
	return azureScheme.SetLabel(name, confdir, label)
}

// Listed is a profile slug with its headline detail (tenant/host) and optional
// display label.
type Listed struct {
	Name   string
	Detail string
	Label  string
}

// Display returns the label when set, otherwise the slug.
func (l Listed) Display() string {
	if l.Label != "" {
		return l.Label
	}
	return l.Name
}

// List returns every <name>.conf in confdir (except azrl.conf) with its tenant,
// sorted by name.
func List(confdir string) ([]Listed, error) {
	return azureScheme.List(confdir)
}
