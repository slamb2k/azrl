package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/slamb2k/azrl/internal/profile"
)

func TestUseCmd(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	chdir(t, work)
	RootCmd.SetArgs([]string{"use", "acme"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(work, ".azprofile"))
	if string(b) != "acme\n" {
		t.Fatalf("azprofile=%q", string(b))
	}
}

func TestRmCmdReservedName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	RootCmd.SetArgs([]string{"rm", "azrl", "-y"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatal("rm azrl should error")
	}
}

func TestRmCmdYes(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles", "acme"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	chdir(t, work)
	RootCmd.SetArgs([]string{"rm", "acme", "-y"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); !os.IsNotExist(err) {
		t.Fatal("conf not removed")
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func TestUnlinkCmd(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	chdir(t, work)
	RootCmd.SetArgs([]string{"use", "acme"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	RootCmd.SetArgs([]string{"unmap"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); !os.IsNotExist(err) {
		t.Fatal("unlink should remove .azprofile")
	}
}

// Unmapping a dir that still carries a provider-steering .envrc surfaces the
// direnv caution alongside the success line.
func TestUnmapWarnsAboutStaleEnvrc(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	chdir(t, work)
	RootCmd.SetArgs([]string{"use", "acme"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(work, ".envrc"), []byte(profile.EnvrcContent), 0o644)
	var out bytes.Buffer
	if err := runUnlink("azure", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "unmapped") || !strings.Contains(out.String(), "still exports this provider's env") {
		t.Fatalf("unmap output should carry the .envrc caution:\n%s", out.String())
	}
}

func TestUnlinkVerbRegisteredOnAllSurfaces(t *testing.T) {
	find := func(cmds []*cobra.Command) bool {
		for _, c := range cmds {
			if strings.HasPrefix(c.Use, "unmap") {
				return true
			}
		}
		return false
	}
	if !find(RootCmd.Commands()) || !find(githubSubcommands()) || !find(awsSubcommands()) || !find(gcpSubcommands()) {
		t.Fatal("unmap missing from a surface")
	}
}

// The pre-map-vocabulary spellings must keep parsing: `unlink` as a command
// alias, and --unlink-all / --no-link normalized to their unmap-era names.
func TestLegacyLinkSpellingsStillParse(t *testing.T) {
	c, _, err := RootCmd.Find([]string{"unmap"})
	if err != nil || c == nil || c.Use != "unmap" {
		t.Fatalf("`unlink` should resolve to the unmap command, got %v (%v)", c, err)
	}
	resetRmFlags(t)
	rm, _, _ := RootCmd.Find([]string{"rm"})
	if err := rm.Flags().Set("unmap-all", "true"); err != nil || !rmUnlinkAll {
		t.Fatalf("--unlink-all should normalize to --unmap-all: err=%v val=%v", err, rmUnlinkAll)
	}
	login, _, _ := RootCmd.Find([]string{"login"})
	if err := login.Flags().Set("no-map", "true"); err != nil {
		t.Fatalf("--no-link should normalize to --no-map: %v", err)
	}
	t.Cleanup(func() {
		if f := login.Flags().Lookup("no-map"); f != nil {
			f.Value.Set("false")
			f.Changed = false
		}
	})
}

// resetRmFlags restores the azure rm command's flag state on the RootCmd
// singleton after the test: value AND pflag's Changed bit — cobra's
// mutually-exclusive validation reads Changed, which pflag never clears
// between Execute calls, so a stale bit would fail a later bare rm.
// (Same leak class as resetAwsCaptureFlags / gh_test.go's no-link cleanup.)
func resetRmFlags(t *testing.T) {
	t.Helper()
	c, _, err := RootCmd.Find([]string{"rm"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for name, zero := range map[string]string{"yes": "false", "unmap-all": "false", "replace": ""} {
			f := c.Flags().Lookup(name)
			if f == nil {
				t.Fatalf("rm flag %q missing", name)
			}
			f.Value.Set(zero)
			f.Changed = false
		}
	})
}

func TestRmRefusesWhileLinkedThenUnlinkAll(t *testing.T) {
	resetRmFlags(t)
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	chdir(t, work)
	RootCmd.SetArgs([]string{"use", "acme"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	RootCmd.SetArgs([]string{"rm", "acme", "-y"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatal("rm must refuse while directories link to the profile")
	}
	RootCmd.SetArgs([]string{"rm", "acme", "-y", "--unmap-all"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); !os.IsNotExist(err) {
		t.Fatal("--unmap-all should remove the linked dir's pointer")
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); !os.IsNotExist(err) {
		t.Fatal("profile should be deleted")
	}
}

func TestRmUnlinkAllDeclinedConfirmKeepsLinks(t *testing.T) {
	resetRmFlags(t)
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	chdir(t, work)
	RootCmd.SetArgs([]string{"use", "acme"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.WriteString("n\n")
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	RootCmd.SetArgs([]string{"rm", "acme", "--unmap-all"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatal("declining the confirmation should abort rm")
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); err != nil {
		t.Fatal("profile should survive a declined confirmation")
	}
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); err != nil {
		t.Fatal("pointer file should survive a declined confirmation — mutation must wait for confirm")
	}
}

func TestRmUnlinkAllAndReplaceAreMutuallyExclusive(t *testing.T) {
	resetRmFlags(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	RootCmd.SetArgs([]string{"rm", "acme", "-y", "--unmap-all", "--replace", "other"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatal("--unmap-all with --replace must error, not silently pick one")
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); err != nil {
		t.Fatal("profile must survive the rejected command")
	}
}
