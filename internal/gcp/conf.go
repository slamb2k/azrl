package gcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/config"
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
		Isolate:       strings.EqualFold(m["GCP_ISOLATE"], "true"),
	}
	if c.Project == "" {
		return c, fmt.Errorf("gcp: GCP_PROJECT not set in %s", path)
	}
	return c, nil
}

// Write atomically writes the conf in the canonical KEY=value format.
func (c Conf) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	isolate := "false"
	if c.Isolate {
		isolate = "true"
	}
	body := fmt.Sprintf("GCP_CONFIG_NAME=%s\nGCP_PROJECT=%s\nGCP_REGION=%s\nGCP_EXPECT_ACCOUNT=%s\nGCP_LABEL=%s\nGCP_ISOLATE=%s\n",
		c.ConfigName, c.Project, c.Region, c.ExpectAccount, c.Label, isolate)
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

// SetIsolate persists the GCP_ISOLATE flag in profile name's conf, preserving
// every other key and its order (including LAST_USED/LAST_DIR).
func SetIsolate(confdir, name string, isolate bool) error {
	v := "false"
	if isolate {
		v = "true"
	}
	return setConfKey(filepath.Join(confdir, name+".conf"), "GCP_ISOLATE", v)
}

// setConfKey updates or appends a single KEY=value line in an existing conf,
// preserving every other key and its order (so LAST_USED/LAST_DIR written by the
// scheme survive). It creates the file when absent.
func setConfKey(path, key, value string) error {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var out []string
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		k, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(k) == key {
			out = append(out, key+"="+value)
			found = true
			continue
		}
		if line != "" {
			out = append(out, line)
		}
	}
	if !found {
		out = append(out, key+"="+value)
	}
	body := strings.Join(out, "\n") + "\n"
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
