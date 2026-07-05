package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/envdetect"
)

func send(m tea.Model, msg tea.Msg) setupModel {
	nm, _ := m.Update(msg)
	return nm.(setupModel)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// TestSetupLocalSingleCandidate: one local candidate skips the pick step, lands
// on the field form pre-filled, and confirms into the expected Global.
func TestSetupLocalSingleCandidate(t *testing.T) {
	cands := envdetect.Detect(envdetect.Env{WSLDistro: "Ubuntu", GOOS: "linux", Has: func(string) bool { return false }})
	m := newSetupModel(cands)
	if m.step != stepFields {
		t.Fatalf("single candidate should skip pick, step=%v", m.step)
	}
	if !strings.Contains(m.View(), "wslview") {
		t.Fatalf("field view should pre-fill wslview:\n%s", m.View())
	}
	m = send(m, key("enter")) // last field → confirm
	if m.step != stepConfirm {
		t.Fatalf("enter on last field should confirm, step=%v", m.step)
	}
	m = send(m, key("y"))
	g, ok := m.Result()
	if !ok || g.BrowserCmd != "wslview" || !g.IsLocal() {
		t.Fatalf("confirm result = %+v ok=%v", g, ok)
	}
}

// TestSetupRemoteFlow: pick remote → OS pick → edit VM host → confirm.
func TestSetupRemoteFlow(t *testing.T) {
	cands := []envdetect.Candidate{
		{Mode: envdetect.Remote, Label: "Remote", VMSSHHost: "203.0.113.10", Recommended: true},
		{Mode: envdetect.Local, Label: "Local", BrowserCmd: "wslview", BrowserHost: "localhost"},
	}
	m := newSetupModel(cands)
	if m.step != stepPick {
		t.Fatalf("two candidates should start at pick, step=%v", m.step)
	}
	m = send(m, key("enter")) // choose remote (cursor 0)
	if m.step != stepOS {
		t.Fatalf("remote choice should enter OS pick, step=%v", m.step)
	}
	m = send(m, key("down")) // WSL (index 1)
	m = send(m, key("enter"))
	if m.step != stepFields {
		t.Fatalf("OS pick should enter fields, step=%v", m.step)
	}
	if m.chosen.BrowserCmd != "wslview" {
		t.Fatalf("OS pick should set BrowserCmd=wslview, got %q", m.chosen.BrowserCmd)
	}
	// VM_SSH_HOST field pre-filled from the candidate.
	if !strings.Contains(m.View(), "203.0.113.10") {
		t.Fatalf("VM host should be pre-filled:\n%s", m.View())
	}
	m = send(m, key("enter")) // VM_SSH_HOST → BROWSER_HOST
	m = send(m, key("enter")) // BROWSER_HOST (last) → confirm
	if m.step != stepConfirm {
		t.Fatalf("should reach confirm, step=%v", m.step)
	}
	m = send(m, key("enter")) // write
	g, ok := m.Result()
	if !ok || g.IsLocal() || g.BrowserCmd != "wslview" || g.VMSSHHost != "203.0.113.10" {
		t.Fatalf("remote result = %+v ok=%v", g, ok)
	}
}

// TestSetupViewChrome checks the visual scaffolding: the winged banner, the
// stage breadcrumb, and the mode badge all render across steps.
func TestSetupViewChrome(t *testing.T) {
	cands := []envdetect.Candidate{
		{Mode: envdetect.Remote, Label: "Remote", Reason: "SSH session detected", VMSSHHost: "203.0.113.10", Recommended: true},
		{Mode: envdetect.Local, Label: "Local", Reason: "WSL detected", BrowserCmd: "wslview", BrowserHost: "localhost"},
	}
	m := newSetupModel(cands)
	m.width = 74
	v := m.View()
	// Banner crest (wordmark glyph), breadcrumb stages, and both mode badges.
	for _, want := range []string{"█", "Detect", "Configure", "Confirm", "LOCAL", "REMOTE"} {
		if !strings.Contains(v, want) {
			t.Fatalf("pick view missing %q", want)
		}
	}

	// Drive to confirm; the sheet shows the REMOTE badge and resolved keys.
	m = send(m, key("enter")) // remote
	m = send(m, key("down"))  // WSL OS
	m = send(m, key("enter"))
	m = send(m, key("enter")) // VM host
	m = send(m, key("enter")) // browser host → confirm
	cv := m.View()
	for _, want := range []string{"REMOTE", "BROWSER_CMD", "wslview", "azrl.conf.bak"} {
		if !strings.Contains(cv, want) {
			t.Fatalf("confirm view missing %q", want)
		}
	}
}

// TestSetupCancel: esc anywhere cancels without a confirmed result.
func TestSetupCancel(t *testing.T) {
	m := newSetupModel([]envdetect.Candidate{{Mode: envdetect.Local, BrowserCmd: "open", BrowserHost: "localhost"}})
	m = send(m, key("esc"))
	if _, ok := m.Result(); ok {
		t.Fatal("esc must not confirm")
	}
}
