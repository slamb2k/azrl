package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestListFiltersByProvider(t *testing.T) {
	seedAgentHome(t)
	for _, spelling := range []string{"gh", "github", "GitHub"} {
		stdout, stderr, code := runAgent(t, "list", spelling, "--json")
		if code != ExitOK {
			t.Fatalf("list %s: exit %d (%s)", spelling, code, stderr)
		}
		var rep profilesReport
		if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
			t.Fatalf("list %s --json: %v\n%s", spelling, err, stdout)
		}
		if len(rep.Unmapped) != 1 || rep.Unmapped[0].Provider != "github" {
			t.Fatalf("list %s should show only the github profile: %+v", spelling, rep.Unmapped)
		}
	}
	stdout, _, code := runAgent(t, "list", "--json")
	var all profilesReport
	if code != ExitOK || json.Unmarshal([]byte(stdout), &all) != nil || len(all.Unmapped) != 5 {
		t.Fatalf("bare list should cover every provider: exit %d\n%s", code, stdout)
	}
}

func TestListAndWhoamiRejectUnknownProvider(t *testing.T) {
	seedAgentHome(t)
	for _, verb := range []string{"list", "whoami"} {
		_, stderr, code := runAgent(t, verb, "nope")
		if code != ExitUsage || !strings.Contains(stderr, "unknown provider") {
			t.Fatalf("%s nope: exit %d, stderr %q", verb, code, stderr)
		}
	}
}

func TestWhoamiFiltersByProvider(t *testing.T) {
	seedAgentHome(t)
	stdout, stderr, code := runAgent(t, "whoami", "aws", "--json")
	if code != ExitOK {
		t.Fatalf("whoami aws: exit %d (%s)", code, stderr)
	}
	var rep whoamiReport
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("whoami aws --json: %v\n%s", err, stdout)
	}
	if len(rep.Providers) != 1 || rep.Providers[0].Provider != "aws" {
		t.Fatalf("whoami aws should report only aws: %+v", rep.Providers)
	}
}

// The superseded read verbs keep working for a release, with their notice on
// stderr so a --json document on stdout stays parseable.
func TestDeprecatedReadVerbsStillWork(t *testing.T) {
	seedAgentHome(t)
	for _, args := range [][]string{
		{"whoami", "--all", "--json"},
		{"gh", "list", "--json"},
		{"aws", "status", "--json"},
	} {
		stdout, stderr, code := runAgent(t, args...)
		if code != ExitOK {
			t.Fatalf("%v: exit %d (%s)", args, code, stderr)
		}
		if !json.Valid([]byte(stdout)) {
			t.Fatalf("%v: stdout is not one JSON document:\n%s", args, stdout)
		}
		if !strings.Contains(stderr, "deprecated") {
			t.Fatalf("%v: missing deprecation notice on stderr: %q", args, stderr)
		}
	}
}

func TestAzGroupAndHiddenAliases(t *testing.T) {
	az, _, err := RootCmd.Find([]string{"az"})
	if err != nil || az.Name() != "az" {
		t.Fatalf("az group missing: %v", err)
	}
	want := []string{"login", "use", "rm", "capture", "browser", "shell", "console", "unmap", "env", "default"}
	for _, verb := range want {
		if c, _, err := RootCmd.Find([]string{"az", verb}); err != nil || c.Name() != verb {
			t.Fatalf("az %s missing: %v", verb, err)
		}
		c, _, err := RootCmd.Find([]string{verb})
		if err != nil || c.Name() != verb || !c.Hidden {
			t.Fatalf("top-level %s should be a hidden alias (err=%v)", verb, err)
		}
	}
	if c, _, _ := RootCmd.Find([]string{"azure", "login"}); c.Name() != "login" {
		t.Fatal("`azure` should alias the az group")
	}
}

func TestRootHelpShowsTheGrid(t *testing.T) {
	groupCommands(RootCmd)
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"--help"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := buf.String()
	for _, want := range []string{"whoami", "list", "setup", "az ", "gh ", "aws ", "gcp "} {
		if !strings.Contains(help, want) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "\n  login ") || strings.Contains(help, "\n  capture ") {
		t.Fatalf("Azure verbs should live under az, not the root help:\n%s", help)
	}
}

func TestGhrlScopesReadVerbsToGithub(t *testing.T) {
	seedAgentHome(t)
	root := GhrlRoot()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"whoami", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("ghrl whoami: %v (%s)", err, errb.String())
	}
	var rep whoamiReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || len(rep.Providers) != 1 || rep.Providers[0].Provider != "github" {
		t.Fatalf("ghrl whoami should report github only: %v\n%s", err, out.String())
	}
}

// Mutating verbs report progress on stderr only: stdout is reserved for data
// (list, whoami, env, --json, --print-url), so `x=$(azrl …)` never captures
// chatter.
func TestMutatingVerbsKeepStdoutClean(t *testing.T) {
	for _, s := range surfaces {
		seedAgentHome(t)
		t.Setenv("PATH", t.TempDir()) // no native CLIs: config syncs are skipped
		for _, args := range [][]string{
			{"use", "work", "--no-envrc"},
			{"unmap"},
			{"browser", "work", "--cmd", "chrome", "--label", "Work"},
			{"browser", "work", "--clear"},
			{"rm", "work", "--yes"},
		} {
			full := argv(s.prefix, args...)
			stdout, stderr, code := runAgent(t, full...)
			if code != ExitOK {
				t.Fatalf("%v: exit %d (%s)", full, code, stderr)
			}
			if stdout != "" {
				t.Fatalf("%v wrote progress to stdout:\n%s", full, stdout)
			}
		}
	}
}
