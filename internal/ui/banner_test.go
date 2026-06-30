package ui

import (
	"strings"
	"testing"
)

func TestBannerContents(t *testing.T) {
	b := Banner()
	if !strings.Contains(b, "Azure Remote Login") {
		t.Fatalf("banner missing tagline:\n%s", b)
	}
	if strings.TrimSpace(AngelArt) == "" {
		t.Fatal("angel art is empty")
	}
	if strings.Count(AngelArt, "\n") < 6 {
		t.Fatalf("angel should be multi-line (>=7 rows), got:\n%s", AngelArt)
	}
}
