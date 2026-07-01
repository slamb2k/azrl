package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/config"
)

// Conf holds a per-profile AWS configuration. SSOStartURL is the account's IAM
// Identity Center portal URL (the headline detail); SSORegion is the region the
// portal lives in; AccountID and RoleName pick the permission set; ExpectAccount
// and ExpectARN drive post-auth assertion; Label is an optional display name;
// Isolate pins this profile to its own AWS_CONFIG_FILE/AWS_SHARED_CREDENTIALS_FILE
// rather than the shared ~/.aws files.
type Conf struct {
	SSOStartURL   string
	SSORegion     string
	AccountID     string
	RoleName      string
	ExpectAccount string
	ExpectARN     string
	Label         string
	Isolate       bool
}

// LoadConf reads <confdir>/<name>.conf and requires AWS_SSO_START_URL.
func LoadConf(name, confdir string) (Conf, error) {
	var c Conf
	path := filepath.Join(confdir, name+".conf")
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("aws: missing config %s: %w", path, err)
	}
	defer f.Close()
	m, err := config.ParseKV(f)
	if err != nil {
		return c, err
	}
	c = Conf{
		SSOStartURL:   m["AWS_SSO_START_URL"],
		SSORegion:     m["AWS_SSO_REGION"],
		AccountID:     m["AWS_ACCOUNT_ID"],
		RoleName:      m["AWS_ROLE_NAME"],
		ExpectAccount: m["AWS_EXPECT_ACCOUNT"],
		ExpectARN:     m["AWS_EXPECT_ARN"],
		Label:         m["AWS_LABEL"],
		Isolate:       strings.EqualFold(m["AWS_ISOLATE"], "true"),
	}
	if c.SSOStartURL == "" {
		return c, fmt.Errorf("aws: AWS_SSO_START_URL not set in %s", path)
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
	body := fmt.Sprintf("AWS_SSO_START_URL=%s\nAWS_SSO_REGION=%s\nAWS_ACCOUNT_ID=%s\nAWS_ROLE_NAME=%s\nAWS_EXPECT_ACCOUNT=%s\nAWS_EXPECT_ARN=%s\nAWS_LABEL=%s\nAWS_ISOLATE=%s\n",
		c.SSOStartURL, c.SSORegion, c.AccountID, c.RoleName, c.ExpectAccount, c.ExpectARN, c.Label, isolate)
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

// SetIsolate persists the AWS_ISOLATE flag in profile name's conf, preserving
// every other key and its order (including LAST_USED/LAST_DIR).
func SetIsolate(confdir, name string, isolate bool) error {
	v := "false"
	if isolate {
		v = "true"
	}
	return setConfKey(filepath.Join(confdir, name+".conf"), "AWS_ISOLATE", v)
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
