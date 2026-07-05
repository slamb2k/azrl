package aws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedLoginEnv installs fake aws/ssh/cap shims on PATH, a global azrl.conf under
// a temp HOME, and the AZRL_CAPTURE override. awsScript is the body of the fake
// aws executable. It returns the aws and ssh log paths.
func seedLoginEnv(t *testing.T, awsScript string) (awsLog, sshLog string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AZRL_BROWSER_CMD", "")
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "azrl.conf"),
		[]byte("LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm-always\n"), 0o644)

	bin := t.TempDir()
	awsLog = filepath.Join(bin, "aws.log")
	sshLog = filepath.Join(bin, "ssh.log")
	os.WriteFile(filepath.Join(bin, "aws"), []byte(awsScript), 0o755)
	// ssh shim: probe (no -R) succeeds; the -R reverse tunnel dies immediately so
	// Bridge falls back to the paste line without leaving a process behind.
	sshScript := "#!/usr/bin/env bash\necho \"$*\" >> \"" + sshLog + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && exit 1; done\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(sshScript), 0o755)
	capPath := filepath.Join(bin, "cap")
	os.WriteFile(capPath, []byte("#!/usr/bin/env bash\nprintf '%s' \"$1\" > \"$AZRL_CAPFILE\"\n"), 0o755)

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZRL_CAPTURE", capPath)
	return awsLog, sshLog
}

func TestLoginPKCEBridgesLoopbackPort(t *testing.T) {
	awsScript := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"$AWS_LOG\"\n" +
		"url='https://oidc/authorize?redirect_uri=http%3A%2F%2F127.0.0.1%3A55021%2Foauth%2Fcallback'\n" +
		"cmd=\"${BROWSER/\\%s/$url}\"\neval \"$cmd\"\nsleep 1\nexit 0\n"
	awsLog, sshLog := seedLoginEnv(t, awsScript)
	t.Setenv("AWS_LOG", awsLog)

	if err := Login(t.TempDir(), "work", false, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(awsLog)
	out := string(b)
	if !strings.Contains(out, "sso login") || !strings.Contains(out, "--profile work") {
		t.Fatalf("aws invocation: %s", out)
	}
	if strings.Contains(out, "--use-device-code") {
		t.Fatalf("PKCE login should not pass --use-device-code: %s", out)
	}
	s, _ := os.ReadFile(sshLog)
	if !strings.Contains(string(s), "-R 55021:localhost:55021 pc") {
		t.Fatalf("bridge did not tunnel the callback port: %s", s)
	}
}

func TestLoginDeviceCodePassesFlag(t *testing.T) {
	awsScript := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"$AWS_LOG\"\n" +
		"url='https://device.sso.amazonaws.com/?user_code=ABCD-EFGH'\n" +
		"cmd=\"${BROWSER/\\%s/$url}\"\neval \"$cmd\"\nsleep 1\nexit 0\n"
	awsLog, _ := seedLoginEnv(t, awsScript)
	t.Setenv("AWS_LOG", awsLog)

	if err := Login(t.TempDir(), "work", false, true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(awsLog)
	if !strings.Contains(string(b), "--use-device-code") {
		t.Fatalf("device login must pass --use-device-code: %s", b)
	}
}

func TestLoginIsolateScopesConfigFiles(t *testing.T) {
	awsScript := "#!/usr/bin/env bash\n" +
		"{ echo \"$*\"; echo \"AWS_CONFIG_FILE=$AWS_CONFIG_FILE\"; echo \"AWS_SHARED_CREDENTIALS_FILE=$AWS_SHARED_CREDENTIALS_FILE\"; } >> \"$AWS_LOG\"\n" +
		"url='https://oidc/authorize?redirect_uri=http%3A%2F%2F127.0.0.1%3A55021%2Foauth%2Fcallback'\n" +
		"cmd=\"${BROWSER/\\%s/$url}\"\neval \"$cmd\"\nsleep 1\nexit 0\n"
	awsLog, _ := seedLoginEnv(t, awsScript)
	t.Setenv("AWS_LOG", awsLog)

	dir := t.TempDir()
	if err := Login(dir, "work", true, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(awsLog)
	out := string(b)
	if !strings.Contains(out, "AWS_CONFIG_FILE="+filepath.Join(dir, "work", "config")) ||
		!strings.Contains(out, "AWS_SHARED_CREDENTIALS_FILE="+filepath.Join(dir, "work", "credentials")) {
		t.Fatalf("isolate did not scope the config files: %s", out)
	}
}
