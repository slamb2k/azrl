package gcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedLoginEnv installs fake gcloud/ssh/cap shims on PATH, a global azrl.conf
// under a temp HOME, and the AZRL_CAPTURE override. gcloudScript is the body of
// the fake gcloud executable. It returns the gcloud and ssh log paths.
func seedLoginEnv(t *testing.T, gcloudScript string) (gcloudLog, sshLog string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "azrl.conf"),
		[]byte("LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm-always\n"), 0o644)

	bin := t.TempDir()
	gcloudLog = filepath.Join(bin, "gcloud.log")
	sshLog = filepath.Join(bin, "ssh.log")
	os.WriteFile(filepath.Join(bin, "gcloud"), []byte(gcloudScript), 0o755)
	// ssh shim: probe (no -R) succeeds; the -R reverse tunnel dies immediately so
	// Bridge falls back to the paste line without leaving a process behind.
	sshScript := "#!/usr/bin/env bash\necho \"$*\" >> \"" + sshLog + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && exit 1; done\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(sshScript), 0o755)
	capPath := filepath.Join(bin, "cap")
	os.WriteFile(capPath, []byte("#!/usr/bin/env bash\nprintf '%s' \"$1\" > \"$AZRL_CAPFILE\"\n"), 0o755)

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZRL_CAPTURE", capPath)
	return gcloudLog, sshLog
}

func TestLoginBridgesLoopbackPort(t *testing.T) {
	gcloudScript := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"$GLOG\"\n" +
		"url='https://accounts.google.com/o/oauth2/auth?redirect_uri=http%3A%2F%2Flocalhost%3A8085%2F'\n" +
		"cmd=\"${BROWSER/\\%s/$url}\"\neval \"$cmd\"\nsleep 1\nexit 0\n"
	gcloudLog, sshLog := seedLoginEnv(t, gcloudScript)
	t.Setenv("GLOG", gcloudLog)

	if err := Login(t.TempDir(), "work", "work", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(gcloudLog)
	if !strings.Contains(string(b), "auth login") {
		t.Fatalf("gcloud invocation: %s", b)
	}
	s, _ := os.ReadFile(sshLog)
	if !strings.Contains(string(s), "-R 8085:localhost:8085 pc") {
		t.Fatalf("bridge did not tunnel the loopback port: %s", s)
	}
}

func TestLoginIsolateScopesConfigDir(t *testing.T) {
	gcloudScript := "#!/usr/bin/env bash\n" +
		"{ echo \"$*\"; echo \"CLOUDSDK_CONFIG=$CLOUDSDK_CONFIG\"; } >> \"$GLOG\"\n" +
		"url='https://accounts.google.com/o/oauth2/auth?redirect_uri=http%3A%2F%2Flocalhost%3A8085%2F'\n" +
		"cmd=\"${BROWSER/\\%s/$url}\"\neval \"$cmd\"\nsleep 1\nexit 0\n"
	gcloudLog, _ := seedLoginEnv(t, gcloudScript)
	t.Setenv("GLOG", gcloudLog)

	dir := t.TempDir()
	if err := Login(dir, "work", "work", true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(gcloudLog)
	if !strings.Contains(string(b), "CLOUDSDK_CONFIG="+filepath.Join(dir, "work")) {
		t.Fatalf("isolate did not scope CLOUDSDK_CONFIG: %s", b)
	}
}

func TestLoginDefaultSelectsNamedConfig(t *testing.T) {
	gcloudScript := "#!/usr/bin/env bash\n" +
		"{ echo \"$*\"; echo \"CLOUDSDK_ACTIVE_CONFIG_NAME=$CLOUDSDK_ACTIVE_CONFIG_NAME\"; } >> \"$GLOG\"\n" +
		"url='https://accounts.google.com/o/oauth2/auth?redirect_uri=http%3A%2F%2Flocalhost%3A8085%2F'\n" +
		"cmd=\"${BROWSER/\\%s/$url}\"\neval \"$cmd\"\nsleep 1\nexit 0\n"
	gcloudLog, _ := seedLoginEnv(t, gcloudScript)
	t.Setenv("GLOG", gcloudLog)

	if err := Login(t.TempDir(), "work", "work", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(gcloudLog)
	if !strings.Contains(string(b), "CLOUDSDK_ACTIVE_CONFIG_NAME=work") {
		t.Fatalf("default login did not select the named configuration: %s", b)
	}
}

// TestLoginActiveAccountUseResolvedConfigName pins the FIX-1 invariant: when the
// gcloud configuration name differs from the profile name (GCP_CONFIG_NAME set),
// Login and ActiveAccount must select the SAME configuration that SyncConfig binds
// under — the resolved config name, never the raw profile name.
func TestLoginActiveAccountUseResolvedConfigName(t *testing.T) {
	gcloudScript := "#!/usr/bin/env bash\n" +
		"{ echo \"$*\"; echo \"CLOUDSDK_ACTIVE_CONFIG_NAME=$CLOUDSDK_ACTIVE_CONFIG_NAME\"; } >> \"$GLOG\"\n" +
		"if [[ \"$1 $2\" == \"auth list\" ]]; then printf '%s' 'x@y.com'; exit 0; fi\n" +
		"if [[ \"$1 $2\" == \"auth login\" ]]; then\n" +
		"  url='https://accounts.google.com/o/oauth2/auth?redirect_uri=http%3A%2F%2Flocalhost%3A8085%2F'\n" +
		"  cmd=\"${BROWSER/\\%s/$url}\"; eval \"$cmd\"; sleep 1; exit 0\n" +
		"fi\nexit 0\n"
	gcloudLog, _ := seedLoginEnv(t, gcloudScript)
	t.Setenv("GLOG", gcloudLog)

	const cfg = "acme-prod" // resolved config name, distinct from profile "work"
	if err := Login(t.TempDir(), "work", cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ActiveAccount(t.TempDir(), "work", cfg, false); err != nil {
		t.Fatal(err)
	}
	if err := SyncConfig("work", Conf{ConfigName: cfg, Project: "p"}, false); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(gcloudLog)
	s := string(out)
	if strings.Contains(s, "CLOUDSDK_ACTIVE_CONFIG_NAME=work") {
		t.Fatalf("selected the raw profile name instead of the resolved config:\n%s", s)
	}
	if !strings.Contains(s, "CLOUDSDK_ACTIVE_CONFIG_NAME="+cfg) {
		t.Fatalf("Login/ActiveAccount did not select the resolved config %q:\n%s", cfg, s)
	}
	if !strings.Contains(s, "config configurations create "+cfg) {
		t.Fatalf("SyncConfig did not bind under the resolved config %q:\n%s", cfg, s)
	}
}
