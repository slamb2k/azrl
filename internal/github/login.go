package github

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
)

// browserCommand returns the BROWSER value gh should use to open the device
// activation page: the GHRL_BROWSER override if set, else the running binary
// invoked as its hidden __browser shim. gh honors $BROWSER, so the local browser
// pops via the shim's relay.
func browserCommand() string {
	if c := os.Getenv("GHRL_BROWSER"); c != "" {
		return c
	}
	self, err := os.Executable()
	if err != nil {
		self = "azrl"
	}
	return self + " __browser"
}

// Login runs `gh auth login` for the profile with an isolated GH_CONFIG_DIR and
// --insecure-storage, forcing the token into the per-profile hosts.yml (the gh
// keyring is global and would otherwise collide across profiles). The web flow's
// device page opens on the local machine via the BROWSER shim. gh's own prompts
// run on the user's terminal.
func Login(profilesDir, name string, c Conf) error {
	dir := ConfigDir(profilesDir, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	protocol := c.Protocol
	if protocol == "" {
		protocol = "https"
	}
	args := []string{"auth", "login", "--hostname", c.Host,
		"--git-protocol", protocol, "--insecure-storage", "--web"}
	cmd := exec.Command("gh", args...)
	env := append(os.Environ(),
		"GH_CONFIG_DIR="+dir,
		"BROWSER="+browserCommand(),
	)
	if c.BrowserCmd != "" {
		// Propagates to the azrl __browser shim gh spawns; LoadGlobal's
		// AZRL_BROWSER_CMD hook applies it there.
		env = append(env, "AZRL_BROWSER_CMD="+c.BrowserCmd)
	}
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// gh prints its UX — including the one-time device code — on stderr.
	// Forward it raw (prompts have no trailing newline) while watching for
	// the code, and OSC 52 it into the user's LOCAL clipboard: the escape is
	// interpreted by the terminal emulator, which lives on the same machine
	// as the browser the code gets pasted into.
	forwardAndCopyCode(stderr, loginErrOut)
	return cmd.Wait()
}

// loginErrOut is where gh's forwarded stderr (and the clipboard note) go;
// overridable in tests.
var loginErrOut io.Writer = os.Stderr

// codeRe matches gh's XXXX-XXXX one-time device code.
var codeRe = regexp.MustCompile(`[A-Z0-9]{4}-[A-Z0-9]{4}`)

// forwardAndCopyCode streams r to w byte-for-byte and, on first sight of a
// one-time code, emits an OSC 52 clipboard write plus a confirmation note.
func forwardAndCopyCode(r io.Reader, w io.Writer) {
	buf := make([]byte, 4096)
	var acc []byte
	copied := false
	for {
		n, err := r.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if !copied {
				acc = append(acc, buf[:n]...)
				if m := codeRe.Find(acc); m != nil {
					copied = true
					writeOSC52(w, string(m))
					fmt.Fprintf(w, "azrl: one-time code %s copied to your local clipboard (OSC 52)\n", m)
				}
				if len(acc) > 8192 {
					acc = acc[len(acc)-16:]
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// writeOSC52 places text on the terminal's clipboard via OSC 52 — over SSH
// the sequence travels to the local emulator, so the clipboard it fills is
// the one next to the browser. Terminals without OSC 52 ignore it.
func writeOSC52(w io.Writer, text string) {
	fmt.Fprintf(w, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(text)))
}
