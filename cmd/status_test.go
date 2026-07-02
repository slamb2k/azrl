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
	for _, k := range []string{"AZURE_CONFIG_DIR", "GH_CONFIG_DIR", "AWS_CONFIG_FILE", "AWS_PROFILE", "CLOUDSDK_CONFIG", "CLOUDSDK_ACTIVE_CONFIG_NAME"} {
		t.Setenv(k, "")
	}
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

func TestStatusCmdPlainSections(t *testing.T) {
	seedStatusHome(t)
	statusJSON = false
	out := runRoot(t, "status")
	for _, want := range []string{"MAPPINGS", "AMBIENT", "UNMAPPED PROFILES",
		"azure:acme", "u@acme.com", "github:work"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}
}

func TestStatusCmdPlainShowsMappingWithScope(t *testing.T) {
	seedStatusHome(t)
	home := os.Getenv("HOME")
	work := filepath.Join(home, "work")
	os.MkdirAll(work, 0o755)
	os.WriteFile(filepath.Join(work, ".azprofile"), []byte("acme\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "mappings"),
		[]byte(work+"\tacme\tpointer\n"), 0o644)
	t.Chdir(work)
	statusJSON = false
	out := runRoot(t, "status")
	if !strings.Contains(out, "● "+work+" → azure:acme") || !strings.Contains(out, ".azprofile") {
		t.Fatalf("mapping row with cwd marker missing:\n%s", out)
	}
	// Mapped profiles leave the unmapped section.
	if strings.Contains(out, "azure:acme · ") {
		t.Fatalf("mapped profile still listed as unmapped:\n%s", out)
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
	var rep statusReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("stdout is not the three-section JSON object: %v\n%s", err, out)
	}
	if len(rep.Unmapped) == 0 {
		t.Fatalf("expected unmapped rows on stdout, got none")
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
	if !strings.Contains(out, "MAPPINGS") {
		t.Fatalf("plain sections not on stdout (err buffer=%q)", errBuf.String())
	}
}

func TestStatusCmdJSON(t *testing.T) {
	seedStatusHome(t)
	home := os.Getenv("HOME")
	work := filepath.Join(home, "work")
	os.MkdirAll(work, 0o755)
	os.WriteFile(filepath.Join(work, ".azprofile"), []byte("acme\n"), 0o644)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "mappings"),
		[]byte(work+"\tacme\tpointer\n"), 0o644)
	// Point the ambient env at acme's isolated dir so the pinned cwd is not drifted.
	t.Setenv("AZURE_CONFIG_DIR", filepath.Join(home, ".azure-profiles", "acme"))
	t.Chdir(work)

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"status", "--json"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	statusJSON = false

	// The exact three-section object shape (AC-012): no extra top-level keys.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &top); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"mappings", "ambient", "unmapped"} {
		if _, ok := top[key]; !ok {
			t.Fatalf("missing %q key:\n%s", key, buf.String())
		}
	}
	if len(top) != 3 {
		t.Fatalf("want exactly 3 top-level keys, got %d:\n%s", len(top), buf.String())
	}

	var rep statusReport
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Mappings) != 1 {
		t.Fatalf("want 1 mapping, got %+v", rep.Mappings)
	}
	m := rep.Mappings[0]
	if m.Dir != work || m.Provider != "azure" || m.Profile != "acme" ||
		m.Source != "pointer" || m.Scope != "cwd" || m.Drifted {
		t.Fatalf("mapping = %+v", m)
	}
	// Ambient rows carry profile:null when unmanaged; none exist in this fixture.
	if rep.Ambient == nil {
		t.Fatalf("ambient must be an empty array, not null:\n%s", buf.String())
	}
	// acme is mapped, so only the github profile remains unmapped.
	if len(rep.Unmapped) != 1 || rep.Unmapped[0].Provider != "github" || rep.Unmapped[0].ProfileName != "work" {
		t.Fatalf("unmapped = %+v", rep.Unmapped)
	}
}

func TestStatusCmdJSONEmptySectionsAreArrays(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, k := range []string{"AZURE_CONFIG_DIR", "GH_CONFIG_DIR", "AWS_CONFIG_FILE", "AWS_PROFILE", "CLOUDSDK_CONFIG", "CLOUDSDK_ACTIVE_CONFIG_NAME"} {
		t.Setenv(k, "")
	}
	statusJSON = false
	out := runRoot(t, "status", "--json")
	statusJSON = false
	for _, want := range []string{`"mappings": []`, `"ambient": []`, `"unmapped": []`} {
		if !strings.Contains(out, want) {
			t.Fatalf("empty section not an array (%q):\n%s", want, out)
		}
	}
}
