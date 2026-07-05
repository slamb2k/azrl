package github

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfWriteAndLoad(t *testing.T) {
	dir := t.TempDir()
	c := Conf{Host: "github.com", User: "octocat", Label: "Work", Protocol: "https", BrowserCmd: "chrome-work", BrowserLabel: "Edge — Work"}
	path := filepath.Join(dir, "work.conf")
	if err := c.Write(path); err != nil {
		t.Fatal(err)
	}
	rd, err := LoadConf("work", dir)
	if err != nil {
		t.Fatal(err)
	}
	if rd.Host != "github.com" || rd.User != "octocat" || rd.Label != "Work" || rd.Protocol != "https" || rd.BrowserCmd != "chrome-work" || rd.BrowserLabel != "Edge — Work" {
		t.Fatalf("roundtrip got %+v", rd)
	}
}

func TestLoadConfRequiresHost(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.conf"), []byte("GH_USER=octocat\n"), 0o644)
	if _, err := LoadConf("bad", dir); err == nil {
		t.Fatal("expected error for missing GH_HOST")
	}
}

func TestConfDefaultsProtocolToHTTPS(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "p.conf"), []byte("GH_HOST=github.com\n"), 0o644)
	c, err := LoadConf("p", dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Protocol != "https" {
		t.Fatalf("protocol default: %q", c.Protocol)
	}
}
