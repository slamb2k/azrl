package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestConsoleActionListedAndDispatches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	nm, _ = nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyDown}) // key nav marks the pane visited; the first profile is already selected
	av := nm.(awsView)
	if !strings.Contains(av.View(), "Open console") {
		t.Fatalf("c Open console missing from actions:\n%s", av.View())
	}
	_, cmd := av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd == nil {
		t.Fatal("c on a selected profile should hand off to azrl console")
	}
}
