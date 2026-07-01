package aws

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvrcContent(t *testing.T) {
	if got := EnvrcContent("work", false); got != "export AWS_PROFILE=work\n" {
		t.Fatalf("shared content = %q", got)
	}
	iso := EnvrcContent("work", true)
	if !strings.Contains(iso, `export AWS_CONFIG_FILE="$HOME/.aws-profiles/work/config"`) ||
		!strings.Contains(iso, `export AWS_SHARED_CREDENTIALS_FILE="$HOME/.aws-profiles/work/credentials"`) {
		t.Fatalf("isolate content = %q", iso)
	}
	if strings.Contains(iso, "AWS_PROFILE=") {
		t.Fatalf("isolate content should not set AWS_PROFILE: %q", iso)
	}
}

func TestWriteEnvrcRefusesToClobber(t *testing.T) {
	pwd := t.TempDir()
	wrote, err := WriteEnvrc(pwd, "work", false)
	if err != nil || !wrote {
		t.Fatalf("first write: wrote=%v err=%v", wrote, err)
	}
	b, _ := os.ReadFile(filepath.Join(pwd, ".envrc"))
	if string(b) != "export AWS_PROFILE=work\n" {
		t.Fatalf(".envrc = %q", b)
	}
	wrote, err = WriteEnvrc(pwd, "other", false)
	if err != nil || wrote {
		t.Fatalf("second write should be a no-op: wrote=%v err=%v", wrote, err)
	}
	b, _ = os.ReadFile(filepath.Join(pwd, ".envrc"))
	if string(b) != "export AWS_PROFILE=work\n" {
		t.Fatalf(".envrc was clobbered: %q", b)
	}
}
