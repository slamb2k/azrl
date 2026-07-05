package github

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGh installs a gh shim on PATH that logs its args and selected env to a
// file, then exits 0. Returns the log path.
func fakeGh(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "gh.log")
	script := "#!/usr/bin/env bash\n" +
		"{ echo \"ARGS: $*\"; echo \"GH_CONFIG_DIR=$GH_CONFIG_DIR\"; echo \"BROWSER=$BROWSER\"; echo \"AZRL_BROWSER_CMD=$AZRL_BROWSER_CMD\"; } >> \"" + log + "\"\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestLoginRunsInsecureStorageScopedToConfigDir(t *testing.T) {
	log := fakeGh(t)
	t.Setenv("GHRL_BROWSER", "myshim __browser")
	profilesDir := t.TempDir()
	c := Conf{Host: "github.com", Protocol: "https"}
	if err := Login(profilesDir, "work", c); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	out := string(b)
	for _, want := range []string{
		"auth login",
		"--insecure-storage",
		"--hostname github.com",
		"--git-protocol https",
		"GH_CONFIG_DIR=" + filepath.Join(profilesDir, "work"),
		"BROWSER=myshim __browser",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("gh invocation missing %q; got:\n%s", want, out)
		}
	}
}

func TestLoginCreatesConfigDir(t *testing.T) {
	fakeGh(t)
	t.Setenv("GHRL_BROWSER", "x")
	profilesDir := t.TempDir()
	if err := Login(profilesDir, "work", Conf{Host: "github.com"}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(profilesDir, "work"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("config dir not created: err=%v", err)
	}
}

func TestLoginPassesProfileBrowserCmdEnv(t *testing.T) {
	log := fakeGh(t)
	profilesDir := t.TempDir()
	c := Conf{Host: "github.com", BrowserCmd: "chrome-work"}
	if err := Login(profilesDir, "work", c); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	if !strings.Contains(string(b), "AZRL_BROWSER_CMD=chrome-work") {
		t.Fatalf("gh env missing the profile browser cmd:\n%s", b)
	}
}
