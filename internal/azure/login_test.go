package azure

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoginCapture(t *testing.T) {
	bin := t.TempDir()
	// Fake az: emulate Python webbrowser by running $BROWSER with a URL.
	azScript := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"" + filepath.Join(bin, "az.log") + "\"\n" +
		"url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F&s=z'\n" +
		"cmd=\"${BROWSER/\\%s/$url}\"\n" +
		"eval \"$cmd\"\n" +
		"sleep 2\n"
	os.WriteFile(filepath.Join(bin, "az"), []byte(azScript), 0o755)
	// Capture shim: write the URL to $AZRL_CAPFILE.
	capShim := "#!/usr/bin/env bash\nprintf '%s' \"$1\" > \"$AZRL_CAPFILE\"\n"
	capPath := filepath.Join(bin, "cap")
	os.WriteFile(capPath, []byte(capShim), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZRL_CAPTURE", capPath)

	lg, err := LoginCapture("fiig.com.au")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lg.Cmd.Process.Kill() }()
	if lg.Port != "40404" {
		t.Fatalf("port=%q url=%q", lg.Port, lg.URL)
	}
	b, _ := os.ReadFile(filepath.Join(bin, "az.log"))
	if !contains(string(b), "--tenant") || !contains(string(b), "--allow-no-subscription") {
		t.Fatalf("az args missing flags: %s", b)
	}
}

func TestLoginCaptureNoTenantOmitsFlag(t *testing.T) {
	bin := t.TempDir()
	azScript := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"" + filepath.Join(bin, "az.log") + "\"\n" +
		"url='https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A40404%2F'\n" +
		"cmd=\"${BROWSER/\\%s/$url}\"\neval \"$cmd\"\nsleep 2\n"
	os.WriteFile(filepath.Join(bin, "az"), []byte(azScript), 0o755)
	capPath := filepath.Join(bin, "cap")
	os.WriteFile(capPath, []byte("#!/usr/bin/env bash\nprintf '%s' \"$1\" > \"$AZRL_CAPFILE\"\n"), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZRL_CAPTURE", capPath)

	lg, err := LoginCapture("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lg.Cmd.Process.Kill() }()
	b, _ := os.ReadFile(filepath.Join(bin, "az.log"))
	if contains(string(b), "--tenant") {
		t.Fatalf("--tenant should be omitted: %s", b)
	}
}
