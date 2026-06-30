package azure

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/slamb2k/azrl/internal/profile"
)

// Login holds the running az login process and the captured callback details.
type Login struct {
	Cmd     *exec.Cmd
	URL     string
	Port    string
	Capfile string
}

// captureCommand returns the BROWSER value: env AZRL_CAPTURE override, else the
// running binary invoked as its hidden __browser-capture subcommand.
func captureCommand() string {
	if c := os.Getenv("AZRL_CAPTURE"); c != "" {
		return c
	}
	self, err := os.Executable()
	if err != nil {
		self = "azrl"
	}
	return self + " __browser-capture"
}

// LoginCapture starts az login in the background with the BROWSER shim and polls
// for the captured callback URL. The caller owns lg.Cmd and must Wait or Kill it.
func LoginCapture(tenant string) (*Login, error) {
	cap, err := os.CreateTemp("", "azrl-cap-*")
	if err != nil {
		return nil, err
	}
	cap.Close()
	capfile := cap.Name()

	args := []string{"login"}
	if tenant != "" {
		args = append(args, "--tenant", tenant)
	}
	args = append(args, "--allow-no-subscription", "--only-show-errors")

	cmd := exec.Command("az", args...)
	cmd.Env = append(os.Environ(),
		"AZRL_CAPFILE="+capfile,
		"BROWSER="+captureCommand()+" %s",
	)
	if err := cmd.Start(); err != nil {
		os.Remove(capfile)
		return nil, err
	}

	lg := &Login{Cmd: cmd, Capfile: capfile}
	pollMax := 200 // 200 × 0.1s = 20s
	for i := 0; i < pollMax; i++ {
		if b, err := os.ReadFile(capfile); err == nil && len(b) > 0 {
			lg.URL = string(b)
			break
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lg.URL == "" {
		return lg, fmt.Errorf("azrl: timed out waiting for auth URL")
	}
	lg.Port = profile.ExtractPort(lg.URL)
	if lg.Port == "" {
		return lg, fmt.Errorf("azrl: could not parse callback port from %q", lg.URL)
	}
	return lg, nil
}
