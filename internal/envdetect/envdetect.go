// Package envdetect turns a snapshot of the host environment into ranked
// candidate azrl configurations. Its decision logic is pure — Detect reads only
// the injected Env — so every signal combination is table-testable. RealEnv is
// the single impure helper that samples the actual process/OS.
package envdetect

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Mode is the login topology a candidate configures.
type Mode int

const (
	// Local: azrl and the browser share this machine; no SSH bridge.
	Local Mode = iota
	// Remote: azrl runs on a VM and the browser opens on the dev machine.
	Remote
)

// Env is an injected snapshot of the host environment. Detect makes no OS reads
// of its own, taking every signal from here.
type Env struct {
	ProcVersion   string                // contents of /proc/version ("" if unreadable)
	WSLDistro     string                // $WSL_DISTRO_NAME
	GOOS          string                // runtime.GOOS
	Display       string                // $DISPLAY + $WAYLAND_DISPLAY concatenated
	SSHConnection string                // $SSH_CONNECTION
	SSHTTY        string                // $SSH_TTY
	Has           func(bin string) bool // PATH lookup (exec.LookPath != nil)
}

// Candidate is a fully-formed config proposal with pre-filled defaults, a human
// label, the reason it was detected, and whether it is the recommended pick.
type Candidate struct {
	Mode        Mode
	Label       string
	Reason      string
	BrowserCmd  string // pre-filled default; "" for remote (the wizard asks the dev-machine OS)
	BrowserHost string // "localhost" for local; "" for remote
	VMSSHHost   string // derived for remote; "" otherwise
	Recommended bool
}

// DeriveVMHost returns field 3 (the server IP) of a well-formed $SSH_CONNECTION
// ("client_ip client_port server_ip server_port"), or "" for empty/malformed
// input.
func DeriveVMHost(sshConn string) string {
	f := strings.Fields(sshConn)
	if len(f) < 4 {
		return ""
	}
	return f[2]
}

// localCandidate returns the single best local candidate for the environment,
// or ok=false when no local browser signal is present.
func localCandidate(e Env) (Candidate, bool) {
	if strings.Contains(strings.ToLower(e.ProcVersion), "microsoft") || e.WSLDistro != "" {
		return Candidate{Mode: Local, Label: "Local (WSL → Windows browser)", Reason: "WSL detected", BrowserCmd: "wslview", BrowserHost: "localhost"}, true
	}
	if e.GOOS == "darwin" {
		return Candidate{Mode: Local, Label: "Local (macOS browser)", Reason: "macOS detected", BrowserCmd: "open", BrowserHost: "localhost"}, true
	}
	if e.Display != "" && e.Has != nil && e.Has("xdg-open") {
		return Candidate{Mode: Local, Label: "Local (Linux desktop browser)", Reason: "X11/Wayland display + xdg-open", BrowserCmd: "xdg-open", BrowserHost: "localhost"}, true
	}
	return Candidate{}, false
}

// Detect returns ranked candidates (recommended first), always at least one. In
// an SSH session the remote candidate is recommended; otherwise the local one
// is. At most one local and one remote candidate are returned.
func Detect(e Env) []Candidate {
	local, hasLocal := localCandidate(e)
	inSSH := e.SSHConnection != "" || e.SSHTTY != ""

	remote := Candidate{Mode: Remote, Label: "Remote (browser on your dev machine over SSH)"}
	hasRemote := false
	switch {
	case inSSH:
		remote.Reason = "SSH session detected"
		remote.VMSSHHost = DeriveVMHost(e.SSHConnection)
		hasRemote = true
	case !hasLocal:
		// No local browser and no SSH session: still offer a remote candidate so
		// the wizard has something to fill in (blank VM host for the user to type).
		remote.Reason = "no local browser detected"
		hasRemote = true
	}

	var out []Candidate
	if inSSH {
		remote.Recommended = true
		out = append(out, remote)
		if hasLocal {
			out = append(out, local)
		}
		return out
	}
	if hasLocal {
		local.Recommended = true
		out = append(out, local)
	}
	if hasRemote {
		remote.Recommended = !hasLocal
		out = append(out, remote)
	}
	return out
}

// RealEnv samples the actual process/OS into an Env. This is the only impure
// helper in the package.
func RealEnv() Env {
	proc, _ := os.ReadFile("/proc/version")
	return Env{
		ProcVersion:   string(proc),
		WSLDistro:     os.Getenv("WSL_DISTRO_NAME"),
		GOOS:          runtime.GOOS,
		Display:       os.Getenv("DISPLAY") + os.Getenv("WAYLAND_DISPLAY"),
		SSHConnection: os.Getenv("SSH_CONNECTION"),
		SSHTTY:        os.Getenv("SSH_TTY"),
		Has:           func(bin string) bool { _, err := exec.LookPath(bin); return err == nil },
	}
}
