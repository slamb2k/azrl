package gcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvrcContent(t *testing.T) {
	if got := EnvrcContent("work", false); got != "export CLOUDSDK_ACTIVE_CONFIG_NAME=work\n" {
		t.Fatalf("shared content = %q", got)
	}
	iso := EnvrcContent("work", true)
	if !strings.Contains(iso, `export CLOUDSDK_CONFIG="$HOME/.gcp-profiles/work"`) {
		t.Fatalf("isolate content = %q", iso)
	}
	if strings.Contains(iso, "CLOUDSDK_ACTIVE_CONFIG_NAME") {
		t.Fatalf("isolate content should not set CLOUDSDK_ACTIVE_CONFIG_NAME: %q", iso)
	}
}

func TestWriteEnvrcRefusesToClobber(t *testing.T) {
	pwd := t.TempDir()
	wrote, err := WriteEnvrc(pwd, "work", false)
	if err != nil || !wrote {
		t.Fatalf("first write: wrote=%v err=%v", wrote, err)
	}
	b, _ := os.ReadFile(filepath.Join(pwd, ".envrc"))
	if string(b) != "export CLOUDSDK_ACTIVE_CONFIG_NAME=work\n" {
		t.Fatalf(".envrc = %q", b)
	}
	wrote, err = WriteEnvrc(pwd, "other", false)
	if err != nil || wrote {
		t.Fatalf("second write should be a no-op: wrote=%v err=%v", wrote, err)
	}
	b, _ = os.ReadFile(filepath.Join(pwd, ".envrc"))
	if string(b) != "export CLOUDSDK_ACTIVE_CONFIG_NAME=work\n" {
		t.Fatalf(".envrc was clobbered: %q", b)
	}
}
