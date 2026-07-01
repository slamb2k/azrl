package browsercapture

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestXdgOpenShimForwards writes the generated xdg-open wrapper pointed at a
// fake binary, runs it with a URL, and asserts the wrapper forwarded
// `__browser <url>` to that binary — the mechanism GCM's xdg-open exec relies on.
func TestXdgOpenShimForwards(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "bin.log")
	fakeBin := filepath.Join(dir, "azrl")
	fakeScript := "#!/usr/bin/env bash\necho \"$*\" >> \"" + log + "\"\n"
	if err := os.WriteFile(fakeBin, []byte(fakeScript), 0o755); err != nil {
		t.Fatal(err)
	}

	shimPath := filepath.Join(dir, "xdg-open")
	if err := os.WriteFile(shimPath, []byte(XdgOpenShimScript(fakeBin)), 0o755); err != nil {
		t.Fatal(err)
	}

	url := "https://github.com/login/oauth/authorize?redirect_uri=http://127.0.0.1:5000/"
	if err := exec.Command(shimPath, url).Run(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	if got := strings.TrimSpace(string(b)); got != "__browser "+url {
		t.Fatalf("forwarded %q want %q", got, "__browser "+url)
	}
}
