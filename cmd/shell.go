package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/gcp"
	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/profile"
)

// shellEnv builds the env pairs a subshell needs to act as the profile — the
// same values the .envrc writers emit — plus the AZRL_PROFILE marker and the
// profile's mapped browser command (so git push / az login inside the
// subshell route to the right browser profile).
func shellEnv(providerName, name string) ([]string, error) {
	var env []string
	browser := ""
	switch providerName {
	case "azure":
		dir := config.ProfilesDir()
		c, err := profile.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown azure profile %q: %w", name, err)
		}
		env = append(env, "AZURE_CONFIG_DIR="+filepath.Join(dir, name))
		browser = c.BrowserCmd
	case "github":
		dir := config.GithubProfilesDir()
		c, err := github.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown github profile %q: %w", name, err)
		}
		env = append(env, "GH_CONFIG_DIR="+github.ConfigDir(dir, name))
		browser = c.BrowserCmd
	case "aws":
		dir := config.AwsProfilesDir()
		c, err := aws.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown aws profile %q: %w", name, err)
		}
		if c.Isolate {
			env = append(env,
				"AWS_CONFIG_FILE="+filepath.Join(dir, name, "config"),
				"AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(dir, name, "credentials"))
		} else {
			env = append(env, "AWS_PROFILE="+name)
		}
		browser = c.BrowserCmd
	case "gcp":
		dir := config.GcpProfilesDir()
		c, err := gcp.LoadConf(name, dir)
		if err != nil {
			return nil, fmt.Errorf("azrl: unknown gcp profile %q: %w", name, err)
		}
		if c.Isolate {
			env = append(env, "CLOUDSDK_CONFIG="+filepath.Join(dir, name))
		}
		env = append(env, "CLOUDSDK_ACTIVE_CONFIG_NAME="+c.ResolvedConfigName(name))
		browser = c.BrowserCmd
	default:
		return nil, fmt.Errorf("azrl: unknown provider %q", providerName)
	}
	if browser != "" {
		env = append(env, "AZRL_BROWSER_CMD="+browser)
	}
	env = append(env, "AZRL_PROFILE="+providerName+":"+name)
	return env, nil
}
