package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// TestJustifyNeverOverflowsWidth guards a real bug: a wide mid segment could
// push the centred layout's right-padding clamp out without shrinking the
// left gap to compensate, so the composed line came out wider than asked —
// silently truncated by the caller's own width normalization, chopping text
// off the right segment (e.g. a shell-override chip) instead of the mid path.
func TestJustifyNeverOverflowsWidth(t *testing.T) {
	left := "🟠 AWS"
	right := "👤 ⌁ shell: prod"
	for _, width := range []int{40, 60, 80, 90, 100, 106, 110, 200} {
		for _, mid := range []string{
			"📁 ~/x",
			"📁 /home/slamb2k/work/azrl/.claude/worktrees/whoami-sub-display/internal/ui",
			strings.Repeat("📁 /very/long/path/segment", 4),
		} {
			out := justify(width, left, mid, right)
			if w := lipgloss.Width(out); w > width {
				t.Fatalf("justify(%d, ..., mid=%q) produced width %d: %q", width, mid, w, out)
			}
			if width >= 40 && !strings.Contains(out, "shell: prod") {
				t.Fatalf("justify(%d, ..., mid=%q) dropped the right segment: %q", width, mid, out)
			}
		}
	}
}

func TestExpiryWordPerProvider(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	live := time.Now().Add(30 * time.Minute)

	// Azure/GCP: a stale access token is refreshed silently on next use.
	for _, prov := range []string{"azure", "gcp"} {
		if w := expiryWord(prov, &past); !strings.Contains(w, "token stale · refreshes on next use") {
			t.Fatalf("%s stale word = %q", prov, w)
		}
		if w := expiryWord(prov, &live); !strings.Contains(w, "left") {
			t.Fatalf("%s live expiry should still show the countdown: %q", prov, w)
		}
	}
	// AWS: expired means sign in again.
	if w := expiryWord("aws", &past); !strings.Contains(w, "expired") {
		t.Fatalf("aws expired word = %q", w)
	}
	// GitHub / none tracked: empty.
	if w := expiryWord("github", nil); w != "" {
		t.Fatalf("nil expiry should render empty, got %q", w)
	}
}
