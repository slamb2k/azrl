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

// The superseded read verbs are gone after their one-release deprecation:
// each is an unknown command (exit 2), and its replacement covers it.
func TestRemovedReadVerbsAreGone(t *testing.T) {
	seedAgentHome(t)
	for _, args := range [][]string{
		{"whoami", "--all"},
		{"gh", "list"},
		{"aws", "status"},
		{"gcp", "list", "--json"},
	} {
		if _, _, code := runAgent(t, args...); code != ExitUsage {
			t.Fatalf("%v should be gone (exit 2), got %d", args, code)
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

// login --json prints one result document on stdout; the bridge, sign-in and
// mapping chatter all stay on stderr.
func TestLoginJSONKeepsStdoutClean(t *testing.T) {
	seedGcpLoginEnv(t, "GCP_CONFIG_NAME=work\nGCP_PROJECT=acme-prod\n")
	chdirClean(t)
	stdout, stderr, code := runAgent(t, "gcp", "login", "fresh", "--yes", "--no-envrc", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d (%s)", code, stderr)
	}
	var res loginResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%q", err, stdout)
	}
	if res.Provider != "gcp" || res.Profile != "fresh" || !res.Created {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(stderr, `created profile "fresh"`) {
		t.Fatalf("progress should be on stderr: %q", stderr)
	}

	confBody := "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_SSO_REGION=us-east-1\n" +
		"AWS_ACCOUNT_ID=123456789012\nAWS_ROLE_NAME=AdminAccess\n"
	seedAwsLoginEnv(t, confBody)
	t.Setenv("STS_JSON", `{"Account":"123456789012","Arn":"arn:aws:sts::123456789012:assumed-role/AWSReservedSSO_AdminAccess_a1b2c3d4/simon"}`)
	stdout, stderr, code = runAgent(t, "aws", "login", "work", "--json")
	if code != ExitOK {
		t.Fatalf("aws: exit %d (%s)", code, stderr)
	}
	res = loginResult{}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || res.Profile != "work" || res.Created {
		t.Fatalf("aws login --json: %v %+v\n%q", err, res, stdout)
	}
}
