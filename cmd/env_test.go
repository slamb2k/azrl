package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func runRootSplit(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	outBuf, errBuf := new(bytes.Buffer), new(bytes.Buffer)
	RootCmd.SetOut(outBuf)
	RootCmd.SetErr(errBuf)
	RootCmd.SetIn(strings.NewReader(stdin))
	// The env command is shared across tests in-process; reset its sticky flag.
	if envCmd, _, ferr := RootCmd.Find([]string{"env"}); ferr == nil {
		_ = envCmd.Flags().Set("off", "false")
	}
	RootCmd.SetArgs(args)
	err := RootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestEnvPrintsExports(t *testing.T) {
	home := seedConsoleHome(t)
	out, errOut, err := runRootSplit(t, "", "env", "work")
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	for _, want := range []string{
		"export AZURE_CONFIG_DIR='" + filepath.Join(home, ".azure-profiles", "work") + "'",
		"export AZRL_PROFILE='azure:work'",
		"export AZRL_BROWSER_CMD='chrome --profile-directory=Work'",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in stdout:\n%s", want, out)
		}
	}
	if !strings.Contains(errOut, "acts as azure:work") {
		t.Fatalf("stderr note missing:\n%s", errOut)
	}
}

func TestEnvOffPrintsUnsets(t *testing.T) {
	seedConsoleHome(t)
	out, _, err := runRootSplit(t, "", "env", "--off")
	if err != nil {
		t.Fatalf("env --off: %v", err)
	}
	if !strings.Contains(out, "unset AZRL_BROWSER_CMD AZRL_PROFILE AZURE_CONFIG_DIR") {
		t.Fatalf("unsets missing:\n%s", out)
	}
}

// TestEnvPickerKeepsStdoutPure is the eval-safety guarantee: with multiple
// profiles and an interactive pick, stdout carries ONLY shell code — the menu
// and prompt live on stderr.
func TestEnvPickerKeepsStdoutPure(t *testing.T) {
	seedConsoleHome(t)
	orig := isInteractive
	isInteractive = func() bool { return true }
	t.Cleanup(func() { isInteractive = orig })

	out, errOut, err := runRootSplit(t, "1\n", "env")
	if err != nil {
		t.Fatalf("env picker: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if !strings.HasPrefix(line, "export ") {
			t.Fatalf("stdout must be pure shell code, got line %q in:\n%s", line, out)
		}
	}
	if !strings.Contains(errOut, "Select a profile") {
		t.Fatalf("picker menu should be on stderr:\n%s", errOut)
	}
}

func TestEnvNoProfilesErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, _, err := runRootSplit(t, "", "env")
	if err == nil || !strings.Contains(err.Error(), "no profiles yet") {
		t.Fatalf("want no-profiles guidance, got %v", err)
	}
}

// TestPrintApplyHint proves the post-login hint fires only when the profile
// does not govern the cwd, and stays silent when it does.
func TestPrintApplyHint(t *testing.T) {
	home := seedConsoleHome(t)
	t.Setenv("AZURE_CONFIG_DIR", "")
	t.Setenv("AZRL_PROFILE", "")
	c := &cobra.Command{}
	buf := new(bytes.Buffer)
	c.SetOut(buf)

	printApplyHint(c, "azrl", "azure", "work")
	if !strings.Contains(buf.String(), "apply with") {
		t.Fatalf("hint expected when nothing governs:\n%s", buf.String())
	}

	// Pointer makes the profile govern: hint suppressed.
	repo := filepath.Join(home, "repo")
	os.MkdirAll(repo, 0o755)
	os.WriteFile(filepath.Join(repo, ".azprofile"), []byte("work\n"), 0o644)
	t.Chdir(repo)
	buf.Reset()
	printApplyHint(c, "azrl", "azure", "work")
	if buf.String() != "" {
		t.Fatalf("hint must be silent when the profile governs:\n%s", buf.String())
	}
}
