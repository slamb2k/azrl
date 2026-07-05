package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/browserpick"
)

func seedAwsHome(t *testing.T, confBody string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	confPath := filepath.Join(ap, "work.conf")
	os.WriteFile(confPath, []byte(confBody), 0o644)
	return confPath
}

func TestBrowserActionListedOnProviderTabs(t *testing.T) {
	seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if out := nm.(awsView).View(); !strings.Contains(out, "Browser profile") {
		t.Fatalf("missing Browser profile action:\n%s", out)
	}
}

func TestBrowserProfilesMsgOpensPickerAndEnterWritesKeys(t *testing.T) {
	confPath := seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	av := nm.(awsView)
	av.providerTabView.browserFor = "work"
	msg := browserProfilesMsg{forProfile: "work", profiles: []browserpick.Profile{
		{Browser: "edge", OS: "linux", Dir: "Profile 2", Name: "Work", Email: "simon@acme.com"},
	}}
	nm2, _ := av.Update(msg)
	out := nm2.(awsView).View()
	if !strings.Contains(out, "BROWSER PROFILE") || !strings.Contains(out, "Edge — Work") {
		t.Fatalf("picker not rendered:\n%s", out)
	}
	nm3, _ := nm2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), `AWS_BROWSER_CMD=microsoft-edge --profile-directory="Profile 2"`) ||
		!strings.Contains(string(b), "AWS_BROWSER_LABEL=Edge — Work") {
		t.Fatalf("keys not written:\n%s", b)
	}
	if strings.Contains(nm3.(awsView).View(), "BROWSER PROFILE") {
		t.Fatal("picker should close after enter")
	}
}

func TestBrowserDiscoveryFailureFallsBackToManualInput(t *testing.T) {
	confPath := seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	av := nm.(awsView)
	av.providerTabView.browserFor = "work"
	nm2, _ := av.Update(browserProfilesMsg{forProfile: "work", err: os.ErrDeadlineExceeded})
	out := nm2.(awsView).View()
	if !strings.Contains(out, "Browser command") {
		t.Fatalf("manual fallback prompt not shown:\n%s", out)
	}
	av2 := nm2.(awsView)
	av2.providerTabView.browserInput.SetValue("my-browser --foo")
	nm3, _ := av2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = nm3
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), "AWS_BROWSER_CMD=my-browser --foo") {
		t.Fatalf("manual command not written:\n%s", b)
	}
}

func TestDetailsShowsBrowserLabel(t *testing.T) {
	seedAwsHome(t, "AWS_SSO_START_URL=https://acme.awsapps.com/start\nAWS_BROWSER_LABEL=Edge — Work\n")
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if out := nm.(awsView).View(); !strings.Contains(out, "Edge — Work") {
		t.Fatalf("DETAILS missing browser label:\n%s", out)
	}
}
