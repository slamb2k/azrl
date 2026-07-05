package browserpick

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slamb2k/azrl/internal/config"
)

// fakeSSH installs an ssh shim that logs args and prints the given stdout.
func fakeSSH(t *testing.T, script string) (logPath string) {
	t.Helper()
	bin := t.TempDir()
	logPath = filepath.Join(bin, "ssh.log")
	body := "#!/usr/bin/env bash\necho \"$*\" >> \"" + logPath + "\"\n" + script
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestDiscoverParsesPosixProbe(t *testing.T) {
	log := fakeSSH(t, `cat <<'EOF'
===AZRL /home/u/.config/microsoft-edge/Local State
{"profile":{"info_cache":{"Profile 2":{"name":"Work","user_name":"simon@acme.com"}}}}
===AZRL /home/u/.config/google-chrome/Local State
{"profile":{"info_cache":{"Default":{"name":"Personal","user_name":"me@gmail.com"}}}}
EOF
`)
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm"}
	ps, err := Discover(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("want 2 profiles, got %+v", ps)
	}
	if ps[0].Browser == ps[1].Browser {
		t.Fatalf("want one edge + one chrome, got %+v", ps)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "BatchMode=yes") || !strings.Contains(string(b), "pc") {
		t.Fatalf("probe must use BatchMode and target LocalHost:\n%s", b)
	}
}

func TestDiscoverUnreachable(t *testing.T) {
	fakeSSH(t, "exit 1\n")
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm"}
	if _, err := Discover(g); err == nil {
		t.Fatal("unreachable host must return an error")
	}
}

func TestDiscoverEmptyOutput(t *testing.T) {
	fakeSSH(t, "exit 0\n")
	g := config.Global{LocalHost: "pc", LocalBrowserCmd: "wslview", VMHost: "vm"}
	if _, err := Discover(g); err == nil {
		t.Fatal("no profiles found must return an error")
	}
}
