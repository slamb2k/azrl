package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedGcpHome points HOME at a temp dir with two GCP profiles on disk.
func seedGcpHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	gp := filepath.Join(home, ".gcp-profiles")
	os.MkdirAll(gp, 0o755)
	os.WriteFile(filepath.Join(gp, "work.conf"),
		[]byte("GCP_CONFIG_NAME=work\nGCP_PROJECT=acme-prod\nGCP_REGION=us-central1\n"), 0o644)
	os.WriteFile(filepath.Join(gp, "play.conf"),
		[]byte("GCP_CONFIG_NAME=play\nGCP_PROJECT=acme-play\n"), 0o644)
	return home
}

func TestGcpListPrintsProfiles(t *testing.T) {
	seedGcpHome(t)
	out := runRoot(t, "gcp", "list")
	if !strings.Contains(out, "work") || !strings.Contains(out, "acme-prod") ||
		!strings.Contains(out, "play") || !strings.Contains(out, "acme-play") {
		t.Fatalf("gcp list output:\n%s", out)
	}
}

func TestGcpUsePinsRepoAndSyncsConfig(t *testing.T) {
	seedGcpHome(t)
	// A gcloud shim so SyncConfig succeeds and we can assert the create call.
	bin := t.TempDir()
	gcloudLog := filepath.Join(bin, "gcloud.log")
	os.WriteFile(filepath.Join(bin, "gcloud"),
		[]byte("#!/usr/bin/env bash\necho \"$*\" >> \""+gcloudLog+"\"\nexit 0\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)
	RootCmd.SetIn(strings.NewReader("n\n")) // decline the .envrc prompt

	runRoot(t, "gcp", "use", "work")

	b, err := os.ReadFile(filepath.Join(work, ".gcpprofile"))
	if err != nil || strings.TrimSpace(string(b)) != "work" {
		t.Fatalf(".gcpprofile=%q err=%v", string(b), err)
	}
	g, _ := os.ReadFile(gcloudLog)
	if !strings.Contains(string(g), "config configurations create work") {
		t.Fatalf("gcloud configuration not synced:\n%s", g)
	}
}

func TestGcpRmRemovesProfile(t *testing.T) {
	home := seedGcpHome(t)
	runRoot(t, "gcp", "rm", "work")
	if _, err := os.Stat(filepath.Join(home, ".gcp-profiles", "work.conf")); !os.IsNotExist(err) {
		t.Fatal("work.conf not removed")
	}
}

func TestGcpCaptureWritesConf(t *testing.T) {
	home := seedGcpHome(t)
	runRoot(t, "gcp", "capture", "new", "--project", "new-proj", "--region", "europe-west1",
		"--expect-account", "simon@acme.com")
	b, err := os.ReadFile(filepath.Join(home, ".gcp-profiles", "new.conf"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, "GCP_PROJECT=new-proj") || !strings.Contains(out, "GCP_REGION=europe-west1") ||
		!strings.Contains(out, "GCP_EXPECT_ACCOUNT=simon@acme.com") || !strings.Contains(out, "GCP_CONFIG_NAME=new") {
		t.Fatalf("captured conf:\n%s", out)
	}
}

// seedGcpLoginEnv wires a full login environment for `gcp login`: a global
// azrl.conf, a gcp-profiles conf carrying an expect-account, and gcloud/ssh/cap
// shims on PATH. The gcloud shim answers `auth login` by driving the BROWSER
// bridge and `auth list` with $GCP_ACCOUNT. It returns the gcloud log path and
// the profile's conf path.
func seedGcpLoginEnv(t *testing.T, confBody string) (gcloudLog, confPath string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(az, 0o755)
	os.WriteFile(filepath.Join(az, "azrl.conf"),
		[]byte("LOCAL_HOST=pc\nLOCAL_BROWSER_CMD=wslview\nVM_HOST=vm-always\n"), 0o644)

	gp := filepath.Join(home, ".gcp-profiles")
	os.MkdirAll(gp, 0o755)
	confPath = filepath.Join(gp, "work.conf")
	os.WriteFile(confPath, []byte(confBody), 0o644)

	bin := t.TempDir()
	gcloudLog = filepath.Join(bin, "gcloud.log")
	sshLog := filepath.Join(bin, "ssh.log")
	gcloudScript := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"" + gcloudLog + "\"\n" +
		"if [[ \"$1 $2\" == \"auth list\" ]]; then printf '%s' \"$GCP_ACCOUNT\"; exit 0; fi\n" +
		"if [[ \"$1 $2\" == \"auth login\" ]]; then\n" +
		"  url='https://accounts.google.com/o/oauth2/auth?redirect_uri=http%3A%2F%2Flocalhost%3A8085%2F'\n" +
		"  cmd=\"${BROWSER/\\%s/$url}\"; eval \"$cmd\"; sleep 1; exit 0\n" +
		"fi\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "gcloud"), []byte(gcloudScript), 0o755)
	sshScript := "#!/usr/bin/env bash\necho \"$*\" >> \"" + sshLog + "\"\n" +
		"for a in \"$@\"; do [[ \"$a\" == \"-R\" ]] && exit 1; done\nexit 0\n"
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(sshScript), 0o755)
	capPath := filepath.Join(bin, "cap")
	os.WriteFile(capPath, []byte("#!/usr/bin/env bash\nprintf '%s' \"$1\" > \"$AZRL_CAPFILE\"\n"), 0o755)

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZRL_CAPTURE", capPath)
	return gcloudLog, confPath
}

func TestGcpLoginAssertsMatchingAccountAndTouches(t *testing.T) {
	confBody := "GCP_CONFIG_NAME=work\nGCP_PROJECT=acme-prod\nGCP_EXPECT_ACCOUNT=simon@acme.com\n"
	gcloudLog, confPath := seedGcpLoginEnv(t, confBody)
	t.Setenv("GCP_ACCOUNT", "simon@acme.com")

	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	if err := runRootErr(t, "gcp", "login", "work"); err != nil {
		t.Fatalf("matching account login should succeed: %v", err)
	}
	if b, _ := os.ReadFile(gcloudLog); !strings.Contains(string(b), "auth list") {
		t.Fatalf("guardrail did not call auth list:\n%s", b)
	}
	if b, _ := os.ReadFile(confPath); !strings.Contains(string(b), "LAST_USED") {
		t.Fatalf("Touch did not run after a successful assert:\n%s", b)
	}
}

func TestGcpLoginRejectsWrongAccountAndSkipsTouch(t *testing.T) {
	confBody := "GCP_CONFIG_NAME=work\nGCP_PROJECT=acme-prod\nGCP_EXPECT_ACCOUNT=simon@acme.com\n"
	_, confPath := seedGcpLoginEnv(t, confBody)
	t.Setenv("GCP_ACCOUNT", "intruder@evil.com")

	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	err := runRootErr(t, "gcp", "login", "work")
	if err == nil {
		t.Fatal("login into the wrong account should fail")
	}
	if !strings.Contains(err.Error(), "ACCOUNT MISMATCH") {
		t.Fatalf("expected an account-mismatch error, got: %v", err)
	}
	if b, _ := os.ReadFile(confPath); strings.Contains(string(b), "LAST_USED") {
		t.Fatalf("Touch must NOT run when the account assertion fails:\n%s", b)
	}
}

// TestGcpLoginIsolateEmitsGKEWarning pins FIX 3: `gcp login --isolate` must
// surface the GKE isolation footgun just like `gcp use` does when GKE is detected.
func TestGcpLoginIsolateEmitsGKEWarning(t *testing.T) {
	confBody := "GCP_CONFIG_NAME=work\nGCP_PROJECT=acme-prod\n"
	seedGcpLoginEnv(t, confBody)
	// Force the GKE signal via a kubeconfig context the pure detector recognises.
	kube := filepath.Join(t.TempDir(), "config")
	os.WriteFile(kube, []byte("contexts:\n- name: gke_acme_us_cluster\n"), 0o644)
	t.Setenv("KUBECONFIG", kube)

	work := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(old)

	out := runRoot(t, "gcp", "login", "work", "--isolate")
	if !strings.Contains(out, "GKE detected") {
		t.Fatalf("login --isolate did not surface the GKE warning:\n%s", out)
	}
}

func TestValidGcpNameRejectsReserved(t *testing.T) {
	if err := validGcpName("gcp"); err == nil {
		t.Fatal("expected the reserved name 'gcp' to be rejected")
	}
	if err := validGcpName(""); err == nil {
		t.Fatal("expected an empty name to be rejected")
	}
}
