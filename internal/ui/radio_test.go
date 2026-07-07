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
	for _, label := range []string{"Sign in", "Use here", "Remove"} {
		if !strings.Contains(v, label) {
			t.Fatalf("view missing %q:\n%s", label, v)
		}
	}
	if !strings.Contains(v, keyGlyph("l")) {
		t.Fatalf("view missing keycap glyph:\n%s", v)
	}
	// Selection is a background block (bright focused / dark parent shade) —
	// invisible under the test colour profile, so only content is asserted;
	// both states must render every label.
	r.focused = false
	uv := r.view(30)
	for _, label := range []string{"Sign in", "Use here", "Remove"} {
		if !strings.Contains(uv, label) {
			t.Fatalf("unfocused view missing %q:\n%s", label, uv)
		}
	}
}

func TestRadioViewRendersDisabledRows(t *testing.T) {
	r := newRadio([]radioOption{
		{label: "Sign in", key: "s"},
		{label: "Link here", key: "u", hint: "already linked here", disabled: true},
	})
	r.focused = true
	v := r.view(60)
	// Disabled rows still render — never hidden — with their reason hint.
	if !strings.Contains(v, "Link here") || !strings.Contains(v, "already linked here") {
		t.Fatalf("disabled row or its reason missing:\n%s", v)
	}
	// Cursor can land on a disabled row and the view still renders both rows.
	r.cursor = 1
	v2 := r.view(60)
	for _, label := range []string{"Sign in", "Link here"} {
		if !strings.Contains(v2, label) {
			t.Fatalf("view with cursor on disabled row missing %q:\n%s", label, v2)
		}
	}
}
