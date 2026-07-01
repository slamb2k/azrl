package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedStatusHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	az := filepath.Join(home, ".azure-profiles")
	os.MkdirAll(filepath.Join(az, "acme"), 0o755)
	os.WriteFile(filepath.Join(az, "acme.conf"),
		[]byte("AZ_TENANT=acme.com\nLAST_USED=2026-06-30T10:00:00Z\nLAST_DIR=/work/acme\n"), 0o644)
	os.WriteFile(filepath.Join(az, "acme", "azureProfile.json"),
		[]byte(`{"subscriptions":[{"user":{"name":"u@acme.com"},"isDefault":true,"tenantId":"g1"}]}`), 0o644)
	gh := filepath.Join(home, ".github-profiles")
	os.MkdirAll(gh, 0o755)
	os.WriteFile(filepath.Join(gh, "work.conf"), []byte("GH_HOST=github.com\n"), 0o644)
}

func TestStatusCmdPlainTable(t *testing.T) {
	seedStatusHome(t)
	statusJSON = false
	out := runRoot(t, "status")
	for _, want := range []string{"PROVIDER", "Azure", "acme", "u@acme.com", "GitHub", "work"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}
}

// captureRealStdout runs fn with the real os.Stdout redirected to a pipe and
// returns what fn wrote to it. Cobra's OutOrStderr() returns the injected
// outWriter when SetOut is used, which would mask a write to stderr; capturing
// the real file descriptor proves output truly lands on stdout.
func captureRealStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestStatusCmdJSONWritesToStdout(t *testing.T) {
	seedStatusHome(t)
	errBuf := new(bytes.Buffer)
	out := captureRealStdout(t, func() {
		RootCmd.SetOut(nil) // clear any injected outWriter so OutOrStdout hits os.Stdout
		RootCmd.SetErr(errBuf)
		RootCmd.SetArgs([]string{"status", "--json"})
		if err := RootCmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	statusJSON = false
	if out == "" {
		t.Fatalf("status --json wrote nothing to stdout (err buffer=%q)", errBuf.String())
	}
	var rows []statusRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\n%s", err, out)
	}
	if len(rows) == 0 {
		t.Fatalf("expected rows on stdout, got none")
	}
}

func TestStatusCmdPlainTableWritesToStdout(t *testing.T) {
	seedStatusHome(t)
	statusJSON = false
	errBuf := new(bytes.Buffer)
	out := captureRealStdout(t, func() {
		RootCmd.SetOut(nil)
		RootCmd.SetErr(errBuf)
		RootCmd.SetArgs([]string{"status"})
		if err := RootCmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "PROVIDER") {
		t.Fatalf("plain table not on stdout (err buffer=%q)", errBuf.String())
	}
}

func TestStatusCmdJSON(t *testing.T) {
	seedStatusHome(t)
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"status", "--json"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	statusJSON = false
	var rows []statusRow
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	var azure, github bool
	for _, r := range rows {
		if r.Provider == "Azure" && r.ProfileName == "acme" && r.Identity == "u@acme.com" {
			azure = true
		}
		if r.Provider == "GitHub" && r.ProfileName == "work" {
			github = true
		}
	}
	if !azure || !github {
		t.Fatalf("rows missing a provider: %+v", rows)
	}
}
