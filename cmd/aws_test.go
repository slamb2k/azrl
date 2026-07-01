package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedAwsHome points HOME at a temp dir with two AWS profiles on disk.
func seedAwsHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_SSO_REGION=us-east-1\nAWS_ACCOUNT_ID=123456789012\nAWS_ROLE_NAME=AdminAccess\n"), 0o644)
	os.WriteFile(filepath.Join(ap, "play.conf"),
		[]byte("AWS_SSO_START_URL=https://play.awsapps.com/start\nAWS_ACCOUNT_ID=210987654321\nAWS_ROLE_NAME=ReadOnly\n"), 0o644)
	return home
}

func TestAwsListPrintsProfiles(t *testing.T) {
	seedAwsHome(t)
	out := runRoot(t, "aws", "list")
	if !strings.Contains(out, "work") || !strings.Contains(out, "acme.awsapps.com") ||
		!strings.Contains(out, "play") || !strings.Contains(out, "play.awsapps.com") {
		t.Fatalf("aws list output:\n%s", out)
	}
}

func TestAwsUsePinsRepoAndSyncsConfig(t *testing.T) {
	home := seedAwsHome(t)
	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)
	RootCmd.SetIn(strings.NewReader("n\n")) // decline the .envrc prompt

	runRoot(t, "aws", "use", "work")

	b, err := os.ReadFile(filepath.Join(work, ".awsprofile"))
	if err != nil || strings.TrimSpace(string(b)) != "work" {
		t.Fatalf(".awsprofile=%q err=%v", string(b), err)
	}
	cfg, err := os.ReadFile(filepath.Join(home, ".aws", "config"))
	if err != nil {
		t.Fatalf("~/.aws/config not written: %v", err)
	}
	if !strings.Contains(string(cfg), "[profile work]") || !strings.Contains(string(cfg), "[sso-session work]") {
		t.Fatalf("~/.aws/config missing stanzas:\n%s", cfg)
	}
}

func TestAwsRmRemovesProfile(t *testing.T) {
	home := seedAwsHome(t)
	runRoot(t, "aws", "rm", "work")
	if _, err := os.Stat(filepath.Join(home, ".aws-profiles", "work.conf")); !os.IsNotExist(err) {
		t.Fatal("work.conf not removed")
	}
}

func TestAwsCaptureWritesConf(t *testing.T) {
	home := seedAwsHome(t)
	runRoot(t, "aws", "capture", "new", "--sso-start-url", "https://new.awsapps.com/start",
		"--account-id", "555", "--role-name", "Admin")
	b, err := os.ReadFile(filepath.Join(home, ".aws-profiles", "new.conf"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, "AWS_SSO_START_URL=https://new.awsapps.com/start") ||
		!strings.Contains(out, "AWS_ACCOUNT_ID=555") || !strings.Contains(out, "AWS_ROLE_NAME=Admin") {
		t.Fatalf("captured conf:\n%s", out)
	}
}

// seedAwsLoginEnv wires a full login environment for `aws login`: a global
// azrl.conf, an aws-profiles conf carrying expect values, and aws/ssh/cap shims
// on PATH. The aws shim answers `sso login` by driving the BROWSER bridge and
// `sts get-caller-identity` with $STS_JSON. It returns the aws log path and the
// profile's conf path.
func seedAwsLoginEnv(t *testing.T, confBody string) (awsLog, confPath string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "azrl.conf"),
		[]byte("LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm-always\n"), 0o644)

	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	confPath = filepath.Join(ap, "work.conf")
	os.WriteFile(confPath, []byte(confBody), 0o644)

	bin := t.TempDir()
	awsLog = filepath.Join(bin, "aws.log")
	sshLog := filepath.Join(bin, "ssh.log")
	awsScript := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"" + awsLog + "\"\n" +
		"if [[ \"$1 $2\" == \"sts get-caller-identity\" ]]; then printf '%s' \"$STS_JSON\"; exit 0; fi\n" +
		"if [[ \"$1 $2\" == \"sso login\" ]]; then\n" +
		"  url='https://oidc/authorize?redirect_uri=http%3A%2F%2F127.0.0.1%3A55021%2Foauth%2Fcallback'\n" +
		"  cmd=\"${BROWSER/\\%s/$url}\"; eval \"$cmd\"; sleep 1; exit 0\n" +
		"fi\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "aws"), []byte(awsScript), 0o755)
	sshScript := "#!/usr/bin/env bash\necho \"$*\" >> \"" + sshLog + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && exit 1; done\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(sshScript), 0o755)
	capPath := filepath.Join(bin, "cap")
	os.WriteFile(capPath, []byte("#!/usr/bin/env bash\nprintf '%s' \"$1\" > \"$AZRL_CAPFILE\"\n"), 0o755)

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZRL_CAPTURE", capPath)
	return awsLog, confPath
}

// runRootErr executes RootCmd like runRoot but returns the error instead of
// failing the test, so a caller can assert a command is expected to fail.
func runRootErr(t *testing.T, args ...string) error {
	t.Helper()
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs(args)
	return RootCmd.Execute()
}

func TestAwsLoginAssertsMatchingIdentityAndTouches(t *testing.T) {
	confBody := "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_SSO_REGION=us-east-1\n" +
		"AWS_ACCOUNT_ID=123456789012\nAWS_ROLE_NAME=AdminAccess\n" +
		"AWS_EXPECT_ACCOUNT=123456789012\nAWS_EXPECT_ARN=AWSReservedSSO_AdminAccess\n"
	awsLog, confPath := seedAwsLoginEnv(t, confBody)
	t.Setenv("STS_JSON", `{"Account":"123456789012","Arn":"arn:aws:sts::123456789012:assumed-role/AWSReservedSSO_AdminAccess_a1b2c3d4/simon"}`)

	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	if err := runRootErr(t, "aws", "login", "work"); err != nil {
		t.Fatalf("matching identity login should succeed: %v", err)
	}
	if b, _ := os.ReadFile(awsLog); !strings.Contains(string(b), "sts get-caller-identity") {
		t.Fatalf("guardrail did not call get-caller-identity:\n%s", b)
	}
	if b, _ := os.ReadFile(confPath); !strings.Contains(string(b), "LAST_USED") {
		t.Fatalf("Touch did not run after a successful assert:\n%s", b)
	}
}

func TestAwsLoginRejectsWrongAccountAndSkipsTouch(t *testing.T) {
	confBody := "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_SSO_REGION=us-east-1\n" +
		"AWS_ACCOUNT_ID=123456789012\nAWS_ROLE_NAME=AdminAccess\n" +
		"AWS_EXPECT_ACCOUNT=123456789012\nAWS_EXPECT_ARN=AWSReservedSSO_AdminAccess\n"
	_, confPath := seedAwsLoginEnv(t, confBody)
	t.Setenv("STS_JSON", `{"Account":"999999999999","Arn":"arn:aws:sts::999999999999:assumed-role/AWSReservedSSO_AdminAccess_a1b2c3d4/simon"}`)

	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	err := runRootErr(t, "aws", "login", "work")
	if err == nil {
		t.Fatal("login into the wrong account should fail")
	}
	if !strings.Contains(err.Error(), "ACCOUNT MISMATCH") {
		t.Fatalf("expected an account-mismatch error, got: %v", err)
	}
	if b, _ := os.ReadFile(confPath); strings.Contains(string(b), "LAST_USED") {
		t.Fatalf("Touch must NOT run when the identity assertion fails:\n%s", b)
	}
}

func TestValidAwsNameRejectsReserved(t *testing.T) {
	if err := validAwsName("aws"); err == nil {
		t.Fatal("expected the reserved name 'aws' to be rejected")
	}
	if err := validAwsName(""); err == nil {
		t.Fatal("expected an empty name to be rejected")
	}
}
