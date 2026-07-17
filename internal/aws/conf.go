package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
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
	BrowserCmd    string // optional local browser command overriding the global LOCAL_BROWSER_CMD
	BrowserLabel  string // human label for BrowserCmd, e.g. "Edge — Work" (display-only)
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
		BrowserCmd:    m["AWS_BROWSER_CMD"],
		BrowserLabel:  m["AWS_BROWSER_LABEL"],
		Isolate:       strings.EqualFold(m["AWS_ISOLATE"], "true"),
	}
	if c.SSOStartURL == "" {
		return c, fmt.Errorf("aws: AWS_SSO_START_URL not set in %s", path)
	}
	return c, nil
}

// Write atomically writes the conf in the canonical KEY=value format.
func (c Conf) Write(path string) error {
	isolate := "false"
	if c.Isolate {
		isolate = "true"
	}
	body := fmt.Sprintf("AWS_SSO_START_URL=%s\nAWS_SSO_REGION=%s\nAWS_ACCOUNT_ID=%s\nAWS_ROLE_NAME=%s\nAWS_EXPECT_ACCOUNT=%s\nAWS_EXPECT_ARN=%s\nAWS_LABEL=%s\nAWS_ISOLATE=%s\nAWS_BROWSER_CMD=%s\nAWS_BROWSER_LABEL=%s\n",
		c.SSOStartURL, c.SSORegion, c.AccountID, c.RoleName, c.ExpectAccount, c.ExpectARN, c.Label, isolate, c.BrowserCmd, c.BrowserLabel)
	return profile.WriteAtomic(path, body)
}

// SetIsolate persists the AWS_ISOLATE flag in profile name's conf, preserving
// every other key and its order (including LAST_USED/LAST_DIR).
func SetIsolate(confdir, name string, isolate bool) error {
	v := "false"
	if isolate {
		v = "true"
	}
	return scheme.SetKey(name, confdir, "AWS_ISOLATE", v)
}
