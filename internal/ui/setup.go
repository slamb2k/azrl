package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/envdetect"
)

// setupStep is the wizard's current screen.
type setupStep int

const (
	stepPick    setupStep = iota // choose among >1 candidate
	stepOS                       // remote only: dev-machine OS → BROWSER_CMD
	stepFields                   // edit the mode's fields
	stepConfirm                  // review + write
)

// osChoice maps a dev-machine OS to the command that opens a URL there.
type osChoice struct{ label, cmd string }

var osChoices = []osChoice{
	{"macOS", "open"},
	{"WSL (Windows browser)", "wslview"},
	{"Linux", "xdg-open"},
}

// setupField is one editable key in the field form.
type setupField struct {
	key   string // BROWSER_CMD / BROWSER_HOST / VM_SSH_HOST
	label string
	hint  string
	ti    textinput.Model
}

// setupModel is the Bubble Tea wizard behind `azrl setup`. It detects the
// environment, lets the user pick a candidate (when ambiguous), edit fields, and
// confirm. On confirm it exposes the resolved Global via Result().
type setupModel struct {
	cands  []envdetect.Candidate
	step   setupStep
	pick   int // candidate cursor
	osPick int // dev-machine OS cursor (remote)
	chosen envdetect.Candidate
	fields []setupField
	fcur   int
	result config.Global
	ok     bool // true once the user confirms
}

// newSetupModel builds the wizard from detected candidates (recommended first).
func newSetupModel(cands []envdetect.Candidate) setupModel {
	m := setupModel{cands: cands}
	if len(cands) <= 1 {
		if len(cands) == 1 {
			m.chosen = cands[0]
		}
		m.enterChosen()
	} else {
		m.step = stepPick
	}
	return m
}

// enterChosen advances from a settled candidate into OS pick (remote) or the
// field form (local).
func (m *setupModel) enterChosen() {
	if m.chosen.Mode == envdetect.Remote {
		m.step = stepOS
		return
	}
	m.buildFields()
	m.step = stepFields
}

// buildFields constructs the editable field set for the chosen mode.
func (m *setupModel) buildFields() {
	m.fields = m.fields[:0]
	add := func(key, label, hint, val string) {
		ti := textinput.New()
		ti.SetValue(val)
		ti.Prompt = ""
		m.fields = append(m.fields, setupField{key: key, label: label, hint: hint, ti: ti})
	}
	if m.chosen.Mode == envdetect.Local {
		add("BROWSER_CMD", "Browser command", "opens a URL on this machine", m.chosen.BrowserCmd)
	} else {
		add("VM_SSH_HOST", "VM SSH host", "this VM's SSH name (derived; edit for NAT/jump hosts)", m.chosen.VMSSHHost)
		add("BROWSER_HOST", "Browser SSH host (optional)", "set for zero-paste; blank = paste mode", m.chosen.BrowserHost)
	}
	m.fcur = 0
	for i := range m.fields {
		if i == 0 {
			m.fields[i].ti.Focus()
		} else {
			m.fields[i].ti.Blur()
		}
	}
}

// collect folds the chosen candidate and edited fields into a Global.
func (m setupModel) collect() config.Global {
	g := config.Global{BrowserCmd: m.chosen.BrowserCmd, BrowserHost: m.chosen.BrowserHost, VMSSHHost: m.chosen.VMSSHHost}
	for _, f := range m.fields {
		v := strings.TrimSpace(f.ti.Value())
		switch f.key {
		case "BROWSER_CMD":
			g.BrowserCmd = v
		case "BROWSER_HOST":
			g.BrowserHost = v
		case "VM_SSH_HOST":
			g.VMSSHHost = v
		}
	}
	return g
}

// Result returns the resolved config and whether the user confirmed it.
func (m setupModel) Result() (config.Global, bool) { return m.result, m.ok }

func (m setupModel) Init() tea.Cmd { return textinput.Blink }

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch msg := key; msg.String() {
	case "ctrl+c", "esc":
		m.ok = false
		return m, tea.Quit
	}
	switch m.step {
	case stepPick:
		return m.updatePick(key)
	case stepOS:
		return m.updateOS(key)
	case stepFields:
		return m.updateFields(key)
	case stepConfirm:
		return m.updateConfirm(key)
	}
	return m, nil
}

func (m setupModel) updatePick(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		if m.pick > 0 {
			m.pick--
		}
	case "down", "j":
		if m.pick < len(m.cands)-1 {
			m.pick++
		}
	case "enter":
		m.chosen = m.cands[m.pick]
		m.enterChosen()
	}
	return m, nil
}

func (m setupModel) updateOS(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		if m.osPick > 0 {
			m.osPick--
		}
	case "down", "j":
		if m.osPick < len(osChoices)-1 {
			m.osPick++
		}
	case "enter":
		m.chosen.BrowserCmd = osChoices[m.osPick].cmd
		m.buildFields()
		m.step = stepFields
	}
	return m, nil
}

func (m setupModel) updateFields(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "shift+tab":
		m.focusField(m.fcur - 1)
		return m, nil
	case "down", "tab":
		m.focusField(m.fcur + 1)
		return m, nil
	case "enter":
		if m.fcur < len(m.fields)-1 {
			m.focusField(m.fcur + 1)
			return m, nil
		}
		m.result = m.collect()
		m.step = stepConfirm
		return m, nil
	}
	var cmd tea.Cmd
	m.fields[m.fcur].ti, cmd = m.fields[m.fcur].ti.Update(key)
	return m, cmd
}

func (m *setupModel) focusField(i int) {
	if i < 0 || i >= len(m.fields) {
		return
	}
	m.fields[m.fcur].ti.Blur()
	m.fcur = i
	m.fields[i].ti.Focus()
}

func (m setupModel) updateConfirm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "enter", "y":
		m.ok = true
		return m, tea.Quit
	case "n", "left":
		// Back to editing.
		m.step = stepFields
	}
	return m, nil
}

func (m setupModel) View() string {
	var b strings.Builder
	b.WriteString(paneTitle("AZRL SETUP", true) + "\n\n")
	switch m.step {
	case stepPick:
		b.WriteString("Detected environments — pick one:\n\n")
		for i, c := range m.cands {
			label := c.Label
			if c.Recommended {
				label += "  " + mutedStyle.Render("(recommended)")
			}
			if i == m.pick {
				b.WriteString("  " + selBlockActive.Render(label) + "\n")
			} else {
				b.WriteString("  " + label + "\n")
			}
			b.WriteString("    " + mutedStyle.Render(c.Reason) + "\n")
		}
		b.WriteString("\n" + keyHelp("↑↓", "select", "↵", "next", "esc", "cancel"))
	case stepOS:
		b.WriteString("Which OS is your dev machine (where the browser opens)?\n\n")
		for i, o := range osChoices {
			line := o.label + "  " + mutedStyle.Render(o.cmd)
			if i == m.osPick {
				b.WriteString("  " + selBlockActive.Render(line) + "\n")
			} else {
				b.WriteString("  " + line + "\n")
			}
		}
		b.WriteString("\n" + keyHelp("↑↓", "select", "↵", "next", "esc", "cancel"))
	case stepFields:
		b.WriteString(m.chosen.Label + "\n\n")
		for i, f := range m.fields {
			marker := "  "
			if i == m.fcur {
				marker = "▸ "
			}
			b.WriteString(marker + f.label + ": " + f.ti.View() + "\n")
			b.WriteString("    " + mutedStyle.Render(f.hint) + "\n")
		}
		b.WriteString("\n" + keyHelp("↑↓", "field", "↵", "next", "esc", "cancel"))
	case stepConfirm:
		g := m.result
		b.WriteString("Write this to ~/.azure-profiles/azrl.conf?\n\n")
		fmt.Fprintf(&b, "  BROWSER_CMD  = %s\n", g.BrowserCmd)
		fmt.Fprintf(&b, "  BROWSER_HOST = %s\n", emptyDash(g.BrowserHost))
		fmt.Fprintf(&b, "  VM_SSH_HOST  = %s\n", emptyDash(g.VMSSHHost))
		mode := "remote"
		if g.IsLocal() {
			mode = "local (no SSH bridge)"
		}
		b.WriteString("\n  " + mutedStyle.Render("mode: "+mode) + "\n")
		b.WriteString("\n" + keyHelp("↵/y", "write", "n", "back", "esc", "cancel"))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(azureBlue).
		Padding(0, 2).
		Render(b.String())
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// RunSetupWizard runs the interactive wizard and returns the resolved config and
// whether the user confirmed it.
func RunSetupWizard(cands []envdetect.Candidate) (config.Global, bool, error) {
	final, err := tea.NewProgram(newSetupModel(cands)).Run()
	if err != nil {
		return config.Global{}, false, err
	}
	fm, ok := final.(setupModel)
	if !ok {
		return config.Global{}, false, nil
	}
	g, confirmed := fm.Result()
	return g, confirmed, nil
}
