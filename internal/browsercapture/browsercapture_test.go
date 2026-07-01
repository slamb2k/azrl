package browsercapture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureWritesURL(t *testing.T) {
	dir := t.TempDir()
	capfile := filepath.Join(dir, "cap")
	url := "https://login/x?redirect_uri=http://localhost:44444/"
	if err := Capture(capfile, url); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(capfile)
	if err != nil || string(b) != url {
		t.Fatalf("capfile=%q err=%v", string(b), err)
	}
}

func TestCaptureEmptyCapfileErrors(t *testing.T) {
	if err := Capture("", "https://login/x"); err == nil {
		t.Fatal("expected error when capfile is empty")
	}
}
