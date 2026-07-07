package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestShellActionListedAndDispatches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	av := nm.(awsView)
	if !strings.Contains(av.View(), "Shell as") {
		t.Fatalf("t Shell as… missing from actions:\n%s", av.View())
	}
	_, cmd := av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if cmd == nil {
		t.Fatal("t on a selected profile should hand off to azrl shell")
	}
}

func TestShellOverrideChipInHeader(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	t.Setenv("AZRL_PROFILE", "aws:prod")
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	av := nm.(awsView)
	av.reload()
	if !strings.Contains(av.View(), "⌁ shell: prod") {
		t.Fatalf("shell override chip missing:\n%s", av.View())
	}
}

func TestForeignShellOverrideIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	t.Setenv("AZRL_PROFILE", "gcp:lab")
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	av := nm.(awsView)
	av.reload()
	if strings.Contains(av.View(), "⌁ shell:") {
		t.Fatalf("foreign provider override must not show a chip:\n%s", av.View())
	}
}
