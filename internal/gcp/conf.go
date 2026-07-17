package gcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
)

// Conf holds a per-profile GCP configuration. ConfigName is the gcloud named
// configuration this profile drives (defaults to the profile slug); Project and
// Region are the bound project/compute-region (Project is the headline detail);
// ExpectAccount drives post-auth assertion; Label is an optional display name;
// Isolate pins this profile to its own CLOUDSDK_CONFIG dir rather than the shared
// ~/.config/gcloud.
type Conf struct {
	ConfigName    string
	Project       string
	Region        string
	ExpectAccount string
	Label         string
	BrowserCmd    string // optional local browser command overriding the global LOCAL_BROWSER_CMD
	BrowserLabel  string // human label for BrowserCmd, e.g. "Edge — Work" (display-only)
	Isolate       bool
}

// ResolvedConfigName returns the effective gcloud named configuration this
// profile drives: the explicit GCP_CONFIG_NAME when set, else the profile name.
// It is the single source of truth shared by SyncConfig, Login, ActiveAccount and
// the drift comparison so they all operate on the same configuration even when
// GCP_CONFIG_NAME differs from the profile name.
func (c Conf) ResolvedConfigName(name string) string {
	if c.ConfigName != "" {
		return c.ConfigName
	}
	return name
}

// LoadConf reads <confdir>/<name>.conf and requires GCP_PROJECT.
func LoadConf(name, confdir string) (Conf, error) {
	var c Conf
	path := filepath.Join(confdir, name+".conf")
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("gcp: missing config %s: %w", path, err)
	}
	defer f.Close()
	m, err := config.ParseKV(f)
	if err != nil {
		return c, err
	}
	c = Conf{
		ConfigName:    m["GCP_CONFIG_NAME"],
		Project:       m["GCP_PROJECT"],
		Region:        m["GCP_REGION"],
		ExpectAccount: m["GCP_EXPECT_ACCOUNT"],
		Label:         m["GCP_LABEL"],
		BrowserCmd:    m["GCP_BROWSER_CMD"],
		BrowserLabel:  m["GCP_BROWSER_LABEL"],
		Isolate:       strings.EqualFold(m["GCP_ISOLATE"], "true"),
	}
	if c.Project == "" {
		return c, fmt.Errorf("gcp: GCP_PROJECT not set in %s", path)
	}
	return c, nil
}

// Write atomically writes the conf in the canonical KEY=value format.
func (c Conf) Write(path string) error {
	isolate := "false"
	if c.Isolate {
		isolate = "true"
	}
	body := fmt.Sprintf("GCP_CONFIG_NAME=%s\nGCP_PROJECT=%s\nGCP_REGION=%s\nGCP_EXPECT_ACCOUNT=%s\nGCP_LABEL=%s\nGCP_ISOLATE=%s\nGCP_BROWSER_CMD=%s\nGCP_BROWSER_LABEL=%s\n",
		c.ConfigName, c.Project, c.Region, c.ExpectAccount, c.Label, isolate, c.BrowserCmd, c.BrowserLabel)
	return profile.WriteAtomic(path, body)
}

// SetIsolate persists the GCP_ISOLATE flag in profile name's conf, preserving
// every other key and its order (including LAST_USED/LAST_DIR).
func SetIsolate(confdir, name string, isolate bool) error {
	v := "false"
	if isolate {
		v = "true"
	}
	return scheme.SetKey(name, confdir, "GCP_ISOLATE", v)
}
