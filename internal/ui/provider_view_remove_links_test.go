package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/aws"
)

// setupAwsWithProfiles writes profile confs for each name under a fresh
// AWS_PROFILES-style home and returns the confdir.
func setupAwsWithProfiles(t *testing.T, names ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	for _, n := range names {
		os.WriteFile(filepath.Join(ap, n+".conf"),
			[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)
	}
	return ap
}

// downTo moves the profile cursor from the first row onto the named profile
// row and returns the updated view.
func downTo(v awsView, name string) awsView {
	for i, p := range v.profiles {
		if p.Name == name {
			for j := 0; j < i; j++ {
				nm, _ := v.Update(tea.KeyMsg{Type: tea.KeyDown})
				v = nm.(awsView)
			}
			return v
		}
	}
	return v
}

// Behavior 1: deleting a profile with 2 links shows both dirs and the three
// options; Unlink removes both pointer files, mapping rows, and the profile.
func TestRemoveConfirmWithLinks_ListsDirsAndUnlinks(t *testing.T) {
	ap := setupAwsWithProfiles(t, "work")
	// Short, fixed-name dirs: the confirm pane truncates long lines to fit its
	// width, so a t.Name()-derived TempDir() path wouldn't survive intact.
	root, err := os.MkdirTemp("", "az")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	d1, d2 := filepath.Join(root, "d1"), filepath.Join(root, "d2")
	os.Mkdir(d1, 0o755)
	os.Mkdir(d2, 0o755)
	scheme := aws.NewProvider().Scheme()
	if err := scheme.Use("work", ap, d1); err != nil {
		t.Fatal(err)
	}
	if err := scheme.Use("work", ap, d2); err != nil {
		t.Fatal(err)
	}

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	v = downTo(nm.(awsView), "work")
	nm, _ = v.Update(tea.KeyMsg{Type: tea.KeyDelete})
	av := nm.(awsView)

	if !av.confirming || av.pendingDelete != "work" {
		t.Fatalf("delete should arm the confirm (confirming=%v pending=%q)", av.confirming, av.pendingDelete)
	}
	if len(av.linkedDirs) != 2 {
		t.Fatalf("expected 2 linked dirs, got %v", av.linkedDirs)
	}
	if len(av.confirm.options) != 3 {
		t.Fatalf("linked-profile confirm should have 3 options, got %d", len(av.confirm.options))
	}
	out := av.View()
	for _, want := range []string{"Unlink 2 dirs + delete", "Replace links with", displayDir(d1), displayDir(d2)} {
		if !strings.Contains(out, want) {
			t.Fatalf("confirm view missing %q:\n%s", want, out)
		}
	}

	// Move onto "Unlink" (index 1) and confirm.
	av.confirm.down()
	nm2, _ := av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	av = nm2.(awsView)

	if av.confirming {
		t.Fatal("confirm should close after unlink+delete")
	}
	if len(av.profiles) != 0 {
		t.Fatal("profile should be removed")
	}
	if _, err := os.Stat(filepath.Join(d1, ".awsprofile")); !os.IsNotExist(err) {
		t.Fatal("d1 pointer should be removed")
	}
	if _, err := os.Stat(filepath.Join(d2, ".awsprofile")); !os.IsNotExist(err) {
		t.Fatal("d2 pointer should be removed")
	}
	if got := scheme.ReadMappings(ap); len(got) != 0 {
		t.Fatalf("mapping rows should be gone, got %v", got)
	}
}

// Behavior 2: Replace opens a picker of other profiles; picking one rewrites
// both pointers and deletes the original.
func TestRemoveConfirmReplace(t *testing.T) {
	ap := setupAwsWithProfiles(t, "work", "personal")
	d1, d2 := t.TempDir(), t.TempDir()
	scheme := aws.NewProvider().Scheme()
	scheme.Use("work", ap, d1)
	scheme.Use("work", ap, d2)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	v = downTo(nm.(awsView), "work")
	nm, _ = v.Update(tea.KeyMsg{Type: tea.KeyDelete})
	av := nm.(awsView)

	// Move onto "Replace links with…" (index 2) and select it.
	av.confirm.down()
	av.confirm.down()
	nm2, _ := av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	av = nm2.(awsView)
	if !av.replacePicking {
		t.Fatal("selecting Replace should open the picker")
	}
	if len(av.replacePick.options) != 1 || av.replacePick.options[0].label != "personal" {
		t.Fatalf("replace picker should list the other profile, got %+v", av.replacePick.options)
	}

	nm3, _ := av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	av = nm3.(awsView)

	if av.confirming || av.replacePicking {
		t.Fatal("confirm/picker should close after replace")
	}
	if len(av.profiles) != 1 || av.profiles[0].Name != "personal" {
		t.Fatalf("work should be removed, personal should remain: %+v", av.profiles)
	}
	for _, d := range []string{d1, d2} {
		b, err := os.ReadFile(filepath.Join(d, ".awsprofile"))
		if err != nil || strings.TrimSpace(string(b)) != "personal" {
			t.Fatalf("%s pointer should now name personal, got %q err=%v", d, b, err)
		}
	}
}

// Behavior 3: with no other profile, Replace is disabled with the exact reason.
func TestRemoveConfirmReplaceDisabledWithNoOtherProfile(t *testing.T) {
	ap := setupAwsWithProfiles(t, "work")
	d1 := t.TempDir()
	scheme := aws.NewProvider().Scheme()
	scheme.Use("work", ap, d1)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	v = downTo(nm.(awsView), "work")
	nm, _ = v.Update(tea.KeyMsg{Type: tea.KeyDelete})
	av := nm.(awsView)

	replace := av.confirm.options[2]
	if !replace.disabled || replace.hint != "no other profile to point them at" {
		t.Fatalf("Replace should be disabled with the exact reason, got disabled=%v hint=%q", replace.disabled, replace.hint)
	}

	// Selecting it and pressing enter must not open the picker or delete anything.
	av.confirm.down()
	av.confirm.down()
	nm2, _ := av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	av = nm2.(awsView)
	if av.replacePicking {
		t.Fatal("disabled Replace must not open the picker")
	}
	if !av.confirming || len(av.profiles) != 1 {
		t.Fatal("disabled Replace must not remove the profile")
	}
}

// Behavior 4: a link-free profile still gets the simple No/Yes confirm.
func TestRemoveConfirmLinkFreeUnchanged(t *testing.T) {
	setupAwsWithProfiles(t, "work")

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	v = downTo(nm.(awsView), "work")
	nm, _ = v.Update(tea.KeyMsg{Type: tea.KeyDelete})
	av := nm.(awsView)

	if len(av.linkedDirs) != 0 {
		t.Fatalf("expected no linked dirs, got %v", av.linkedDirs)
	}
	if len(av.confirm.options) != 2 {
		t.Fatalf("link-free confirm should keep the 2-option dialog, got %d", len(av.confirm.options))
	}
	out := av.View()
	// The pane frame wraps each line with border padding, so check the
	// wording piecewise rather than as one contiguous multi-line substring.
	for _, want := range []string{"Removes its conf, token dir,", "and this dir's .awsprofile.", "No, keep it", "Yes, remove work"} {
		if !strings.Contains(out, want) {
			t.Fatalf("link-free confirm text should be unchanged, missing %q:\n%s", want, out)
		}
	}
}

// Behavior 5: esc cancels at both levels; 'y' fast-path maps to unlink-all
// when links exist.
func TestRemoveConfirmEscAndYFastPath(t *testing.T) {
	ap := setupAwsWithProfiles(t, "work", "personal")
	d1 := t.TempDir()
	scheme := aws.NewProvider().Scheme()
	scheme.Use("work", ap, d1)

	// esc at the top level cancels without deleting.
	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	v = downTo(nm.(awsView), "work")
	nm, _ = v.Update(tea.KeyMsg{Type: tea.KeyDelete})
	nm, _ = nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyEsc})
	av := nm.(awsView)
	if av.confirming || len(av.profiles) != 2 {
		t.Fatal("esc at the top level should cancel without deleting")
	}

	// esc inside the replace picker cancels entirely (not just one level back).
	nm, _ = v.Update(tea.KeyMsg{Type: tea.KeyDelete})
	av = nm.(awsView)
	av.confirm.down()
	av.confirm.down()
	nm2, _ := av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	av = nm2.(awsView)
	if !av.replacePicking {
		t.Fatal("Replace should open the picker")
	}
	nm3, _ := av.Update(tea.KeyMsg{Type: tea.KeyEsc})
	av = nm3.(awsView)
	if av.confirming || av.replacePicking || len(av.profiles) != 2 {
		t.Fatal("esc in the replace picker should cancel entirely without deleting")
	}
	if _, err := os.Stat(filepath.Join(d1, ".awsprofile")); err != nil {
		t.Fatal("d1's link should survive a cancelled replace")
	}

	// 'y' with links present unlinks + deletes (not a no-op, not a bare delete
	// that would strand the pointer file).
	nm, _ = v.Update(tea.KeyMsg{Type: tea.KeyDelete})
	nm, _ = nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	av = nm.(awsView)
	if av.confirming || len(av.profiles) != 1 || av.profiles[0].Name != "personal" {
		t.Fatalf("'y' should unlink+delete the linked profile: profiles=%+v confirming=%v", av.profiles, av.confirming)
	}
	if _, err := os.Stat(filepath.Join(d1, ".awsprofile")); !os.IsNotExist(err) {
		t.Fatal("'y' fast-path must also remove the linked pointer file, not strand it")
	}
}
