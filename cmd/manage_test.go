package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestListCmd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "fiig.conf"), []byte("AZ_TENANT=fiig.com.au\n"), 0o644)
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetArgs([]string{"list"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("fiig")) {
		t.Fatalf("list output: %s", buf.String())
	}
}

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
	RootCmd.SetArgs([]string{"unlink"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); !os.IsNotExist(err) {
		t.Fatal("unlink should remove .azprofile")
	}
}

func TestUnlinkVerbRegisteredOnAllSurfaces(t *testing.T) {
	find := func(cmds []*cobra.Command) bool {
		for _, c := range cmds {
			if strings.HasPrefix(c.Use, "unlink") {
				return true
			}
		}
		return false
	}
	if !find(RootCmd.Commands()) || !find(githubSubcommands()) || !find(awsSubcommands()) || !find(gcpSubcommands()) {
		t.Fatal("unlink missing from a surface")
	}
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
		for name, zero := range map[string]string{"yes": "false", "unlink-all": "false", "replace": ""} {
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
	RootCmd.SetArgs([]string{"rm", "acme", "-y", "--unlink-all"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, ".azprofile")); !os.IsNotExist(err) {
		t.Fatal("--unlink-all should remove the linked dir's pointer")
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); !os.IsNotExist(err) {
		t.Fatal("profile should be deleted")
	}
}

func TestRmUnlinkAllAndReplaceAreMutuallyExclusive(t *testing.T) {
	resetRmFlags(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	RootCmd.SetArgs([]string{"rm", "acme", "-y", "--unlink-all", "--replace", "other"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatal("--unlink-all with --replace must error, not silently pick one")
	}
	if _, err := os.Stat(filepath.Join(home, ".azure-profiles", "acme.conf")); err != nil {
		t.Fatal("profile must survive the rejected command")
	}
}
