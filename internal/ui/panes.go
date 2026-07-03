package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// providerIcon returns the brand-coloured emoji for a provider tab header.
func providerIcon(name string) string {
	switch name {
	case "azure":
		return "🔷"
	case "aws":
		return "🟠"
	case "gcp":
		return "🔴"
	case "github":
		return "🐙"
	}
	return "◆"
}

// headerStrip is the standard provider-tab header: icon + provider title on
// the left, the current directory and the effective identity on the right —
// the same anatomy on every tab so the eye always knows where to look.
// identity may be "" (rendered as a muted em-dash).
func headerStrip(icon, title, cwd, identity string) string {
	id := mutedStyle.Render("—")
	if identity != "" {
		id = accentStyle.Render(identity)
	}
	return icon + " " + paneTitleStyle.Render(title) +
		mutedStyle.Render("   ·   ") + mutedStyle.Render("dir ") + displayDir(cwd) +
		mutedStyle.Render("   ·   ") + mutedStyle.Render("id ") + id
}

// profileInfoBlock renders the top of the DETAILS pane for one profile: a
// key/value sheet with a fixed key column — the conf detail plus the
// disk-only status (identity, expiry, last-used).
func profileInfoBlock(pr profile.Listed, st provider.Status, w int) string {
	row := func(k, v string) string {
		if v == "" {
			v = mutedStyle.Render("—")
		}
		return truncateLine(mutedStyle.Render(padTo(k, 10))+" "+v, w)
	}
	rows := []string{
		row("Name", pr.Display()),
		row("Identity", st.Identity),
		row("Detail", pr.Detail),
		row("Expiry", expiryWord(st.Expiry)),
		row("Last used", lastUsedWord(st.LastUsed)),
	}
	return strings.Join(rows, "\n")
}

func expiryWord(t *time.Time) string {
	if t == nil {
		return ""
	}
	d := time.Until(*t)
	if d <= 0 {
		return failureStyle.Render("expired")
	}
	if d < 2*time.Hour {
		return accentStyle.Render(d.Round(time.Minute).String() + " left")
	}
	return d.Round(time.Hour).String() + " left"
}

func lastUsedWord(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return d.Round(time.Minute).String() + " ago"
	case d < 48*time.Hour:
		return d.Round(time.Hour).String() + " ago"
	}
	return t.Format("2006-01-02")
}

// selMode grades how a container's selection renders: bright while focused,
// the darker parent shade while a descendant holds focus, and not at all
// while an ancestor does.
type selMode int

const (
	selNone selMode = iota
	selParent
	selActive
)

// scopeLegend is the one-line-per-tier key rendered under the profiles list
// (centered), decoding the relevance icons.
func scopeLegend(w int) string {
	rows := []string{
		successStyle.Render("●") + mutedStyle.Render(" this dir   ") +
			lipgloss.NewStyle().Foreground(goldDeep).Render("●") + mutedStyle.Render(" parent dir"),
		"🌐" + mutedStyle.Render(" default    ") +
			lipgloss.NewStyle().Foreground(whiteDim).Render("●") + mutedStyle.Render(" elsewhere  ") +
			lipgloss.NewStyle().Foreground(grayDeep).Render("●") + mutedStyle.Render(" unmapped"),
	}
	for i, r := range rows {
		rows[i] = lipgloss.PlaceHorizontal(w, lipgloss.Center, truncateLine(r, w))
	}
	return strings.Join(rows, "\n")
}
