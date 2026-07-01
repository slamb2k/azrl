package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedAzLoginEnv wires a full environment for `azrl login`: a global azrl.conf,
// the named per-profile confs, and az/ssh/cap shims on PATH. The az shim answers
// `account show` from $AZ_ACCT and drives the BROWSER bridge for `login`,
// recording every invocation's argv to the returned az.log so tests can assert
// which name (or the tenant-less form) was resolved. It mirrors the AWS login
// harness (seedAwsLoginEnv). confs maps profile name -> conf body.
func seedAzLoginEnv(t *testing.T, confs map[string]string) (azLog string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Keep any leaked AZURE_CONFIG_DIR from login.go scoped to this test.
	t.Setenv("AZURE_CONFIG_DIR", "")

	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "azrl.conf"),
		[]byte("LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm-always\n"), 0o644)
	for name, body := range confs {
		os.WriteFile(filepath.Join(az, name+".conf"), []byte(body), 0o644)
	}

	bin := t.TempDir()
	azLog = filepath.Join(bin, "az.log")
	sshLog := filepath.Join(bin, "ssh.log")
	azScript := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"" + azLog + "\"\n" +
		"if [[ \"$1 $2\" == \"account show\" ]]; then printf '%s' \"$AZ_ACCT\"; exit 0; fi\n" +
		"if [[ \"$1\" == \"login\" ]]; then\n" +
		"  url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F&s=z'\n" +
		"  cmd=\"${BROWSER/\\%s/$url}\"; eval \"$cmd\"; exit 0\n" +
		"fi\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "az"), []byte(azScript), 0o755)
	// ssh: fail the reverse tunnel (-R) so the bridge falls back to the paste
	// path and never opens a real tunnel.
	sshScript := "#!/usr/bin/env bash\necho \"$*\" >> \"" + sshLog + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && exit 1; done\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(sshScript), 0o755)
	capPath := filepath.Join(bin, "cap")
	os.WriteFile(capPath, []byte("#!/usr/bin/env bash\nprintf '%s' \"$1\" > \"$AZRL_CAPFILE\"\n"), 0o755)

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZRL_CAPTURE", capPath)
	t.Setenv("AZRL_LOGIN_TIMEOUT", "20")
	return azLog
}

// acctJSON is a minimal `az account show -o json` document whose default domain
// is tenant, so AssertAccount matches a profile carrying AZ_TENANT=tenant.
func acctJSON(tenant, user string) string {
	return `{"tenantId":"guid-x","tenantDefaultDomain":"` + tenant +
		`","id":"sub-1","name":"Pay-As-You-Go","user":{"name":"` + user + `"}}`
}

// execRoot runs RootCmd capturing combined out/err, returning both the buffer
// text and any error (unlike runRoot it never fails the test on error).
func execRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs(args)
	err := RootCmd.Execute()
	return buf.String(), err
}

// TestLoginZeroProfilesTenantless proves that with no arg, no directory pin and
// no saved profiles, azrl signs in tenant-less (no --tenant) into default
// ~/.azure rather than erroring.
func TestLoginZeroProfilesTenantless(t *testing.T) {
	azLog := seedAzLoginEnv(t, nil)
	t.Setenv("AZ_ACCT", acctJSON("contoso.com", "simon"))
	chdirClean(t)

	out, err := execRoot(t, "login")
	if err != nil {
		t.Fatalf("tenant-less login should succeed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "tenant-less sign-in") {
		t.Fatalf("missing tenant-less announcement:\n%s", out)
	}
	log, _ := os.ReadFile(azLog)
	if !strings.Contains(string(log), "login") || !strings.Contains(string(log), "--allow-no-subscription") {
		t.Fatalf("az login not invoked:\n%s", log)
	}
	if strings.Contains(string(log), "--tenant") {
		t.Fatalf("tenant-less path must not pass --tenant:\n%s", log)
	}
}

// TestLoginSingleProfileAutoSelect proves the sole profile is announced and used
// (its tenant reaches az login) without any interactive prompt.
func TestLoginSingleProfileAutoSelect(t *testing.T) {
	azLog := seedAzLoginEnv(t, map[string]string{"solo": "AZ_TENANT=solo.example.com\n"})
	t.Setenv("AZ_ACCT", acctJSON("solo.example.com", "simon"))
	chdirClean(t)

	out, err := execRoot(t, "login")
	if err != nil {
		t.Fatalf("single-profile login should succeed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, `using the only profile "solo"`) {
		t.Fatalf("missing auto-select announcement:\n%s", out)
	}
	if !strings.Contains(out, "profile=solo tenant=solo.example.com") {
		t.Fatalf("wrong resolved profile:\n%s", out)
	}
	if log, _ := os.ReadFile(azLog); !strings.Contains(string(log), "--tenant solo.example.com") {
		t.Fatalf("az login did not target the solo tenant:\n%s", log)
	}
}

// TestLoginMultiNonInteractiveErrors proves that with >=2 profiles and no TTY,
// login refuses with the "specify one of" error, lists the names, attempts no
// sign-in, and dumps no usage block.
func TestLoginMultiNonInteractiveErrors(t *testing.T) {
	azLog := seedAzLoginEnv(t, map[string]string{
		"work": "AZ_TENANT=work.example.com\n",
		"emu":  "AZ_TENANT=emu.example.com\n",
	})
	chdirClean(t)
	stubInteractive(t, false)

	out, err := execRoot(t, "login")
	if err == nil {
		t.Fatalf("multi-profile non-interactive login should error (out=%q)", out)
	}
	if !strings.Contains(err.Error(), "multiple profiles — specify one of") {
		t.Fatalf("wrong error: %v", err)
	}
	if !strings.Contains(err.Error(), "emu") || !strings.Contains(err.Error(), "work") {
		t.Fatalf("error should list both names: %v", err)
	}
	if _, statErr := os.Stat(azLog); statErr == nil {
		log, _ := os.ReadFile(azLog)
		t.Fatalf("no az command should run when resolution fails:\n%s", log)
	}
	if strings.Contains(out, "Usage:") {
		t.Fatalf("runtime error must not dump usage:\n%s", out)
	}
}

// TestLoginMultiInteractivePicksChoice proves an interactive selection fed on
// stdin chooses that profile and drives login for it.
func TestLoginMultiInteractivePicksChoice(t *testing.T) {
	azLog := seedAzLoginEnv(t, map[string]string{
		"work": "AZ_TENANT=work.example.com\n",
		"emu":  "AZ_TENANT=emu.example.com\n",
	})
	t.Setenv("AZ_ACCT", acctJSON("work.example.com", "simon"))
	chdirClean(t)
	stubInteractive(t, true)
	// Profiles sort by name: [emu, work]; selecting 2 -> "work".
	RootCmd.SetIn(strings.NewReader("2\n"))

	out, err := execRoot(t, "login")
	if err != nil {
		t.Fatalf("interactive login should succeed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "Select a profile") {
		t.Fatalf("picker prompt missing:\n%s", out)
	}
	if !strings.Contains(out, "profile=work tenant=work.example.com") {
		t.Fatalf("selection 2 should resolve to work:\n%s", out)
	}
	if log, _ := os.ReadFile(azLog); !strings.Contains(string(log), "--tenant work.example.com") {
		t.Fatalf("az login did not target the chosen tenant:\n%s", log)
	}
}

// TestLoginExplicitArgBeatsPin proves an explicit profile arg wins over a
// directory .azprofile pin.
func TestLoginExplicitArgBeatsPin(t *testing.T) {
	azLog := seedAzLoginEnv(t, map[string]string{
		"work": "AZ_TENANT=work.example.com\n",
		"emu":  "AZ_TENANT=emu.example.com\n",
	})
	t.Setenv("AZ_ACCT", acctJSON("work.example.com", "simon"))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".azprofile"), []byte("emu\n"), 0o644)
	chdir(t, dir)

	out, err := execRoot(t, "login", "work")
	if err != nil {
		t.Fatalf("explicit-arg login should succeed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "profile=work tenant=work.example.com") {
		t.Fatalf("explicit arg should beat the emu pin:\n%s", out)
	}
	if log, _ := os.ReadFile(azLog); !strings.Contains(string(log), "--tenant work.example.com") {
		t.Fatalf("az login did not target the arg tenant:\n%s", log)
	}
}

// TestLoginUsesPinWhenNoArg proves a directory .azprofile pin is used when no
// profile arg is given.
func TestLoginUsesPinWhenNoArg(t *testing.T) {
	azLog := seedAzLoginEnv(t, map[string]string{
		"work": "AZ_TENANT=work.example.com\n",
		"emu":  "AZ_TENANT=emu.example.com\n",
	})
	t.Setenv("AZ_ACCT", acctJSON("emu.example.com", "simon"))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".azprofile"), []byte("emu\n"), 0o644)
	chdir(t, dir)

	out, err := execRoot(t, "login")
	if err != nil {
		t.Fatalf("pinned login should succeed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "profile=emu tenant=emu.example.com") {
		t.Fatalf("pin should resolve to emu:\n%s", out)
	}
	if log, _ := os.ReadFile(azLog); !strings.Contains(string(log), "--tenant emu.example.com") {
		t.Fatalf("az login did not target the pinned tenant:\n%s", log)
	}
}
