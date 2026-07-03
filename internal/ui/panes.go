package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
// identity is a pre-rendered segment; "" renders as a muted em-dash.
func headerStrip(icon, title, cwd, identity string) string {
	if identity == "" {
		identity = mutedStyle.Render("—")
	}
	return icon + " " + paneTitleStyle.Render(title) +
		mutedStyle.Render("   ·   ") + "📁 " + displayDir(cwd) +
		mutedStyle.Render("   ·   ") + "👤 " + identity
}

// effectiveIdentity renders the header's 👤 segment: the dir-pinned profile's
// signed-in identity, the pinned profile's name with a not-signed-in note
// when its session is empty (capture is metadata-only — identity appears
// after login), else the provider's ambient identity, else "".
func effectiveIdentity(dirProfile, dirIdentity, ambient string) string {
	switch {
	case dirIdentity != "":
		return accentStyle.Render(dirIdentity)
	case dirProfile != "":
		return accentStyle.Render(dirProfile) + mutedStyle.Render(" · not signed in")
	case ambient != "":
		return accentStyle.Render(ambient)
	}
	return ""
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

// overlayCenter splices box over the middle of bg (a full-width rendered
// view), preserving the background around it — a true popup. Both truncation
// sides are ANSI-aware so styling doesn't bleed across the seams.
func overlayCenter(bg, box string, width int) string {
	bgl := strings.Split(bg, "\n")
	bl := strings.Split(box, "\n")
	boxW := lipgloss.Width(box)
	x := (width - boxW) / 2
	if x < 0 {
		x = 0
	}
	y := (len(bgl) - len(bl)) / 2
	if y < 0 {
		y = 0
	}
	for i, l := range bl {
		if y+i >= len(bgl) {
			break
		}
		line := padTo(bgl[y+i], width)
		left := ansi.Truncate(line, x, "")
		right := ansi.TruncateLeft(line, x+boxW, "")
		bgl[y+i] = left + padTo(l, boxW) + right
	}
	return strings.Join(bgl, "\n")
}
