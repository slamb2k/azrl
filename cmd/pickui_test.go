package cmd

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPickModelArrowsAndSelect(t *testing.T) {
	m := pickModel{title: "t", items: []pickItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}, choice: -1}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyDown})
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyUp})
	nm, cmd := nm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	fm := nm.(pickModel)
	if fm.choice != 1 || cmd == nil {
		t.Fatalf("choice = %d (cmd nil=%v), want 1 selected", fm.choice, cmd == nil)
	}
}

func TestPickModelDigitJumpAndCancel(t *testing.T) {
	m := pickModel{items: []pickItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}, choice: -1}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if fm := nm.(pickModel); fm.choice != 2 {
		t.Fatalf("digit jump choice = %d, want 2", fm.choice)
	}
	m2 := pickModel{items: []pickItem{{Label: "a"}}, choice: -1}
	nm2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if fm := nm2.(pickModel); !fm.quit || fm.choice != -1 {
		t.Fatalf("esc should cancel: %+v", fm)
	}
}

func TestPickModelViewMarksCursor(t *testing.T) {
	m := pickModel{title: "Select", items: []pickItem{{Label: "one", Detail: "d1"}, {Label: "two"}}, choice: -1, cursor: 1}
	v := m.View()
	if !strings.Contains(v, "Select") || !strings.Contains(v, "❯") {
		t.Fatalf("view missing title/cursor:\n%s", v)
	}
	if !strings.Contains(v, "esc cancel") {
		t.Fatalf("view missing key legend:\n%s", v)
	}
}
