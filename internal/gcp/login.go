package gcp

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/slamb2k/azrl/internal/bridge"
	"github.com/slamb2k/azrl/internal/browsercapture"
	"github.com/slamb2k/azrl/internal/config"
)

// Login runs `gcloud auth login` in the background with the BROWSER shim,
// captures the authorize URL, and bridges its loopback callback (the default
// http://localhost:8085/ redirect) to the local browser over SSH (path B reverse
// tunnel / path A paste). This replaces gcloud's own --no-browser remote story,
// which would require gcloud installed on the machine with the browser. With
// isolate set it scopes CLOUDSDK_CONFIG to the profile's own dir; in every case
// it selects the resolved named configuration (configName) via
// CLOUDSDK_ACTIVE_CONFIG_NAME — the same configuration SyncConfig binds under.
func Login(dir, name, configName string, isolate bool) error {
	g, err := config.LoadGlobal(config.ProfilesDir())
	if err != nil {
		return err
	}

	cap, err := os.CreateTemp("", "azrl-gcp-cap-*")
	if err != nil {
		return err
	}
	cap.Close()
	capfile := cap.Name()
	defer os.Remove(capfile)

	cmd := exec.Command("gcloud", "auth", "login")
	env := append(selectorEnv(dir, name, configName, isolate),
		"AZRL_CAPFILE="+capfile,
		"BROWSER="+browsercapture.CaptureCommand()+" %s",
	)
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
				return fmt.Errorf("gcp: gcloud auth login exited before producing an auth URL: %w", err)
			}
			return fmt.Errorf("gcp: gcloud auth login exited before producing an auth URL")
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	if url == "" {
		_ = cmd.Process.Kill()
		<-waitErr
		return fmt.Errorf("gcp: timed out waiting for auth URL")
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
			fmt.Fprintf(os.Stdout, "gcp: browser opened on %s (zero-paste path B)\n", g.BrowserHost)
		} else if g.IsLocal() {
			fmt.Fprintf(os.Stdout, "gcp: browser opened locally (%s)\n", g.BrowserCmd)
		} else {
			fmt.Fprintf(os.Stdout, "gcp: paste this on your LOCAL machine:\n\n%s\n\n", paste)
		}
	} else {
		fmt.Fprintf(os.Stdout, "gcp: open the sign-in URL on your LOCAL machine:\n\n%s\n\n", url)
	}

	select {
	case err := <-waitErr:
		return err
	case <-time.After(browsercapture.LoginTimeout()):
		_ = cmd.Process.Kill()
		<-waitErr
		return fmt.Errorf("gcp: sign-in did not complete within %s", browsercapture.LoginTimeout())
	}
}
