package ui

import (
	"strings"
	"testing"
)

func testRadio() radio {
	return newRadio([]radioOption{
		{label: "Sign in", key: "l"},
		{label: "Use here", key: "u"},
		{label: "Remove", key: "d"},
	})
}

func TestRadioCursorBounds(t *testing.T) {
	r := testRadio()
	r.up() // already at top
	if r.cursor != 0 {
		t.Fatalf("up at top: cursor=%d", r.cursor)
	}
	r.down()
	r.down()
	r.down() // past the end
	if r.cursor != 2 {
		t.Fatalf("down past end: cursor=%d", r.cursor)
	}
	if r.selected().label != "Remove" {
		t.Fatalf("selected=%q", r.selected().label)
	}
}

func TestRadioSelectByKey(t *testing.T) {
	r := testRadio()
	if !r.selectByKey("u") || r.selected().key != "u" {
		t.Fatalf("selectByKey(u): cursor=%d", r.cursor)
	}
	if r.selectByKey("z") {
		t.Fatal("selectByKey(z) should not match")
	}
	if r.selected().key != "u" {
		t.Fatalf("cursor moved on miss: %q", r.selected().key)
	}
}

func TestRadioView(t *testing.T) {
	r := testRadio()
	r.focused = true
	v := r.view(30)
	if !strings.Contains(v, "◉") || !strings.Contains(v, "○") {
		t.Fatalf("view missing radio markers:\n%s", v)
	}
	for _, label := range []string{"Sign in", "Use here", "Remove"} {
		if !strings.Contains(v, label) {
			t.Fatalf("view missing %q:\n%s", label, v)
		}
	}
	if !strings.Contains(v, keyGlyph("l")) {
		t.Fatalf("view missing keycap glyph:\n%s", v)
	}
}
