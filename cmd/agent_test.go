package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/slamb2k/azrl/internal/config"
)

var tagOnce sync.Once

// noReadStdin fails the test if a command reads stdin: under the agent
// contract an unattended caller is never prompted.
type noReadStdin struct{ t *testing.T }

func (r noReadStdin) Read([]byte) (int, error) {
	r.t.Errorf("command read stdin with no TTY")
	return 0, errors.New("stdin read")
}

// resetFlags returns every flag in the tree to its default, so a flag set by
// one Execute does not leak into the next (cobra commands are singletons).
func resetFlags(c *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if f.Changed {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
	c.Flags().VisitAll(reset)
	c.PersistentFlags().VisitAll(reset)
	for _, sub := range c.Commands() {
		resetFlags(sub)
	}
}

// runAgent executes the azrl tree as an agent would: no TTY, stdout and stderr
// captured separately, and the exit code the binary would return.
func runAgent(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	tagOnce.Do(func() { tagUsageErrors(RootCmd) })
	stubInteractive(t, false)
	resetFlags(RootCmd)
	var o, e bytes.Buffer
	RootCmd.SetOut(&o)
	RootCmd.SetErr(&e)
	RootCmd.SetIn(noReadStdin{t})
	RootCmd.SetArgs(args)
	err := executeTagged(RootCmd)
	RootCmd.SetIn(nil)
	return o.String(), e.String(), ExitCode(err)
}

// seedAgentHome is a HOME with a global conf and one "work" profile per
// provider (plus a second aws profile for steal tests).
func seedAgentHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AZRL_PROFILE", "")
	seedGlobalConf(t, home)
	write := func(rel, body string) {
		p := filepath.Join(home, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".azure-profiles/work.conf", "AZ_TENANT=acme.com\n")
	write(".github-profiles/work.conf", "GH_HOST=github.com\nGH_USER=octocat\n")
	write(".aws-profiles/work.conf", "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_SSO_REGION=us-east-1\n")
	write(".aws-profiles/other.conf", "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_BROWSER_CMD=edge\n")
	write(".gcp-profiles/work.conf", "GCP_PROJECT=acme-lab\n")
	chdirClean(t)
	return home
}

// surfaces is every provider's verb prefix. Azure is driven both through its
// `az` group and through the hidden top-level aliases.
var surfaces = []struct{ provider, prefix string }{
	{"azure", "az"}, {"azure", ""}, {"github", "gh"}, {"aws", "aws"}, {"gcp", "gcp"},
}

func argv(prefix string, rest ...string) []string {
	if prefix == "" {
		return rest
	}
	return append([]string{prefix}, rest...)
}

// TestAgentContractGrid drives every verb on every provider with no TTY and
// asserts each returns the documented exit code without reading stdin.
func TestAgentContractGrid(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"rm unknown", []string{"rm", "ghost"}, ExitNotFound},
		{"rm without --yes", []string{"rm", "work"}, ExitUsage},
		{"use unknown", []string{"use", "ghost"}, ExitNotFound},
		{"use invalid name", []string{"use", ".."}, ExitUsage},
		{"browser unknown", []string{"browser", "ghost"}, ExitNotFound},
		{"browser without flags", []string{"browser", "work"}, ExitUsage},
		{"browser label without cmd", []string{"browser", "work", "--label", "x"}, ExitUsage},
		{"shell unknown", []string{"shell", "ghost", "--", "true"}, ExitNotFound},
		{"shell dead session", []string{"shell", "work", "--", "true"}, ExitNeedsHuman},
		{"shell empty after dash", []string{"shell", "work", "--"}, ExitUsage},
		{"console unknown", []string{"console", "ghost", "--print-url"}, ExitNotFound},
		{"console nothing governs", []string{"console", "--print-url"}, ExitUsage},
		{"capture invalid name", []string{"capture", ".."}, ExitUsage},
		{"unknown flag", []string{"use", "work", "--nope"}, ExitUsage},
		{"too many args", []string{"use", "a", "b"}, ExitUsage},
		{"envrc flags exclusive", []string{"use", "work", "--envrc", "--no-envrc"}, ExitUsage},
	}
	for _, s := range surfaces {
		for _, c := range cases {
			t.Run(s.provider+"/"+c.name, func(t *testing.T) {
				seedAgentHome(t)
				_, stderr, code := runAgent(t, argv(s.prefix, c.args...)...)
				if code != c.want {
					t.Fatalf("exit %d, want %d\nstderr: %s", code, c.want, stderr)
				}
			})
		}
	}
}

func TestAgentRmYesRemovesWithoutPrompt(t *testing.T) {
	for _, s := range surfaces {
		t.Run(s.provider, func(t *testing.T) {
			home := seedAgentHome(t)
			dir := map[string]string{"azure": ".azure-profiles", "github": ".github-profiles", "aws": ".aws-profiles", "gcp": ".gcp-profiles"}[s.provider]
			if _, stderr, code := runAgent(t, argv(s.prefix, "rm", "work")...); code != ExitUsage {
				t.Fatalf("rm without --yes: exit %d (stderr %s)", code, stderr)
			}
			if _, err := os.Stat(filepath.Join(home, dir, "work.conf")); err != nil {
				t.Fatal("refused rm must leave the profile in place")
			}
			if _, stderr, code := runAgent(t, argv(s.prefix, "rm", "work", "--yes")...); code != ExitOK {
				t.Fatalf("rm --yes: exit %d (stderr %s)", code, stderr)
			}
			if _, err := os.Stat(filepath.Join(home, dir, "work.conf")); !os.IsNotExist(err) {
				t.Fatal("rm --yes must remove the profile")
			}
		})
	}
}

func TestAgentBrowserFlags(t *testing.T) {
	home := seedAgentHome(t)
	conf := func(name string) string {
		b, _ := os.ReadFile(filepath.Join(home, ".aws-profiles", name+".conf"))
		return string(b)
	}
	if _, stderr, code := runAgent(t, "aws", "browser", "work", "--cmd", "chrome --profile-directory=Work", "--label", "Chrome · Work"); code != ExitOK {
		t.Fatalf("--cmd: exit %d (%s)", code, stderr)
	}
	if c := conf("work"); !strings.Contains(c, "AWS_BROWSER_CMD=chrome --profile-directory=Work") || !strings.Contains(c, "AWS_BROWSER_LABEL=Chrome · Work") {
		t.Fatalf("mapping not written:\n%s", c)
	}
	// "edge" belongs to other: without --steal it refuses, with it it moves.
	if _, _, code := runAgent(t, "aws", "browser", "work", "--cmd", "edge"); code != ExitUsage {
		t.Fatalf("taking a held browser without --steal: exit %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(conf("other"), "AWS_BROWSER_CMD=edge") {
		t.Fatal("refused steal must leave the other holder untouched")
	}
	if _, stderr, code := runAgent(t, "aws", "browser", "work", "--cmd", "edge", "--steal"); code != ExitOK {
		t.Fatalf("--steal: exit %d (%s)", code, stderr)
	}
	if strings.Contains(conf("other"), "AWS_BROWSER_CMD=edge") || !strings.Contains(conf("work"), "AWS_BROWSER_CMD=edge") {
		t.Fatalf("steal did not move the mapping:\nwork:\n%s\nother:\n%s", conf("work"), conf("other"))
	}
	if _, stderr, code := runAgent(t, "aws", "browser", "work", "--clear"); code != ExitOK {
		t.Fatalf("--clear: exit %d (%s)", code, stderr)
	}
	if strings.Contains(conf("work"), "AWS_BROWSER_CMD=edge") {
		t.Fatalf("--clear left the mapping:\n%s", conf("work"))
	}
}

func TestAgentShellRunsOneCommandAsProfile(t *testing.T) {
	home := seedAgentHome(t)
	liveAzureSession(t, home, "work")
	log := filepath.Join(t.TempDir(), "env.log")
	code := -1
	orig := shellExit
	shellExit = func(c int) { code = c }
	defer func() { shellExit = orig }()

	_, stderr, exit := runAgent(t, "shell", "work", "--", "sh", "-c", `echo "$AZURE_CONFIG_DIR|$AZRL_PROFILE" > "$0"; exit 5`, log)
	if exit != ExitOK {
		t.Fatalf("exit %d (%s)", exit, stderr)
	}
	if code != 5 {
		t.Fatalf("command exit status not passed through: %d", code)
	}
	b, _ := os.ReadFile(log)
	want := filepath.Join(home, ".azure-profiles", "work") + "|azure:work"
	if strings.TrimSpace(string(b)) != want {
		t.Fatalf("command env = %q, want %q", strings.TrimSpace(string(b)), want)
	}
}

func TestAgentShellDeadSessionNamesLoginCommand(t *testing.T) {
	seedAgentHome(t)
	called := false
	orig := shellLoginRunner
	shellLoginRunner = func(string, string) error { called = true; return nil }
	defer func() { shellLoginRunner = orig }()
	_, stderr, code := runAgent(t, "aws", "shell", "work", "--", "aws", "s3", "ls")
	if code != ExitNeedsHuman {
		t.Fatalf("exit %d, want %d", code, ExitNeedsHuman)
	}
	if called {
		t.Fatal("an unattended shell -- cmd must never start a sign-in")
	}
	if !strings.Contains(stderr, "aws login work") {
		t.Fatalf("missing login hint:\n%s", stderr)
	}
}

func TestAgentConsolePrintURL(t *testing.T) {
	seedAgentHome(t)
	opened := false
	orig := consoleOpen
	consoleOpen = func(config.Global, string) error { opened = true; return nil }
	defer func() { consoleOpen = orig }()
	stdout, stderr, code := runAgent(t, "console", "work", "--print-url")
	if code != ExitOK {
		t.Fatalf("exit %d (%s)", code, stderr)
	}
	if strings.TrimSpace(stdout) != "https://portal.azure.com/#@acme.com" {
		t.Fatalf("stdout = %q", stdout)
	}
	if opened {
		t.Fatal("--print-url must not launch a browser")
	}
}

func TestAgentListAndStatusJSON(t *testing.T) {
	seedAgentHome(t)
	if _, stderr, code := runAgent(t, "gcp", "use", "work"); code != ExitOK {
		t.Fatalf("use: exit %d (%s)", code, stderr)
	}
	stdout, stderr, code := runAgent(t, "gcp", "list", "--json")
	if code != ExitOK {
		t.Fatalf("list: exit %d (%s)", code, stderr)
	}
	var rows []listedProfile
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("list --json is not JSON: %v\n%s", err, stdout)
	}
	if len(rows) != 1 || rows[0].Name != "work" || rows[0].Detail != "acme-lab" || len(rows[0].Mapped) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	stdout, stderr, code = runAgent(t, "gcp", "status", "--json")
	if code != ExitOK {
		t.Fatalf("status: exit %d (%s)", code, stderr)
	}
	var row whoamiRow
	if err := json.Unmarshal([]byte(stdout), &row); err != nil {
		t.Fatalf("status --json is not JSON: %v\n%s", err, stdout)
	}
	if row.Provider != "gcp" || row.Profile != "work" || row.Via != "pointer" {
		t.Fatalf("row = %+v", row)
	}
}

func TestAgentCaptureJSONKeepsStdoutClean(t *testing.T) {
	seedAgentHome(t)
	shimAzCapture(t)
	stdout, stderr, code := runAgent(t, "capture", "fresh", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d (%s)", code, stderr)
	}
	var res captureResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%q", err, stdout)
	}
	if res.Provider != "azure" || res.Profile != "fresh" || !res.Created || res.Detail != "acme.onmicrosoft.com" {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(stderr, "wrote") {
		t.Fatalf("progress should go to stderr:\n%s", stderr)
	}
}

func TestAgentEnvrcFlagWritesWithoutPrompt(t *testing.T) {
	seedAgentHome(t)
	cwd, _ := os.Getwd()
	if _, stderr, code := runAgent(t, "gh", "use", "work", "--envrc"); code != ExitOK {
		t.Fatalf("exit %d (%s)", code, stderr)
	}
	b, err := os.ReadFile(filepath.Join(cwd, ".envrc"))
	if err != nil || !strings.Contains(string(b), "GH_CONFIG_DIR") {
		t.Fatalf(".envrc not written: %v %q", err, b)
	}
}

func TestAgentNoEnvrcWithoutTTY(t *testing.T) {
	seedAgentHome(t)
	cwd, _ := os.Getwd()
	if _, stderr, code := runAgent(t, "aws", "use", "work"); code != ExitOK {
		t.Fatalf("exit %d (%s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".envrc")); !os.IsNotExist(err) {
		t.Fatal("no TTY and no --envrc must not write .envrc")
	}
}
