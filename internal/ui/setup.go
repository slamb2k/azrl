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
	width  int
	height int
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
		add("VM_SSH_HOST", "VM SSH host", "this VM's SSH name — edit for NAT / jump hosts", m.chosen.VMSSHHost)
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
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
		return m, nil
	}
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

// setupCardWidth is the fixed inner width of the wizard card so every step's
// content and framing line up regardless of terminal size. Sized to seat the
// longest one-line blurb/hint without wrapping.
const setupCardWidth = 66

var (
	setupCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(azureBlue).
			Padding(1, 3).
			Width(setupCardWidth)

	setupHeadStyle = lipgloss.NewStyle().Foreground(azureSky).Bold(true)
	fieldKeyStyle  = lipgloss.NewStyle().Foreground(gold).Bold(true)
	confKeyStyle   = lipgloss.NewStyle().Foreground(azureSky)
	localBadge     = lipgloss.NewStyle().Foreground(white).Background(green).Bold(true)
	remoteBadge    = lipgloss.NewStyle().Foreground(white).Background(azureBlue).Bold(true)
	arrowStyle     = lipgloss.NewStyle().Foreground(gold).Bold(true)
)

// modeBadge renders a LOCAL/REMOTE pill: green for local (no bridge), blue for
// remote (browser over SSH).
func modeBadge(local bool) string {
	if local {
		return localBadge.Render(" LOCAL ")
	}
	return remoteBadge.Render(" REMOTE ")
}

// modeBlurb is the one-line description that sits beside the mode badge.
func modeBlurb(local bool) string {
	if local {
		return "browser opens on this machine — no SSH bridge"
	}
	return "browser opens on your dev machine over SSH"
}

// stageLabels lists the ordered wizard stages for the current flow; the browser
// (OS) stage only exists on the remote path.
func (m setupModel) stageLabels() []string {
	if m.chosen.Mode == envdetect.Remote {
		return []string{"Detect", "Browser", "Configure", "Confirm"}
	}
	return []string{"Detect", "Configure", "Confirm"}
}

// activeStage maps the current step onto its stage label.
func (m setupModel) activeStage() string {
	switch m.step {
	case stepPick:
		return "Detect"
	case stepOS:
		return "Browser"
	case stepFields:
		return "Configure"
	default:
		return "Confirm"
	}
}

// breadcrumb renders the stage trail: completed stages ticked blue, the active
// stage gold, upcoming stages muted.
func (m setupModel) breadcrumb() string {
	active := m.activeStage()
	seen := false
	var parts []string
	for _, l := range m.stageLabels() {
		switch {
		case l == active:
			seen = true
			parts = append(parts, accentStyle.Render("● "+l))
		case !seen:
			parts = append(parts, dotStyle.Render("✓ "+l))
		default:
			parts = append(parts, mutedStyle.Render("○ "+l))
		}
	}
	return strings.Join(parts, mutedStyle.Render("  ─  "))
}

// stepCounter labels the current step's position in the whole flow, e.g.
// "STEP 2 OF 4" — the plain-language companion to the breadcrumb.
func (m setupModel) stepCounter() string {
	labels := m.stageLabels()
	active := m.activeStage()
	idx := 1
	for i, l := range labels {
		if l == active {
			idx = i + 1
			break
		}
	}
	return accentStyle.Render(fmt.Sprintf("STEP %d OF %d", idx, len(labels)))
}

// stepTitle is the headline inside the card for the current step.
func (m setupModel) stepTitle() string {
	switch m.step {
	case stepPick:
		return "Which environment is this?"
	case stepOS:
		return "Where does your browser open?"
	case stepFields:
		return "Review the details"
	default:
		return "Ready to write"
	}
}

// footerHelp is the keycap hint bar for the current step.
func (m setupModel) footerHelp() string {
	switch m.step {
	case stepConfirm:
		return keyHelp("↵/y", "write", "n", "back", "esc", "cancel")
	case stepFields:
		return keyHelp("↑↓", "field", "↵", "next", "esc", "cancel")
	default:
		return keyHelp("↑↓", "select", "↵", "next", "esc", "cancel")
	}
}

func (m setupModel) View() string {
	w := m.width
	if w <= 0 {
		w = bannerWidth()
	}
	var b strings.Builder
	b.WriteString(setupHeadStyle.Render(m.stepTitle()) + "\n\n")
	switch m.step {
	case stepPick:
		for i, c := range m.cands {
			marker, name := "  ", c.Label
			if i == m.pick {
				marker, name = arrowStyle.Render("▸ "), selBlockActive.Render(c.Label)
			}
			star := ""
			if c.Recommended {
				star = "  " + accentStyle.Render("★")
			}
			b.WriteString(marker + modeBadge(c.Mode == envdetect.Local) + "  " + name + star + "\n")
			b.WriteString("     " + mutedStyle.Render(c.Reason) + "\n\n")
		}
	case stepOS:
		for i, o := range osChoices {
			marker, name := "  ", o.label
			if i == m.osPick {
				marker, name = arrowStyle.Render("▸ "), selBlockActive.Render(o.label)
			}
			b.WriteString(marker + name + "   " + mutedStyle.Render(o.cmd) + "\n")
		}
	case stepFields:
		local := m.chosen.Mode == envdetect.Local
		b.WriteString(modeBadge(local) + "  " + mutedStyle.Render(modeBlurb(local)) + "\n\n")
		for i, f := range m.fields {
			marker, label := "  ", mutedStyle.Render(f.label)
			if i == m.fcur {
				marker, label = arrowStyle.Render("▸ "), fieldKeyStyle.Render(f.label)
			}
			b.WriteString(marker + label + "\n")
			b.WriteString("    " + f.ti.View() + "\n")
			b.WriteString("    " + mutedStyle.Render(f.hint) + "\n\n")
		}
	case stepConfirm:
		g := m.result
		local := g.IsLocal()
		b.WriteString(modeBadge(local) + "  " + mutedStyle.Render(modeBlurb(local)) + "\n\n")
		row := func(k, v string) {
			b.WriteString("  " + confKeyStyle.Render(fmt.Sprintf("%-13s", k)) + accentStyle.Render(v) + "\n")
		}
		row("BROWSER_CMD", g.BrowserCmd)
		row("BROWSER_HOST", emptyDash(g.BrowserHost))
		row("VM_SSH_HOST", emptyDash(g.VMSSHHost))
		b.WriteString("\n" + arrowStyle.Render("→ ") + mutedStyle.Render("writes ~/.azure-profiles/azrl.conf") + "\n")
		b.WriteString("  " + mutedStyle.Render("existing config is backed up to azrl.conf.bak") + "\n")
	}
	b.WriteString("\n" + m.footerHelp())

	// Stack the crest, the step counter + breadcrumb, and the card, each centered
	// on the widest element (the banner), so the whole thing reads as one column.
	block := lipgloss.JoinVertical(
		lipgloss.Center,
		bannerFor(w),
		"",
		m.stepCounter(),
		m.breadcrumb(),
		"",
		setupCardStyle.Render(b.String()),
	)
	// Full-screen: centre the block in the alt-screen viewport. Falls back to the
	// bare block when dimensions are unknown (e.g. in tests).
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
	}
	return block
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// RunSetupWizard runs the interactive wizard full-screen (alt-screen) and
// returns the resolved config and whether the user confirmed it. The alt-screen
// is torn down on exit, so any status the caller prints lands on a clean
// terminal with no leftover wizard chrome.
func RunSetupWizard(cands []envdetect.Candidate) (config.Global, bool, error) {
	final, err := tea.NewProgram(newSetupModel(cands), tea.WithAltScreen()).Run()
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
