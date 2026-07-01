package ui

import (
	"strings"
	"testing"
)

func TestBannerContents(t *testing.T) {
	b := Banner()
	if !strings.Contains(b, "█") {
		t.Fatalf("banner missing block letters:\n%s", b)
	}
	// at least one braille glyph (U+2800..U+28FF) for the wings
	hasBraille := false
	for _, r := range b {
		if r >= 0x2800 && r <= 0x28FF {
			hasBraille = true
			break
		}
	}
	if !hasBraille {
		t.Fatalf("banner missing braille wings:\n%s", b)
	}
}
