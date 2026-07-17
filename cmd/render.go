package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// Plain-CLI styling: the same visual language as the TUI (green = this dir,
// orange = parent dir, gold = shell/ambient accents). lipgloss detects the
// output profile from os.Stdout, so pipes, files, tests, and NO_COLOR all
// degrade to plain text automatically.
var (
	cliGood       = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950"))
	cliParent     = lipgloss.NewStyle().Foreground(lipgloss.Color("#d99a2b"))
	cliBad        = lipgloss.NewStyle().Foreground(lipgloss.Color("#f85149"))
	cliAccent     = lipgloss.NewStyle().Foreground(lipgloss.Color("#f2c14e"))
	cliDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b949e"))
	cliBold       = lipgloss.NewStyle().Bold(true)
	cliAccentBlue = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true) // bright blue — matches the prompt chip
)

// cliWidth returns the terminal width when stdout is a real terminal, else 0
// meaning "unlimited" — piped/redirected output is never truncated.
func cliWidth() int {
	fd := os.Stdout.Fd()
	if !term.IsTerminal(fd) {
		return 0
	}
	w, _, err := term.GetSize(fd)
	if err != nil || w <= 0 {
		return 0
	}
	return w
}

// tildePath shows $HOME as ~ for display; structured (JSON) output keeps
// absolute paths.
func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(os.PathSeparator)) {
		return "~" + p[len(home):]
	}
	return p
}

// truncateLeft keeps the tail of s (the discriminating end of a path) under
// max visible cells, prefixing an ellipsis.
func truncateLeft(s string, max int) string {
	if max <= 1 || lipgloss.Width(s) <= max {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > max {
		r = r[1:]
	}
	return "…" + string(r)
}

// renderAligned prints rows as space-padded columns sized to their content
// (cells may carry ANSI styling — widths are measured visually). When stdout
// is a terminal and the table overflows it, the widest columns shrink first,
// each cell tail-truncated with an ellipsis, down to a floor of 10 cells.
func renderAligned(w io.Writer, indent string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	for _, r := range rows {
		for i, c := range r {
			if cw := lipgloss.Width(c); cw > widths[i] {
				widths[i] = cw
			}
		}
	}
	if max := cliWidth(); max > 0 {
		avail := max - lipgloss.Width(indent) - 2*(cols-1)
		total := 0
		for _, cw := range widths {
			total += cw
		}
		for total > avail {
			widest := 0
			for i := 1; i < cols; i++ {
				if widths[i] > widths[widest] {
					widest = i
				}
			}
			if widths[widest] <= 10 {
				break
			}
			shrink := total - avail
			if shrink > widths[widest]-10 {
				shrink = widths[widest] - 10
			}
			widths[widest] -= shrink
			total -= shrink
		}
	}
	for _, r := range rows {
		var b strings.Builder
		b.WriteString(indent)
		for i, c := range r {
			if lipgloss.Width(c) > widths[i] {
				// Paths lose their head (the tail discriminates); styled or
				// prose cells lose their tail (ansi-aware).
				if !strings.Contains(c, "\x1b") && strings.Contains(c, "/") {
					c = truncateLeft(c, widths[i])
				} else {
					c = ansi.Truncate(c, widths[i]-1, "") + "…"
				}
			}
			if i == len(r)-1 {
				b.WriteString(c)
				break
			}
			b.WriteString(c)
			b.WriteString(strings.Repeat(" ", widths[i]-lipgloss.Width(c)+2))
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
}

// printList renders name/detail profile rows: bold names, content-sized
// columns instead of a hardcoded 24-cell gutter.
func printList(w io.Writer, pairs [][2]string) {
	rows := make([][]string, len(pairs))
	for i, p := range pairs {
		rows[i] = []string{cliBold.Render(p[0]), p[1]}
	}
	renderAligned(w, "", rows)
}
