package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// dirPicker is the change-directory overlay: a fuzzy finder over the mapping
// index's known directories plus a bounded walk of $HOME. Enter accepts the
// highlighted match (or the literal input when it names an existing dir);
// esc cancels. The container owns it and applies the result via os.Chdir so
// every existing cwd consumer — and exec'd handoffs — follow along.
type dirPicker struct {
	input      textinput.Model
	candidates []string
	matches    []string
	cursor     int
	width      int
	height     int
}

// walkDepth and walkCap bound the $HOME scan so opening the picker stays fast
// on large home directories.
const (
	walkDepth = 3
	walkCap   = 4000
)

func newDirPicker(width, height int) dirPicker {
	ti := textinput.New()
	ti.Placeholder = "fuzzy path…"
	ti.Focus()
	p := dirPicker{input: ti, candidates: candidateDirs(), width: width, height: height}
	p.refilter()
	return p
}

// candidateDirs merges every provider's mapping-index directories with a
// depth- and count-bounded walk of $HOME (hidden dirs and dependency
// directories skipped), deduplicated and sorted.
func candidateDirs() []string {
	seen := map[string]bool{}
	add := func(d string) {
		if d != "" && !seen[d] {
			seen[d] = true
		}
	}
	for _, p := range provider.All() {
		for _, m := range profile.ReadMappings(p.ProfilesDir()) {
			add(m.Dir)
		}
	}
	home, err := os.UserHomeDir()
	if err == nil {
		add(home)
		walkDirs(home, home, walkDepth, seen, add)
	}
	if cwd, err := os.Getwd(); err == nil {
		add(cwd)
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func walkDirs(root, dir string, depth int, seen map[string]bool, add func(string)) {
	if depth == 0 || len(seen) >= walkCap {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
			continue
		}
		child := filepath.Join(dir, name)
		add(child)
		if len(seen) >= walkCap {
			return
		}
		walkDirs(root, child, depth-1, seen, add)
	}
}

// fuzzyScore is a case-insensitive subsequence match of pattern against s:
// -1 when pattern isn't a subsequence; otherwise higher is better, with
// bonuses for consecutive runs and matches at path-segment starts, and a
// mild penalty for longer paths so shallow dirs surface first.
func fuzzyScore(pattern, s string) int {
	if pattern == "" {
		return 0
	}
	p, t := strings.ToLower(pattern), strings.ToLower(s)
	score, run, ti := 0, 0, 0
	for _, pc := range p {
		found := false
		for ti < len(t) {
			if rune(t[ti]) == pc {
				bonus := 1 + run
				if ti == 0 || t[ti-1] == '/' || t[ti-1] == '-' || t[ti-1] == '_' {
					bonus += 4
				}
				score += bonus
				run++
				ti++
				found = true
				break
			}
			run = 0
			ti++
		}
		if !found {
			return -1
		}
	}
	return score - len(s)/8
}

func (p *dirPicker) refilter() {
	pattern := p.input.Value()
	type scored struct {
		dir   string
		score int
	}
	var hits []scored
	for _, d := range p.candidates {
		if sc := fuzzyScore(pattern, displayDir(d)); sc >= 0 {
			hits = append(hits, scored{d, sc})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	p.matches = p.matches[:0]
	for _, h := range hits {
		p.matches = append(p.matches, h.dir)
	}
	if p.cursor >= len(p.matches) {
		p.cursor = 0
	}
}

// displayDir shortens $HOME to ~ for matching and rendering.
func displayDir(d string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if d == home {
			return "~"
		}
		if strings.HasPrefix(d, home+string(filepath.Separator)) {
			return "~" + d[len(home):]
		}
	}
	return d
}

// expandDir reverses displayDir and cleans the result.
func expandDir(d string) string {
	if d == "~" || strings.HasPrefix(d, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			d = filepath.Join(home, strings.TrimPrefix(d, "~"))
		}
	}
	return filepath.Clean(d)
}

// update handles one key. picked is the accepted directory ("" when none);
// closed reports that the overlay should be dismissed.
func (p dirPicker) update(msg tea.KeyMsg) (dirPicker, string, bool) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return p, "", true
	case "up":
		if p.cursor > 0 {
			p.cursor--
		}
		return p, "", false
	case "down":
		if p.cursor < len(p.matches)-1 {
			p.cursor++
		}
		return p, "", false
	case "enter":
		if p.cursor < len(p.matches) {
			return p, p.matches[p.cursor], true
		}
		if p.input.Value() == "" {
			return p, "", false
		}
		if d := expandDir(p.input.Value()); d != "" {
			if fi, err := os.Stat(d); err == nil && fi.IsDir() {
				return p, d, true
			}
		}
		return p, "", false
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	_ = cmd
	p.refilter()
	return p, "", false
}

// view renders the overlay body (the container supplies banner + tab bar).
func (p dirPicker) view() string {
	contentW := p.width - 4
	if contentW < 1 {
		contentW = 1
	}
	rows := p.height - 7 // frame, title, input, help
	if rows < 1 {
		rows = 1
	}
	var b strings.Builder
	b.WriteString(paneTitle("CHANGE DIRECTORY", true) + "\n\n")
	b.WriteString(p.input.View() + "\n\n")
	for i, m := range p.matches {
		if i >= rows {
			break
		}
		line := truncateLine(displayDir(m), contentW-4)
		if i == p.cursor {
			b.WriteString(selectionBar.Foreground(gold).Bold(true).Render(line) + "\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(white).PaddingLeft(2).Render(line) + "\n")
		}
	}
	if len(p.matches) == 0 {
		b.WriteString(mutedStyle.Render("  (no matches — enter accepts a literal existing path)") + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑↓ select · ↵ change dir · esc cancel"))
	lines := strings.Split(b.String(), "\n")
	for i, l := range lines {
		lines[i] = padTo(truncateLine(l, contentW), contentW)
	}
	return frameStyle.Render(strings.Join(lines, "\n"))
}
