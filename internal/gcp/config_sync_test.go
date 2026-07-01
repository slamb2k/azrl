package gcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedGcloudShim installs a fake gcloud on PATH that logs its args to gcloudLog.
// When body is non-empty it is spliced in to customise create's behaviour.
func seedGcloudShim(t *testing.T, createBody string) (gcloudLog string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := t.TempDir()
	gcloudLog = filepath.Join(bin, "gcloud.log")
	script := "#!/usr/bin/env bash\necho \"$*\" >> \"" + gcloudLog + "\"\n" + createBody + "exit 0\n"
	os.WriteFile(filepath.Join(bin, "gcloud"), []byte(script), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return gcloudLog
}

func TestSyncConfigCreatesAndBinds(t *testing.T) {
	gcloudLog := seedGcloudShim(t, "")
	c := Conf{ConfigName: "work", Project: "acme-prod", Region: "us-central1"}
	if err := SyncConfig("work", c, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(gcloudLog)
	out := string(b)
	for _, want := range []string{
		"config configurations create work --no-activate",
		"config set project acme-prod --configuration work",
		"config set compute/region us-central1 --configuration work",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in gcloud calls:\n%s", want, out)
		}
	}
}

func TestSyncConfigIgnoresAlreadyExists(t *testing.T) {
	// create exits 1 with an "already exists" message; SyncConfig must not error.
	createBody := "if [[ \"$1 $2 $3\" == \"config configurations create\" ]]; then echo 'ERROR: Configuration [work] already exists' >&2; exit 1; fi\n"
	gcloudLog := seedGcloudShim(t, createBody)
	c := Conf{ConfigName: "work", Project: "acme-prod"}
	if err := SyncConfig("work", c, false); err != nil {
		t.Fatalf("already-exists must be idempotent, got: %v", err)
	}
	b, _ := os.ReadFile(gcloudLog)
	if !strings.Contains(string(b), "config set project acme-prod") {
		t.Fatalf("still binds project after an existing config:\n%s", b)
	}
}

func TestSyncConfigIsolateScopesConfigDir(t *testing.T) {
	// The fake records CLOUDSDK_CONFIG so we can assert it was scoped.
	gcloudLog := seedGcloudShim(t, "echo \"CLOUDSDK_CONFIG=$CLOUDSDK_CONFIG\" >> \"$GLOG\"\n")
	t.Setenv("GLOG", gcloudLog)
	c := Conf{ConfigName: "work", Project: "acme-prod", Isolate: true}
	if err := SyncConfig("work", c, true); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := "CLOUDSDK_CONFIG=" + filepath.Join(home, ".gcp-profiles", "work")
	b, _ := os.ReadFile(gcloudLog)
	if !strings.Contains(string(b), want) {
		t.Fatalf("isolate did not scope CLOUDSDK_CONFIG (%s):\n%s", want, b)
	}
}
