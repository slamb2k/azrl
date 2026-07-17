package cmd

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

// pickItem is one row of the interactive arrow picker.
type pickItem struct{ Label, Detail string }

// useArrowPicker reports whether the arrow-key picker can run: a real
// terminal on stdin and stderr. A package var so tests (buffered stdin) keep
// the plain numbered path deterministically.
var useArrowPicker = func() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stderr.Fd())
}

// pickModel is a minimal bubbletea list: ↑↓/jk move, 1-9 jump-select,
// enter selects, esc/q cancels. Rendered on stderr so stdout stays clean
// (env's eval contract).
type pickModel struct {
	title  string
	items  []pickItem
	cursor int
	choice int // -1 until selected
	quit   bool
}

func (m pickModel) Init() tea.Cmd { return nil }

func (m pickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch s := key.String(); s {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "enter":
		m.choice = m.cursor
		return m, tea.Quit
	case "esc", "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	default:
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' && int(s[0]-'1') < len(m.items) {
			m.choice = int(s[0] - '1')
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m pickModel) View() string {
	if m.choice >= 0 || m.quit {
		return "" // clear the list on exit
	}
	var b strings.Builder
	b.WriteString(cliBold.Render(m.title) + "\n")
	labelW := 0
	for _, it := range m.items {
		if w := len([]rune(it.Label)); w > labelW {
			labelW = w
		}
	}
	for i, it := range m.items {
		line := fmt.Sprintf("%-*s  %s", labelW, it.Label, cliDim.Render(it.Detail))
		if i == m.cursor {
			b.WriteString(cliAccentBlue.Render("  ❯ "+line) + "\n")
		} else {
			b.WriteString("    " + line + "\n")
		}
	}
	b.WriteString(cliDim.Render("  ↑↓ move · ↵ select · 1-9 jump · esc cancel") + "\n")
	return b.String()
}

// pickArrow runs the picker and returns the chosen index, or an error when
// the user cancelled.
func pickArrow(title string, items []pickItem) (int, error) {
	m := pickModel{title: title, items: items, choice: -1}
	final, err := tea.NewProgram(m, tea.WithOutput(os.Stderr)).Run()
	if err != nil {
		return 0, err
	}
	fm := final.(pickModel)
	if fm.choice < 0 {
		return 0, fmt.Errorf("azrl: cancelled")
	}
	return fm.choice, nil
}
