package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
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
