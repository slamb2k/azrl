package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slamb2k/azrl/internal/profile"
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

func TestAwsCapturePreservesExistingKeys(t *testing.T) {
	home := seedAwsHome(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_ACCOUNT_ID=123456789012\nAWS_ROLE_NAME=AdminAccess\n"+
			"AWS_LABEL=Keep Me\nAWS_ISOLATE=true\nAWS_BROWSER_CMD=chrome-work\nAWS_BROWSER_LABEL=Edge — Work\n"), 0o644)

	runRoot(t, "aws", "capture", "work", "--sso-start-url", "https://acme.awsapps.com/start",
		"--account-id", "123456789012", "--role-name", "AdminAccess")

	b, err := os.ReadFile(filepath.Join(ap, "work.conf"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, "AWS_LABEL=Keep Me") || !strings.Contains(out, "AWS_ISOLATE=true") ||
		!strings.Contains(out, "AWS_BROWSER_CMD=chrome-work") || !strings.Contains(out, "AWS_BROWSER_LABEL=Edge — Work") {
		t.Fatalf("recapture wiped existing keys:\n%s", out)
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
	t.Setenv("AZRL_BROWSER_CMD", "")
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

func TestAwsLoginUnknownWithoutStartURLErrors(t *testing.T) {
	home := seedAwsHome(t)
	chdirClean(t)

	err := runRootErr(t, "aws", "login", "brandnew")
	if err == nil {
		t.Fatal("unknown profile without --sso-start-url should error")
	}
	if !strings.Contains(err.Error(), "provide --sso-start-url") {
		t.Fatalf("wrong error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".aws-profiles", "brandnew.conf")); !os.IsNotExist(statErr) {
		t.Fatal("no conf must be written without a start URL")
	}
}

func TestAwsLoginCreatesWithStartURLAndYes(t *testing.T) {
	seedAwsLoginEnv(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	home := os.Getenv("HOME")
	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"aws", "login", "fresh",
		"--sso-start-url", "https://fresh.awsapps.com/start", "--yes"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("create-on-login should succeed: %v (out=%q)", err, buf.String())
	}
	if !strings.Contains(buf.String(), `created profile "fresh" (https://fresh.awsapps.com/start)`) {
		t.Fatalf("missing created-profile announce:\n%s", buf.String())
	}
	b, err := os.ReadFile(filepath.Join(home, ".aws-profiles", "fresh.conf"))
	if err != nil {
		t.Fatalf("fresh.conf not written: %v", err)
	}
	if !strings.Contains(string(b), "AWS_SSO_START_URL=https://fresh.awsapps.com/start") {
		t.Fatalf("created conf missing start URL:\n%s", b)
	}
	// Pin-on-create: the new profile pins the cwd.
	if pin, err := os.ReadFile(filepath.Join(work, ".awsprofile")); err != nil || strings.TrimSpace(string(pin)) != "fresh" {
		t.Fatalf(".awsprofile not pinned on create (err=%v pin=%q)", err, pin)
	}
}

// TestAwsLoginFirstLoginCreatesFromPrompt proves that on a TTY with zero saved
// profiles, `aws login --sso-start-url ...` prompts for a name (defaulting to the
// dir), creates the profile and signs in — with no second [y/N] confirm.
func TestAwsLoginFirstLoginCreatesFromPrompt(t *testing.T) {
	_, confPath := seedAwsLoginEnv(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	os.Remove(confPath) // zero profiles -> first-login prompt
	home := os.Getenv("HOME")
	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)
	stubInteractive(t, true)
	pwd, _ := os.Getwd()
	want := profile.DefaultName("", pwd)

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetIn(strings.NewReader("\n"))
	RootCmd.SetArgs([]string{"aws", "login", "--sso-start-url", "https://fresh.awsapps.com/start"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("aws first-login should succeed: %v (out=%q)", err, buf.String())
	}
	if !strings.Contains(buf.String(), "No azrl aws profiles yet") {
		t.Fatalf("missing first-login prompt:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `created profile "`+want+`"`) {
		t.Fatalf("missing created-profile announce:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "doesn't exist. Create it") {
		t.Fatalf("must not double-confirm the just-named profile:\n%s", buf.String())
	}
	b, err := os.ReadFile(filepath.Join(home, ".aws-profiles", want+".conf"))
	if err != nil {
		t.Fatalf("aws conf not created: %v", err)
	}
	if !strings.Contains(string(b), "AWS_SSO_START_URL=https://fresh.awsapps.com/start") {
		t.Fatalf("created conf missing start URL:\n%s", b)
	}
}

// TestAwsLoginFirstLoginWithoutStartURLErrors proves the --sso-start-url guard
// still applies on the first-login path: even after naming the profile, aws
// refuses to create an empty-SSO profile and writes nothing.
func TestAwsLoginFirstLoginWithoutStartURLErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".aws-profiles"), 0o755)
	chdirClean(t)
	stubInteractive(t, true)
	pwd, _ := os.Getwd()
	want := profile.DefaultName("", pwd)

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetIn(strings.NewReader("\n"))
	// Reset --sso-start-url explicitly: cobra flag vars persist across tests.
	RootCmd.SetArgs([]string{"aws", "login", "--sso-start-url=", "--yes=false"})
	err := RootCmd.Execute()
	if err == nil {
		t.Fatalf("first-login without --sso-start-url should error (out=%q)", buf.String())
	}
	if !strings.Contains(err.Error(), "provide --sso-start-url") {
		t.Fatalf("wrong error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".aws-profiles", want+".conf")); !os.IsNotExist(statErr) {
		t.Fatal("no conf should be written without a start URL")
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

func TestAwsLoginProfileBrowserCmdOverridesGlobal(t *testing.T) {
	confBody := "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_BROWSER_CMD=chrome-work\n"
	awsLog, _ := seedAwsLoginEnv(t, confBody)
	// Replace the seed's dying ssh with one whose -R tunnel stays alive, so
	// the bridge takes path B and the browser launch lands in ssh.log.
	bin := filepath.Dir(awsLog)
	sshLog := filepath.Join(bin, "ssh.log")
	alive := "#!/usr/bin/env bash\necho \"$*\" >> \"" + sshLog + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && { sleep 2; exit 0; }; done\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(alive), 0o755)

	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	if err := runRootErr(t, "aws", "login", "work"); err != nil {
		t.Fatalf("login: %v", err)
	}
	b, _ := os.ReadFile(sshLog)
	if !strings.Contains(string(b), "chrome-work") {
		t.Fatalf("browser launch should use the profile cmd:\n%s", b)
	}
	if strings.Contains(string(b), "wslview") {
		t.Fatalf("global browser cmd leaked:\n%s", b)
	}
}
