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

// headerStrip is the standard provider-tab header, justified across the
// content width: icon + provider title anchored left, 📁 directory centered,
// 👤 effective identity anchored right — the same anatomy on every tab.
// identity is a pre-rendered segment; "" renders as a muted em-dash.
func headerStrip(width int, icon, title, cwd, identity string) string {
	if identity == "" {
		identity = mutedStyle.Render("—")
	}
	return justify(width,
		icon+" "+paneTitleStyle.Render(title),
		"📁 "+displayDir(cwd),
		"👤 "+identity)
}

// justify spreads three segments across width — left anchored, mid centered,
// right anchored — collapsing to a compact join when the width is tight. The
// balanced whitespace separates the zones, so no divider glyphs are needed.
func justify(width int, left, mid, right string) string {
	lw, mw, rw := lipgloss.Width(left), lipgloss.Width(mid), lipgloss.Width(right)
	if lw+mw+rw+4 > width {
		return truncateLine(left+"  "+mid+"  "+right, width)
	}
	midStart := (width - mw) / 2
	if midStart < lw+2 {
		midStart = lw + 2
	}
	rightStart := width - rw
	if rightStart < midStart+mw+2 {
		rightStart = midStart + mw + 2
	}
	return left + strings.Repeat(" ", midStart-lw) + mid +
		strings.Repeat(" ", rightStart-midStart-mw) + right
}

// effectiveIdentity renders the header's 👤 segment: the linked profile's
// signed-in identity, the linked profile's name with a not-signed-in note
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
// disk-only status (identity, linked dirs, expiry, last-used).
func profileInfoBlock(prov string, pr profile.Listed, st provider.Status, browser, linked, driftNote string, w int) string {
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
		row("Browser", browser),
		row("Linked", linked),
		row("Expiry", expiryWord(prov, st.Expiry)),
		row("Last used", lastUsedWord(st.LastUsed)),
	}
	if driftNote != "" {
		rows = append(rows, truncateLine(mutedStyle.Render(padTo("Drift", 10))+" "+failureStyle.Render("⚠ ")+driftNote, w))
	}
	return strings.Join(rows, "\n")
}

// expiryWord renders the DETAILS expiry truthfully per provider: AWS expiry
// is actionable ("expired" = sign in again), while a stale Azure/GCP access
// token is refreshed silently by az/gcloud on next use — say so instead of
// crying wolf. nil (GitHub / nothing tracked) renders empty.
func expiryWord(prov string, t *time.Time) string {
	if t == nil {
		return ""
	}
	d := time.Until(*t)
	if d <= 0 {
		if !ExpiryActionable(prov) {
			return mutedStyle.Render("token stale · refreshes on next use")
		}
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
// scopeLegend decodes the row marks, centered under the profiles list: two
// relevance dots plus the default tag.
func scopeLegend(w int) string {
	row := successStyle.Render("●") + mutedStyle.Render(" this dir   ") +
		lipgloss.NewStyle().Foreground(goldDeep).Render("●") + mutedStyle.Render(" parent dir   ") +
		mutedStyle.Render("⌁ default")
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, truncateLine(row, w))
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

// orNoSession renders an identity or an explicit no-session note.
func orNoSession(identity string) string {
	if identity == "" {
		return "no active session"
	}
	return identity
}
