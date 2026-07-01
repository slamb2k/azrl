package aws

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/slamb2k/azrl/internal/bridge"
	"github.com/slamb2k/azrl/internal/browsercapture"
	"github.com/slamb2k/azrl/internal/config"
)

// Login runs `aws sso login --profile <name>` in the background with the BROWSER
// shim, captures the authorize URL, and bridges its loopback callback port to the
// local browser over SSH (path B reverse tunnel / path A paste). With device set
// it passes --use-device-code (device flow has no loopback port to tunnel). With
// isolate set it scopes AWS_CONFIG_FILE/AWS_SHARED_CREDENTIALS_FILE to the
// profile's own files.
func Login(dir, name string, isolate, device bool) error {
	g, err := config.LoadGlobal(config.ProfilesDir())
	if err != nil {
		return err
	}

	cap, err := os.CreateTemp("", "azrl-aws-cap-*")
	if err != nil {
		return err
	}
	cap.Close()
	capfile := cap.Name()
	defer os.Remove(capfile)

	args := []string{"sso", "login", "--profile", name}
	if device {
		args = append(args, "--use-device-code")
	}
	cmd := exec.Command("aws", args...)
	env := append(os.Environ(),
		"AZRL_CAPFILE="+capfile,
		"BROWSER="+browsercapture.CaptureCommand()+" %s",
	)
	if isolate {
		env = append(env,
			"AWS_CONFIG_FILE="+filepath.Join(dir, name, "config"),
			"AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(dir, name, "credentials"),
		)
	}
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return err
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()

	url := ""
	for i := 0; i < 200; i++ { // 200 × 0.1s = 20s
		if b, rerr := os.ReadFile(capfile); rerr == nil && len(b) > 0 {
			url = string(b)
			break
		}
		select {
		case err := <-waitErr:
			if err != nil {
				return fmt.Errorf("aws: aws sso login exited before producing an auth URL: %w", err)
			}
			return fmt.Errorf("aws: aws sso login exited before producing an auth URL")
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	if url == "" {
		_ = cmd.Process.Kill()
		<-waitErr
		return fmt.Errorf("aws: timed out waiting for auth URL")
	}

	var tunnel *exec.Cmd
	if port := browsercapture.ParseCallbackPort(url); port != "" {
		t, paste, berr := bridge.Bridge(port, url, g, false)
		if berr != nil {
			_ = cmd.Process.Kill()
			<-waitErr
			return berr
		}
		tunnel = t
		if tunnel != nil {
			defer func() { _ = tunnel.Process.Kill() }()
			fmt.Fprintf(os.Stdout, "aws: browser opened on %s (zero-paste path B)\n", g.LocalHost)
		} else {
			fmt.Fprintf(os.Stdout, "aws: paste this on your LOCAL machine:\n\n%s\n\n", paste)
		}
	} else {
		fmt.Fprintf(os.Stdout, "aws: device sign-in — open the verification URL on your LOCAL machine:\n\n%s\n\n", url)
	}

	select {
	case err := <-waitErr:
		return err
	case <-time.After(browsercapture.LoginTimeout()):
		_ = cmd.Process.Kill()
		<-waitErr
		return fmt.Errorf("aws: sign-in did not complete within %s", browsercapture.LoginTimeout())
	}
}
