# TUI Unified Tab + Action Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One tab implementation for all four providers with a never-hide action model — Azure folds into the shared `providerTabView`, every verb gets one key everywhere, Remove confirms everywhere, Capture becomes contextual, "pin" language becomes "link", and the scope icons shrink to two dots plus a `⌁ default` tag.

**Architecture:** The Azure tab (`internal/ui/model.go`, ~1050 lines) and the shared provider view (`internal/ui/provider_view.go`) are two implementations of the same screen and have drifted. This plan first upgrades the shared view (disabled-action support, shared confirm, contextual capture), then deletes the Azure `Model` and replaces it with a thin `azureView` wrapper (drift notice + `e` envrc hotkey are the only Azure-specific parts). Presentation semantics (scope marks, Linked row, `?` overlay) land on the single surviving implementation.

**Tech Stack:** Go, Bubble Tea, lipgloss (all already vendored — no new dependencies).

**Scope note:** This is Plan 1 of the TUI UX redesign spec (`docs/superpowers/specs/2026-07-07-tui-ux-redesign-design.md`). The spec's "phase 1" splits into this plan and a follow-up dashboard/expiry plan. `azrl shell` (`t`), `azrl console` (`c`), mouse support, and dashboard verbs-on-rows are later plans — do NOT add them here.

## Global Constraints

- **No new dependencies.** bubblezone is for the later mouse phase, not this plan.
- **Never hide — disable with reason.** Every verb is always listed; an inapplicable one renders dim with its reason as the hint. The only exception: the empty state (zero profiles) shows exactly the two bootstrap verbs (New profile, Capture session).
- **One keymap:** `s` Sign in · `u` Link here · `n` New profile · `b` Browser profile · `delete` Remove (confirm) · `a` Capture (empty state only) · `r`/`f5` refresh · `?` help overlay (container) · `e` write .envrc (Azure only) · `q`/`ctrl+c` quit. Same keys on every tab.
- **UI language:** "link"/"linked", never "pin", in every label, hint, status message, and code comment touched by this plan. Internal file names (`.azprofile`, mappings TSV) do not change.
- **Azure CLI verbs are top-level** (`azrl login`, `azrl capture`), not under an `azure` group: `cliGroup("azure")` must return `""` and `groupArgs("")` must return the rest unchanged.
- **The ghrl promoted binary must keep working:** `groupArgs`'s `gh` special case (drops the group when the executable is ghrl) stays untouched.
- **Every task ends green:** `go build ./... && gofmt -l . && go vet ./... && go test ./...` (gofmt output must be empty).
- Commit messages: conventional commits with scope, each ending with the trailer line `Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq`.

## File Structure

- `internal/ui/radio.go` — gains `disabled` on `radioOption` (Task 1)
- `internal/ui/provider_view.go` — the one tab implementation: `enabledActions`, shared confirm, `namingVerb`, `notice`/`identityOverride` header fields (Tasks 2–4, 6–8)
- `internal/ui/aws_view.go`, `gcp_view.go`, `github_view.go` — shrink to `providerActions(<group>)` one-liners (Task 2)
- `internal/ui/frame.go` — NEW: shared frame renderers extracted verbatim from model.go (Task 5)
- `internal/ui/azure_view.go` — NEW: thin Azure wrapper (drift check, `e` hotkey) (Task 6)
- `internal/ui/model.go`, `model_test.go` — DELETED (Task 6); tests port to `azure_view_test.go`
- `internal/ui/actions.go` — loses `runUse`/`runDelete`/`runEdit`/`runRelabel`/`editorCmd`/`handoffArgs` (Task 6)
- `internal/ui/styles.go`, `panes.go` — scope slot/legend/info block changes (Tasks 7–8)
- `internal/ui/tabs.go` — `?` help overlay (Task 9)
- `CLAUDE.md`, `README.md` — docs (Task 10)

---

### Task 1: radio — disabled options

**Files:**
- Modify: `internal/ui/radio.go`
- Test: `internal/ui/radio_test.go`

**Interfaces:**
- Produces: `radioOption{label, key, hint string, disabled bool}` — later tasks construct options with `disabled: true` and expect the view to render them dim.

- [ ] **Step 1: Write the failing test** (append to `internal/ui/radio_test.go`)

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestRadioViewRendersDisabledRows -v`
Expected: FAIL — `unknown field disabled in struct literal`

- [ ] **Step 3: Implement**

In `internal/ui/radio.go`, change `radioOption` (lines 10–14) to:

```go
// radioOption is one selectable row in a radio group.
type radioOption struct {
	label    string // human-facing action name
	key      string // single-rune hotkey accelerator (e.g. "l"); empty for none
	hint     string // short trailing note shown muted (optional)
	disabled bool   // rendered dim; the hint carries the reason it can't apply
}
```

In `view` (line 73), replace the label-style block:

```go
		labelStyle := lipgloss.NewStyle().Foreground(white)
		if o.disabled {
			labelStyle = mutedStyle
		}
		if i == r.cursor && r.focused {
			// Selection shows only while this (deepest) level holds focus; a
			// disabled row under the cursor takes the dim parent shade so it
			// reads as "selected, but not runnable".
			labelStyle = selBlockActive
			if o.disabled {
				labelStyle = selBlockParent
			}
		}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/ -run TestRadio -v`
Expected: PASS (all radio tests)

- [ ] **Step 5: Full verification and commit**

Run: `go build ./... && gofmt -l . && go vet ./... && go test ./...`
Expected: all green, gofmt silent.

```bash
git add internal/ui/radio.go internal/ui/radio_test.go
git commit -m "feat(ui): radio rows can render disabled with a reason hint

Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq"
```

---

### Task 2: never-hide action model on the shared view

**Files:**
- Modify: `internal/ui/provider_view.go`
- Modify: `internal/ui/aws_view.go`, `internal/ui/gcp_view.go`, `internal/ui/github_view.go`
- Test: `internal/ui/aws_view_test.go`, `internal/ui/gcp_view_test.go`

**Interfaces:**
- Consumes: `radioOption.disabled` (Task 1).
- Produces: `providerActions(group string) []providerAction` (the shared verb set; Task 6 calls `providerActions("")` for Azure); `actionState{providerAction; enabled bool}`; `(providerTabView) enabledActions() []actionState`; `providerAction.bootstrap bool`; `newProviderTabView(prov, actions)` (header param removed). Keys change: New profile moves `a` → `n`.

- [ ] **Step 1: Write the failing tests**

In `internal/ui/aws_view_test.go`, DELETE `TestUseHereHiddenWhenSelectedProfilePinsCwd` and `TestSignInHiddenWhenSessionLive`, and add:

```go
func TestLinkHereDisabledWhenAlreadyLinked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)
	linked := t.TempDir()
	os.WriteFile(filepath.Join(linked, ".awsprofile"), []byte("work\n"), 0o644)
	t.Chdir(linked)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	av := nm.(awsView)
	out := av.View()
	// Never hidden: the verb stays listed, disabled, with its reason.
	if !strings.Contains(out, "Link here") || !strings.Contains(out, "already linked here") {
		t.Fatalf("Link here should render disabled with its reason:\n%s", out)
	}
	if !strings.Contains(out, "ACTIONS (5)") {
		t.Fatalf("action count must not drop when a verb is disabled:\n%s", out)
	}
	// The accelerator explains instead of running.
	nm, cmd := av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if cmd != nil {
		t.Fatal("disabled accelerator must not run")
	}
	if !strings.Contains(nm.(awsView).status, "already linked here") {
		t.Fatalf("disabled accelerator should surface the reason, got %q", nm.(awsView).status)
	}
}

func TestSignInVisibleWithLiveSessionHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	v.statuses["work"] = provider.Status{ProfileName: "work", Identity: "123/Admin"}
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	av := nm.(awsView)
	out := av.View()
	if !strings.Contains(out, "Sign in") || !strings.Contains(out, "re-auth anyway") {
		t.Fatalf("Sign in must stay visible for a live session, with the swapped hint:\n%s", out)
	}
	// Still runnable — re-auth is idempotent.
	_, cmd := av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("Sign in on a live session must still return the handoff command")
	}
}

func TestRefreshKeysReload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)

	v := newAwsView()
	os.WriteFile(filepath.Join(ap, "late.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)
	nm, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if len(nm.(awsView).profiles) != 1 {
		t.Fatal("'r' did not reload the profile list")
	}
}
```

Also update existing tests in the same file:
- `TestAwsViewRendersProfilesAndActions`: in the `for _, want := range` list, replace `"Use here"` with `"Link here"`.
- `TestNewProfilePromptsForNameThenExecsCreate`: replace both `[]rune("a")` with `[]rune("n")` (New profile key is now `n`).
- `TestEmptyProviderShowsOnlyBootstrapAction`: replace both `[]rune("a")` with `[]rune("n")`, and in the `hidden` loop replace `"Use here"` with `"Link here"`. (Task 4 revisits this test for the Capture pair.)

In `internal/ui/gcp_view_test.go`, `TestGcpViewRendersProfilesAndActions`: replace `"Use here"` with `"Link here"` in the want list.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestLinkHere|TestSignInVisible|TestRefreshKeys|TestAwsViewRenders|TestNewProfilePrompts|TestEmptyProvider|TestGcpViewRenders' -v`
Expected: FAIL (labels/keys/behavior not implemented yet)

- [ ] **Step 3: Implement in `internal/ui/provider_view.go`**

3a. Change `providerAction` and add the shared set + `actionState`:

```go
// providerAction is one entry in a provider tab's action pane. run mutates the
// view in place to reflect the action's outcome (status line, reloaded list)
// and may return a command (e.g. a runHandoff exec) for interactive flows.
type providerAction struct {
	key, label string
	hint       string // short muted description shown beside the label when it fits
	run        func(v *providerTabView) tea.Cmd
	bootstrap  bool // offered in the empty state (onboarding verbs)
}

// actionState is one action resolved against the current selection: always
// listed; disabled (with the reason swapped into the hint) when it can't apply.
type actionState struct {
	providerAction
	enabled bool
}

// providerActions is the shared verb set every tab offers. group is the azrl
// CLI command group the interactive verbs exec ("gh" for GitHub, "" for
// Azure's top-level verbs).
func providerActions(group string) []providerAction {
	return []providerAction{
		{key: "s", label: "Sign in", hint: "session only — no link", run: loginAction(group)},
		{key: "u", label: "Link here", hint: "link this dir — no login", run: useAction},
		{key: "n", label: "New profile", hint: "sign in + link this dir", run: newProfileAction, bootstrap: true},
		{key: "b", label: "Browser profile", hint: "map to a local browser profile", run: browserAction},
		{key: "delete", label: "Remove…", hint: "delete profile", run: removeAction},
	}
}
```

3b. Replace `visibleActions` (lines 113–139) with:

```go
// enabledActions resolves the action set against the current selection.
// Nothing is ever hidden: a verb that can't apply is listed disabled with its
// reason as the hint. The empty state is the one exception — only the
// bootstrap (onboarding) verbs show.
func (v providerTabView) enabledActions() []actionState {
	if len(v.profiles) == 0 {
		var out []actionState
		for _, a := range v.actions {
			if a.bootstrap {
				out = append(out, actionState{providerAction: a, enabled: true})
			}
		}
		return out
	}
	sel := v.selected()
	out := make([]actionState, 0, len(v.actions))
	for _, a := range v.actions {
		st := actionState{providerAction: a, enabled: true}
		switch a.key {
		case "u":
			if sel != "" && sel == v.dirProfile && v.dirScope == ScopeCwd {
				st.enabled = false
				st.hint = "already linked here"
			}
		case "s":
			if sel != "" && sessionLive(v.statuses[sel]) {
				// Still runnable — re-auth is idempotent — but say why it's optional.
				st.hint = "session live · re-auth anyway"
			}
		}
		out = append(out, st)
	}
	return out
}
```

3c. In `update`'s key handling, replace every `v.visibleActions()` reference:

- the `down` case bound: `if v.actionCur < len(v.enabledActions())-1 {`
- the `enter` case:

```go
		case "enter":
			// Selecting a profile opens the action pane; enter there runs the action.
			if v.focus == focusProfiles {
				v.focus = focusActions
			} else if acts := v.enabledActions(); v.actionCur < len(acts) {
				a := acts[v.actionCur]
				if !a.enabled {
					v.status = mutedStyle.Render(a.hint)
					return v, nil
				}
				return v.dispatch(a.key)
			}
```

- add refresh keys and change the accelerator default:

```go
		case "f5", "r":
			v.reload()
		default:
			// An accelerator key selects its action and runs it; a disabled
			// action's key explains itself in the status line instead.
			for i, a := range v.enabledActions() {
				if a.key == msg.String() {
					v.actionCur = i
					if !a.enabled {
						v.status = mutedStyle.Render(a.hint)
						return v, nil
					}
					return v.dispatch(a.key)
				}
			}
```

- `clampAction`: `if n := len(v.enabledActions()); v.actionCur >= n && n > 0 {`

3d. In `View`, build the radio from action states (replace the `acts := v.visibleActions()` block):

```go
	acts := v.enabledActions()
	opts := make([]radioOption, len(acts))
	for i, a := range acts {
		opts[i] = radioOption{label: a.label, key: a.key, hint: a.hint, disabled: !a.enabled}
	}
```

3e. Remove the dead pre-rendered header: delete the `header string` field from `providerTabView`, change the constructor to `func newProviderTabView(prov provider.Provider, actions []providerAction) providerTabView` (drop the `header` param and the `header: header` assignment), and update the doc comment on `providerTabView` to say the tabs "differ only in their provider and CLI command group".

3f. Shrink the three wrappers. `internal/ui/aws_view.go` becomes:

```go
package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/aws"
)

// awsView is the AWS provider tab — the shared view with the aws CLI group.
type awsView struct{ providerTabView }

func newAwsView() awsView {
	return awsView{newProviderTabView(aws.NewProvider(), providerActions("aws"))}
}

func (v awsView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	return v, cmd
}
```

`gcp_view.go` and `github_view.go` are identical in shape with `gcp.NewProvider(), providerActions("gcp")` and `github.NewProvider(), providerActions("gh")` (keep their existing type names, comments trimmed the same way).

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/ui/ -v 2>&1 | tail -20`
Expected: PASS. (`model_test.go` still passes untouched — the Azure `Model` keeps its old behavior until Task 6.)

- [ ] **Step 5: Full verification and commit**

Run: `go build ./... && gofmt -l . && go vet ./... && go test ./...`

```bash
git add internal/ui/provider_view.go internal/ui/aws_view.go internal/ui/gcp_view.go internal/ui/github_view.go internal/ui/aws_view_test.go internal/ui/gcp_view_test.go
git commit -m "feat(ui): never-hide action model — disabled verbs explain themselves

Sign in stays visible on live sessions (hint swaps to re-auth), Link here
disables with its reason on the already-linked selection, New profile moves
to 'n', r/f5 reload, and the dead pre-rendered header field is gone.

Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq"
```

---

### Task 3: Remove confirms on every tab

**Files:**
- Modify: `internal/ui/provider_view.go`
- Test: `internal/ui/aws_view_test.go`

**Interfaces:**
- Consumes: `newRadio`, `radio` (Task 1 state), `actionState` (Task 2).
- Produces: `providerTabView.confirming bool`, `.pendingDelete string`, `.confirm radio`, `(providerTabView) doRemove() (providerTabView, tea.Cmd)`. Task 6's Azure wrapper relies on `capturesInput()` returning true while confirming.

- [ ] **Step 1: Write the failing test** (replace `TestProviderViewDeleteKeyRemoves` in `internal/ui/aws_view_test.go`)

```go
func TestProviderViewRemoveConfirms(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	// delete arms the confirm — nothing is removed yet.
	nm, _ = nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyDelete})
	av := nm.(awsView)
	if !av.confirming || av.pendingDelete != "work" {
		t.Fatalf("delete should arm the confirm (confirming=%v pending=%q)", av.confirming, av.pendingDelete)
	}
	out := av.View()
	if !strings.Contains(out, "CONFIRM") || !strings.Contains(out, "work") || !strings.Contains(out, ".awsprofile") {
		t.Fatalf("confirm pane should name the profile and its pointer file:\n%s", out)
	}
	if len(av.profiles) != 1 {
		t.Fatal("arming the confirm must not delete")
	}
	// 'n' cancels.
	nm, _ = av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	av = nm.(awsView)
	if av.confirming || len(av.profiles) != 1 {
		t.Fatal("'n' should cancel without deleting")
	}
	// delete + 'y' removes.
	nm, _ = av.Update(tea.KeyMsg{Type: tea.KeyDelete})
	nm, _ = nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	av = nm.(awsView)
	if !strings.Contains(av.status, "removed") || len(av.profiles) != 0 {
		t.Fatalf("'y' should remove (status=%q, %d profiles)", av.status, len(av.profiles))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestProviderViewRemoveConfirms -v`
Expected: FAIL — `av.confirming undefined`

- [ ] **Step 3: Implement in `internal/ui/provider_view.go`**

3a. Add fields to `providerTabView` (after `nameInput textinput.Model`):

```go
	confirming    bool
	pendingDelete string
	confirm       radio
```

3b. Replace `removeAction` and add `doRemove`:

```go
// removeAction arms the shared confirm dialog for the selected profile —
// nothing is deleted until the user confirms.
func removeAction(v *providerTabView) tea.Cmd {
	name := v.selected()
	if name == "" {
		v.status = mutedStyle.Render("no profile selected")
		return nil
	}
	v.confirming = true
	v.pendingDelete = name
	v.confirm = newRadio([]radioOption{
		{label: "No, keep it"},
		{label: "Yes, remove " + name},
	})
	v.confirm.focused = true
	return nil
}

// doRemove deletes the pending profile and reloads the list.
func (v providerTabView) doRemove() (providerTabView, tea.Cmd) {
	name := v.pendingDelete
	v.confirming = false
	v.pendingDelete = ""
	dir := v.prov.ProfilesDir()
	pwd, _ := os.Getwd()
	if _, err := v.prov.Remove(name, dir, pwd); err != nil {
		v.status = failureStyle.Render(err.Error())
	} else {
		v.status = successStyle.Render(fmt.Sprintf("removed %q", name))
		v.reload()
	}
	return v, nil
}
```

3c. Route confirm keys first — in `update`, immediately after `case tea.KeyMsg:` (before the `v.browserPick != nil` block):

```go
		if v.confirming {
			switch msg.String() {
			case "ctrl+c":
				return v, tea.Quit
			case "esc", "n", "q":
				v.confirming = false
				v.pendingDelete = ""
			case "y":
				return v.doRemove()
			case "up", "k", "left":
				v.confirm.up()
			case "down", "j", "right":
				v.confirm.down()
			case "enter":
				if v.confirm.cursor == 1 {
					return v.doRemove()
				}
				v.confirming = false
				v.pendingDelete = ""
			}
			return v, nil
		}
```

3d. `capturesInput` gains the confirm state (the container must not switch tabs mid-confirm):

```go
func (v providerTabView) capturesInput() bool {
	return v.namingVerb != "" || v.browserManual || v.browserPick != nil || v.confirming
}
```

NOTE: at this task the field is still `v.naming` — write `return v.naming || v.browserManual || v.browserPick != nil || v.confirming` here; Task 4 renames it.

3e. In `View`, render the confirm as the right column — insert before the `right := paneTitle("DETAILS", …)` assignment, and use it instead when confirming:

```go
	right := paneTitle("DETAILS", v.focus == focusActions) + "\n\n" +
		info + "\n\n" + rule(rightW) + "\n" +
		paneTitle(fmt.Sprintf("ACTIONS (%d)", len(acts)), v.focus == focusActions && !v.suspended) + "\n\n" + actionsBody
	if v.confirming {
		right = paneTitle("CONFIRM", true) + "\n\n" +
			mutedStyle.Render("Removes its conf, token dir,\nand this dir's "+v.prov.Scheme().Pointer+".") + "\n\n" +
			v.confirm.view(rightW)
	}
```

3f. Confirm-mode help bar — in `View`, replace the `help := keyHelpFit(…)` assignment with:

```go
	help := keyHelpFit(contentW,
		[]string{"↑↓", "select", "↵", "open/run", "esc", "back"},
		[]string{"q", "quit", "→", "details", "⇥", "tab", "d", "dir", "o", "options"})
	if v.confirming {
		help = keyHelp("↑↓", "choose", "↵", "confirm", "y", "yes", "n/esc", "cancel")
	}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/ -v 2>&1 | tail -15`
Expected: PASS

- [ ] **Step 5: Full verification and commit**

Run: `go build ./... && gofmt -l . && go vet ./... && go test ./...`

```bash
git add internal/ui/provider_view.go internal/ui/aws_view_test.go
git commit -m "feat(ui): Remove asks for confirmation on every provider tab

Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq"
```

---

### Task 4: contextual Capture + naming generalization

**Files:**
- Modify: `internal/ui/provider_view.go`
- Test: `internal/ui/aws_view_test.go`

**Interfaces:**
- Consumes: `providerActions` (Task 2), `groupArgs`, `cliGroup`, `runHandoff` (existing, `internal/ui/actions.go`).
- Produces: `providerTabView.namingVerb string` (replaces `naming bool`; `""`/`"login"`/`"capture"`), `captureAction(v *providerTabView) tea.Cmd`, and the `a` Capture entry in `providerActions`. Empty state now shows New profile + Capture session.

- [ ] **Step 1: Write the failing tests**

In `internal/ui/aws_view_test.go`, replace `TestEmptyProviderShowsOnlyBootstrapAction` with:

```go
func TestEmptyProviderOffersOnboardingPair(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".aws-profiles"), 0o755)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	out := nm.(awsView).View()
	if !strings.Contains(out, "ACTIONS (2)") ||
		!strings.Contains(out, "New profile") || !strings.Contains(out, "Capture session") {
		t.Fatalf("empty provider should offer New profile + Capture session:\n%s", out)
	}
	for _, hidden := range []string{"Sign in", "Link here", "Remove"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("%q should not show with zero profiles:\n%s", hidden, out)
		}
	}
	// 'a' opens the capture name prompt with an adopt-flavored confirm hint.
	nm, _ = nm.(awsView).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	av := nm.(awsView)
	if av.namingVerb != "capture" {
		t.Fatalf("'a' should open the capture prompt, namingVerb=%q", av.namingVerb)
	}
	if !strings.Contains(av.View(), "adopt session + link") {
		t.Fatalf("capture prompt missing its confirm hint:\n%s", av.View())
	}
	// enter with the placeholder name execs the capture handoff.
	nm, cmd := av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if nm.(awsView).namingVerb != "" || cmd == nil {
		t.Fatal("enter should close the prompt and exec the capture handoff")
	}
}

func TestCaptureAbsentFromNonEmptyActionList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	out := nm.(awsView).View()
	if strings.Contains(out, "Capture session") {
		t.Fatalf("Capture is onboarding-contextual; it must not sit in the everyday list:\n%s", out)
	}
	if !strings.Contains(out, "ACTIONS (5)") {
		t.Fatalf("everyday action count should be 5:\n%s", out)
	}
}
```

Also update in the same file:
- `TestNewProfilePromptsForNameThenExecsCreate`: replace the two `av.naming` reads with `av.namingVerb != ""` (e.g. `if av.namingVerb == "" || cmd != nil {` for the arm assertion, `if av.namingVerb != "" || cmd == nil {` after enter, and the final esc assertion becomes `if nm.(awsView).namingVerb != "" || cmd != nil {`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestEmptyProviderOffers|TestCaptureAbsent|TestNewProfilePrompts' -v`
Expected: FAIL — `av.namingVerb undefined`

- [ ] **Step 3: Implement in `internal/ui/provider_view.go`**

3a. Rename the field: `naming bool` → `namingVerb string` (empty = no prompt). Update `capturesInput` (`v.namingVerb != ""`), and every `v.naming` reference.

3b. `newProfileAction` sets the verb; add `captureAction`:

```go
// newProfileAction opens the name prompt; confirming execs `login <name>
// --yes`, whose create path signs in and links the current directory.
func newProfileAction(v *providerTabView) tea.Cmd {
	pwd, _ := os.Getwd()
	ti := textinput.New()
	ti.Placeholder = profile.DefaultName("", pwd)
	ti.Focus()
	v.nameInput = ti
	v.namingVerb = "login"
	v.status = ""
	return nil
}

// captureAction opens the name prompt for adopting the CLI's current session;
// confirming execs `capture <name>`, which records the session's metadata as
// a profile and links the current directory. Onboarding-contextual: offered
// only in the empty state (and via the dashboard's adopt flow).
func captureAction(v *providerTabView) tea.Cmd {
	pwd, _ := os.Getwd()
	ti := textinput.New()
	ti.Placeholder = profile.DefaultName("", pwd)
	ti.Focus()
	v.nameInput = ti
	v.namingVerb = "capture"
	v.status = ""
	return nil
}
```

3c. Add the Capture entry to `providerActions` (after the New profile entry):

```go
		{key: "a", label: "Capture session", hint: "adopt current CLI session · links this dir", run: captureAction, bootstrap: true},
```

3d. Skip it outside the empty state — in `enabledActions`' non-empty loop, first line of the loop body:

```go
		if a.key == "a" {
			// Capture is onboarding-contextual: empty state + dashboard adopt only.
			continue
		}
```

3e. The naming key handler routes by verb (replace the `if v.naming { … }` block's `enter` case):

```go
			case "enter":
				name := strings.TrimSpace(v.nameInput.Value())
				if name == "" {
					name = v.nameInput.Placeholder
				}
				name = profile.SanitizeName(name)
				if name == "" {
					return v, nil
				}
				verb := v.namingVerb
				v.namingVerb = ""
				v.status = ""
				if verb == "capture" {
					return v, runHandoff(groupArgs(cliGroup(v.prov.Name()), "capture", name))
				}
				return v, runHandoff(groupArgs(cliGroup(v.prov.Name()), "login", name, "--yes"))
```

(the `esc` case becomes `v.namingVerb = ""`.)

3f. Verb-aware prompt in `View` (replace the `if v.naming { actionsBody = … }` block):

```go
	if v.namingVerb != "" {
		prompt, confirmHint := "Name for the new profile:", "create + sign in + link"
		if v.namingVerb == "capture" {
			prompt, confirmHint = "Name for the captured profile:", "adopt session + link"
		}
		actionsBody = mutedStyle.Render(prompt) + "\n\n" +
			v.nameInput.View() + "\n\n" +
			keyHelp("↵", confirmHint, "esc", "cancel")
	}
```

3g. Empty-list hint in `renderProfilePane` (`internal/ui/model.go` until Task 5 moves it) — replace the `(none yet — …)` line with:

```go
		b.WriteString(mutedStyle.Render("  (none yet — ") + keycap("n") + mutedStyle.Render(" creates, ") + keycap("a") + mutedStyle.Render(" adopts)"))
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/ -v 2>&1 | tail -15`
Expected: PASS

- [ ] **Step 5: Full verification and commit**

Run: `go build ./... && gofmt -l . && go vet ./... && go test ./...`

```bash
git add internal/ui/provider_view.go internal/ui/model.go internal/ui/aws_view_test.go
git commit -m "feat(ui): contextual Capture — onboarding pair in the empty state

Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq"
```

---

### Task 5: extract the shared frame renderers

**Files:**
- Create: `internal/ui/frame.go`
- Modify: `internal/ui/model.go` (remove the moved functions), `internal/ui/provider_view.go` (drop the `Model` dependency)

**Interfaces:**
- Produces: `internal/ui/frame.go` holding — moved VERBATIM from `model.go`, bodies unchanged — `paneDims`, `renderPaneFrame`, `renderProfilePane`, `joinColumns`, `padTo`, `paneTitle`, `rule`. Task 6 deletes what's left of `model.go`.

- [ ] **Step 1: Create `internal/ui/frame.go`**

Package clause + imports (`fmt`, `strings`, `github.com/charmbracelet/lipgloss`, `github.com/slamb2k/azrl/internal/profile`), then cut-paste these functions from `model.go` with their doc comments, bodies byte-identical: `paneDims` (model.go:296–315), `renderPaneFrame` (:796–836), `renderProfilePane` (:838–903, including the Task 4 hint edit), `joinColumns` (:905–936), `padTo` (:938–944), `paneTitle` (:993–1000), `rule` (:1002–1007). Do NOT move `rightPane`, `Model.dims`, or anything else — those stay in model.go and die with it in Task 6.

- [ ] **Step 2: Break the provider view's dependency on `Model`**

In `internal/ui/provider_view.go` `View()`, replace:

```go
	_, leftW, rightW, _ := (Model{width: v.width, height: v.height}).dims()
```

with:

```go
	_, leftW, rightW := paneDims(v.width)
```

- [ ] **Step 3: Verify green — a pure move changes nothing**

Run: `go build ./... && gofmt -l . && go vet ./... && go test ./...`
Expected: all green, zero test edits needed.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/frame.go internal/ui/model.go internal/ui/provider_view.go
git commit -m "refactor(ui): extract shared frame renderers into frame.go

Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq"
```

---

### Task 6: fold the Azure tab into the shared view

**Files:**
- Create: `internal/ui/azure_view.go`, `internal/ui/azure_view_test.go`
- Delete: `internal/ui/model.go`, `internal/ui/model_test.go`
- Modify: `internal/ui/provider_view.go` (notice/identityOverride fields), `internal/ui/actions.go` (dead helpers), `internal/ui/actions_test.go`, `internal/ui/tabs.go` (buildTabs), `internal/ui/tabs_test.go` (renaming test)

**Interfaces:**
- Consumes: everything from Tasks 1–5; `providerActions("")`; `runWriteEnvrc`, `runHandoff`, `groupArgs` (actions.go); `azure.AccountShowIn`, `azure.QualifiedIdentity`, `profile.Resolve`, `profile.LocateAzprofile`, `profile.HasEnvrc`.
- Produces: `azureView` (registered in `buildTabs`), `providerTabView.notice string` + `.identityOverride string` (rendered by the shared `identityStrip`), `cliGroup("azure") == ""`, `groupArgs("") == rest`.

- [ ] **Step 1: Write the failing tests** — create `internal/ui/azure_view_test.go`:

```go
package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/browserpick"
)

// seedAzure returns a sized azure view with one profile on disk.
func seedAzure(t *testing.T) azureView {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	os.WriteFile(filepath.Join(home, ".azure-profiles", "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)
	v := newAzureView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	return nm.(azureView)
}

func TestAzureViewRendersUnifiedActions(t *testing.T) {
	v := seedAzure(t)
	out := v.View()
	for _, want := range []string{"Azure", "PROFILES (1)", "acme", "Sign in", "Link here", "New profile", "Browser profile", "Remove"} {
		if !strings.Contains(out, want) {
			t.Fatalf("azure view missing %q:\n%s", want, out)
		}
	}
	// Retired TUI verbs and the demoted everyday Capture must be gone.
	for _, gone := range []string{"Edit", "Rename", "Capture session"} {
		if strings.Contains(out, gone) {
			t.Fatalf("azure view still offers %q:\n%s", gone, out)
		}
	}
}

func TestAzureSignInHotkeyReturnsHandoff(t *testing.T) {
	v := seedAzure(t)
	_, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Fatal("'s' should return the login handoff command")
	}
}

func TestAzureNewProfilePromptExecsCreateLogin(t *testing.T) {
	v := seedAzure(t)
	nm, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	av := nm.(azureView)
	if av.namingVerb != "login" {
		t.Fatalf("'n' should open the new-profile prompt, verb=%q", av.namingVerb)
	}
	for _, r := range "fresh" {
		nm, _ = av.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		av = nm.(azureView)
	}
	nm, cmd := av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if nm.(azureView).namingVerb != "" || cmd == nil {
		t.Fatal("enter should close the prompt and exec the create login")
	}
}

func TestAzureEmptyStateOffersOnboardingPair(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	v := newAzureView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	out := nm.(azureView).View()
	if !strings.Contains(out, "ACTIONS (2)") || !strings.Contains(out, "Capture session") {
		t.Fatalf("empty azure tab should offer the onboarding pair:\n%s", out)
	}
}

func TestAzureDriftNoticeMentionsEnvrc(t *testing.T) {
	v := seedAzure(t)
	nm, _ := v.Update(identityMsg{
		who:        "u@fiig.com.au · fiig.com.au",
		ambientWho: "u@fiig.com.au · velrada.com",
		drift:      true,
	})
	av := nm.(azureView)
	got := av.identityStrip()
	if !strings.Contains(got, ".envrc") {
		t.Fatalf("drift strip should offer .envrc: %q", got)
	}
	for _, want := range []string{"velrada.com", "expects u@fiig.com.au", "fiig.com.au"} {
		if !strings.Contains(got, want) {
			t.Fatalf("drift strip missing %q: %q", want, got)
		}
	}
	// Clearing drift clears the notice.
	nm, _ = av.Update(identityMsg{who: "u@fiig.com.au · fiig.com.au"})
	if strings.Contains(nm.(azureView).identityStrip(), ".envrc") {
		t.Fatal("no drift should not mention .envrc")
	}
}

func TestAzureEnvrcHotkeyNeedsProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	os.MkdirAll(filepath.Join(home, ".azure-profiles"), 0o755)
	t.Chdir(t.TempDir()) // no .azprofile anywhere up the tree
	v := newAzureView()
	nm, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if cmd != nil {
		t.Fatal("e without a resolved profile should not run a command")
	}
	if !strings.Contains(nm.(azureView).status, "no profile") {
		t.Fatalf("expected a 'no profile' status, got %q", nm.(azureView).status)
	}
}

func TestAzureBrowserActionOpensPickerAndWritesKeys(t *testing.T) {
	v := seedAzure(t)
	confPath := filepath.Join(os.Getenv("HOME"), ".azure-profiles", "acme.conf")
	nm, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	av := nm.(azureView)
	if av.browserFor != "acme" || cmd == nil {
		t.Fatalf("'b' did not arm discovery (for=%q)", av.browserFor)
	}
	nm, _ = av.Update(browserProfilesMsg{forProfile: "acme", profiles: []browserpick.Profile{
		{Browser: "edge", OS: "linux", Dir: "Profile 2", Name: "Work", Email: "simon@acme.com"},
	}})
	av = nm.(azureView)
	if av.browserPick == nil {
		t.Fatal("browser profiles msg did not open the picker")
	}
	nm, _ = av.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if nm.(azureView).browserPick != nil {
		t.Fatal("picker should close after enter")
	}
	b, _ := os.ReadFile(confPath)
	if !strings.Contains(string(b), `AZ_BROWSER_CMD=microsoft-edge --profile-directory="Profile 2"`) ||
		!strings.Contains(string(b), "AZ_BROWSER_LABEL=Edge — Work") {
		t.Fatalf("keys not written:\n%s", b)
	}
}

func TestGroupArgsTopLevel(t *testing.T) {
	if got := strings.Join(groupArgs("", "login", "acme"), " "); got != "login acme" {
		t.Fatalf("groupArgs(\"\") = %q", got)
	}
	if got := cliGroup("azure"); got != "" {
		t.Fatalf("cliGroup(azure) = %q, want \"\"", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/ -run TestAzure -v`
Expected: FAIL — `newAzureView undefined`

- [ ] **Step 3: Add the shared header hooks** — in `internal/ui/provider_view.go`:

3a. Fields on `providerTabView` (after `confirm radio`):

```go
	// notice is an optional extra header line (e.g. Azure's drift warning);
	// identityOverride, when set, replaces the dir-linked profile's disk
	// identity in the header (Azure's live az-account-show result is fresher).
	notice           string
	identityOverride string
```

3b. `identityStrip` becomes:

```go
// identityStrip is the standard provider header: icon + title, the current
// directory, the effective identity there, and an optional notice line.
func (v providerTabView) identityStrip() string {
	pwd, _ := os.Getwd()
	contentW, _, _ := paneDims(v.width)
	dirIdentity := v.statuses[v.dirProfile].Identity
	if v.identityOverride != "" {
		dirIdentity = v.identityOverride
	}
	strip := headerStrip(contentW, providerIcon(v.prov.Name()), v.prov.Title(), pwd,
		effectiveIdentity(v.dirProfile, dirIdentity, v.ambIdent))
	if v.notice != "" {
		strip += "\n" + ansi.Wordwrap(v.notice, contentW, "")
	}
	return strip
}
```

Add `"github.com/charmbracelet/x/ansi"` to provider_view.go's imports.

- [ ] **Step 4: Create `internal/ui/azure_view.go`**

```go
package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
)

// accountShowFn is overridable in tests; it reports the az identity for a
// specific profile config dir.
var accountShowFn = azure.AccountShowIn

// azureView is the Azure provider tab: the shared providerTabView plus the
// live drift check (az account show against the dir-linked profile's isolated
// config dir vs the ambient session) and the `e` write-.envrc recovery hotkey.
type azureView struct {
	providerTabView
	signedIn     string
	ambientWho   string
	drift        bool
	ambientEmpty bool
}

func newAzureView() azureView {
	return azureView{providerTabView: newProviderTabView(azure.NewProvider(), providerActions(""))}
}

// identityMsg carries the signed-in identity of this dir's profile session
// ("" when that profile has no live session), the ambient session's identity,
// and whether they differ with no .envrc linking them together (drift). Both
// identities are tenant-qualified, so a B2B guest signed into two tenants
// with one UPN still reads as drift.
type identityMsg struct {
	who          string
	ambientWho   string
	drift        bool
	ambientEmpty bool
}

// identityCmd reads the account from the resolved profile's token dir, so the
// strip reflects who you'd be in this dir — not the ambient ~/.azure session.
// When a profile is resolved but the ambient `az` shows a different identity
// and no .envrc links it, it flags drift so the UI can offer to write one.
func identityCmd() tea.Cmd {
	pwd, _ := os.Getwd()
	return func() tea.Msg {
		name, rErr := profile.Resolve("", pwd)
		dir := ""
		if rErr == nil {
			dir = filepath.Join(config.ProfilesDir(), name)
		}
		who := identityOf(accountShowFn(dir))
		msg := identityMsg{who: who}
		envrcDir := pwd
		if d, ok := profile.LocateAzprofile(pwd); ok {
			envrcDir = d
		}
		if rErr == nil && who != "" && !profile.HasEnvrc(envrcDir) {
			msg.ambientWho = identityOf(accountShowFn(""))
			msg.drift = msg.ambientWho != who
			msg.ambientEmpty = msg.ambientWho == ""
		}
		return msg
	}
}

// identityOf extracts the tenant-qualified identity from `az account show`
// output — the same composition the disk-only readers use, so comparisons
// stay tenant-aware (B2B guests share a UPN across tenants).
func identityOf(b []byte, err error) string {
	if err != nil {
		return ""
	}
	var a profile.AccountJSON
	if json.Unmarshal(b, &a) != nil {
		return ""
	}
	return azure.QualifiedIdentity(a.User.Name, a.TenantDefaultDomain, a.TenantID)
}

func (v azureView) Init() tea.Cmd { return identityCmd() }

// syncHeader projects the async drift state into the shared view's header
// fields: the freshest identity for the linked dir, plus the warning notice.
func (v *azureView) syncHeader() {
	v.identityOverride = v.signedIn
	v.notice = ""
	if v.drift {
		what := "is " + v.ambientWho
		if v.ambientEmpty {
			what = "has no active session"
		}
		v.notice = failureStyle.Render("⚠ shell az "+what+" — this dir expects "+v.signedIn) +
			mutedStyle.Render(" · ") + keycap("e") + mutedStyle.Render(" writes .envrc")
	}
}

func (v azureView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case identityMsg:
		v.signedIn = msg.who
		v.ambientWho = msg.ambientWho
		v.drift = msg.drift
		v.ambientEmpty = msg.ambientEmpty
		v.syncHeader()
		return v, nil
	case tea.KeyMsg:
		if msg.String() == "e" && !v.capturesInput() {
			pwd, _ := os.Getwd()
			if _, err := profile.Resolve("", pwd); err != nil {
				v.status = failureStyle.Render("✗ no profile here to link")
				return v, nil
			}
			v.status = ""
			return v, runWriteEnvrc()
		}
	}
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	switch msg.(type) {
	case cwdChangedMsg, opDoneMsg:
		// Disk state changed under us; re-check the live identity + drift.
		return v, tea.Batch(cmd, identityCmd())
	}
	return v, cmd
}
```

- [ ] **Step 5: Wire in and delete the old implementation**

5a. `internal/ui/tabs.go` line 67: `"azure": NewModel()` → `"azure": newAzureView()`.

5b. In `internal/ui/provider_view.go`, change `cliGroup`:

```go
// cliGroup maps a provider name to its azrl command group ("" = the verbs
// sit at the top level, as Azure's do).
func cliGroup(name string) string {
	switch name {
	case "github":
		return "gh"
	case "azure":
		return ""
	}
	return name
}
```

And in `internal/ui/actions.go`, `groupArgs` gains the top-level case as its first statement:

```go
func groupArgs(group string, rest ...string) []string {
	if group == "" {
		return rest
	}
	if group == "gh" {
		if self, err := os.Executable(); err == nil &&
			strings.TrimSuffix(filepath.Base(self), ".exe") == "ghrl" {
			return rest
		}
	}
	return append([]string{group}, rest...)
}
```

Also in `loginAction` no change is needed — `groupArgs("", "login")` now yields `["login"]`.

5c. Delete from `internal/ui/actions.go`: `runUse`, `runDelete`, `runEdit`, `runRelabel`, `editorCmd`, `handoffArgs` (keep `opDoneMsg`, `runWriteEnvrc`, `groupArgs`, `runHandoff`). Drop the now-unused `"github.com/slamb2k/azrl/internal/config"` import if nothing else uses it (`runWriteEnvrc` doesn't; check `os/exec` too — `runHandoff` still needs it).

5d. Delete from `internal/ui/actions_test.go`: `TestRunUseProducesMsg`, `TestRunDeleteProducesMsg` (keep `TestRunWriteEnvrc`).

5e. Delete `internal/ui/model.go` and `internal/ui/model_test.go` entirely (frame.go from Task 5 holds the shared renderers; azure_view.go holds the identity machinery; the ported tests live in azure_view_test.go).

5f. `internal/ui/tabs_test.go`: rewrite `TestTabsTabKeyForwardedWhileRenaming` as:

```go
func TestTabsTabKeyForwardedWhileNaming(t *testing.T) {
	m := seedTabs(t)
	// Move to the Azure tab and arm the new-profile text input ('n').
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	nm, _ = nm.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	tm := nm.(tabsModel)
	if tm.tabs[1].model.(azureView).namingVerb == "" {
		t.Fatal("'n' did not arm the name input")
	}
	// While naming, tab/d must reach the text input, not the container.
	nm2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyTab})
	if nm2.(tabsModel).active != 1 {
		t.Fatal("tab key switched tabs during naming")
	}
	nm3, _ := nm2.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if nm3.(tabsModel).picker != nil {
		t.Fatal("'d' opened the dir picker during naming")
	}
}
```

Fix any other `(Model)` assertions `grep -n "(Model)" internal/ui/*_test.go` surfaces the same way (assert on `azureView` state instead).

- [ ] **Step 6: Full verification**

Run: `go build ./... && gofmt -l . && go vet ./... && go test ./...`
Expected: all green. Also confirm nothing references the deleted symbols: `grep -rn "NewModel\|homeActions\|profileDelegate\|handoffArgs\|runRelabel\|runEdit\b\|runDelete\|runUse\b" internal/ cmd/ | grep -v _test.go` returns nothing.

- [ ] **Step 7: Commit**

```bash
git add -A internal/ui/
git commit -m "refactor(ui): fold the Azure tab into the shared provider view

Azure becomes a thin azureView wrapper (live drift notice + e envrc hotkey)
over providerTabView; Model and its duplicate keymap/actions/confirm die.
Edit and Rename retire from the TUI. Azure verbs exec at the CLI top level
via cliGroup(azure)=\"\".

Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq"
```

---

### Task 7: scope marks — two dots + a `⌁ default` tag

**Files:**
- Modify: `internal/ui/styles.go` (scopeSlot), `internal/ui/panes.go` (scopeLegend), `internal/ui/frame.go` (renderProfilePane tag), `internal/ui/provider_view.go` (rowScope, mappingDirs)
- Test: `internal/ui/aws_view_test.go`

**Interfaces:**
- Consumes: `profile.Mapping{Dir, Profile, Source}` (existing).
- Produces: `providerTabView.mappingDirs map[string][]string` (replaces `mapped map[string]bool`; Task 8 reads it); `scopeElsewhere` const deleted; `scopeSlot` renders an empty 3-space slot for anything but cwd/ancestor.

- [ ] **Step 1: Write the failing test** — replace `TestRenderProfilePaneScopeGlyphs` in `internal/ui/aws_view_test.go`:

```go
func TestRenderProfilePaneScopeMarks(t *testing.T) {
	profiles := []profile.Listed{
		{Name: "work", Detail: "acme.awsapps.com"},
		{Name: "staging", Detail: "acme.awsapps.com"},
		{Name: "personal", Detail: "personal.awsapps.com"},
		{Name: "idle", Detail: "idle.awsapps.com"},
	}
	scopes := map[string]string{"work": ScopeCwd, "staging": ScopeAncestor, "personal": scopeGlobal}
	out := renderProfilePane(profiles, 0, selActive, true, 44, scopes)
	// Linked-here and linked-via-parent carry the dot.
	if !strings.Contains(out, "●  work") || !strings.Contains(out, "●  staging") {
		t.Fatalf("linked profiles missing leading ● icon:\n%s", out)
	}
	// The ambient default is a tag, not a scope glyph.
	if !strings.Contains(out, "personal") || !strings.Contains(out, "⌁ default") || strings.Contains(out, "🌐") {
		t.Fatalf("default should render as a trailing tag, never 🌐:\n%s", out)
	}
	// Everything else gets a calm empty slot — no grey dots.
	if strings.Contains(out, "●  idle") || strings.Contains(out, "●  personal") {
		t.Fatalf("non-linked rows must not carry a dot:\n%s", out)
	}
	if !strings.Contains(out, "   idle") {
		t.Fatalf("non-linked row should keep the aligned empty slot:\n%s", out)
	}
}

func TestScopeLegendIsTwoDotsAndATag(t *testing.T) {
	l := scopeLegend(60)
	for _, want := range []string{"this dir", "parent dir", "⌁ default"} {
		if !strings.Contains(l, want) {
			t.Fatalf("legend missing %q: %q", want, l)
		}
	}
	for _, gone := range []string{"elsewhere", "unmapped", "🌐"} {
		if strings.Contains(l, gone) {
			t.Fatalf("legend still shows retired tier %q: %q", gone, l)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/ -run 'TestRenderProfilePaneScopeMarks|TestScopeLegend' -v`
Expected: FAIL

- [ ] **Step 3: Implement**

3a. `internal/ui/styles.go` — replace `scopeSlot` and the const block:

```go
// scopeGlobal extends the overview's Scope values for profile rows: the
// provider's global default (ambient identity match). It renders as a
// trailing "⌁ default" tag, never as a scope glyph — the icon slot means
// exactly one thing: relevance to this directory.
const scopeGlobal = "global"

// scopeSlot renders a profile row's leading icon as a fixed-width slot so
// names align. ● green: a link in the current dir makes this profile
// effective. ● orange: the link is inherited from a parent dir. Everything
// else gets an empty slot — no marker means not in play here.
func scopeSlot(scope string) string {
	switch scope {
	case ScopeCwd:
		return successStyle.Render("●") + "  "
	case ScopeAncestor:
		return lipgloss.NewStyle().Foreground(goldDeep).Render("●") + "  "
	}
	return "   "
}
```

(the `scopeElsewhere` const is deleted.)

3b. `internal/ui/provider_view.go` — replace the `mapped map[string]bool` field with `mappingDirs map[string][]string`, the reload block:

```go
	v.mappingDirs = map[string][]string{}
	for _, m := range v.prov.Scheme().ReadMappings(dir) {
		v.mappingDirs[m.Profile] = append(v.mappingDirs[m.Profile], m.Dir)
	}
```

and `rowScope`:

```go
// rowScope returns one profile row's relevance to the current directory —
// the closest link wins; the global default is tagged, not scoped; anything
// else renders an empty slot.
func (v providerTabView) rowScope(name string) string {
	if name == v.dirProfile {
		return v.dirScope
	}
	if name == v.active {
		return scopeGlobal
	}
	return ""
}
```

3c. `internal/ui/frame.go` `renderProfilePane` — the row's first line gains the tag; replace the two `b.WriteString` row lines with:

```go
		line := scopeSlot(scopes[p.Name]) + nameStyle.Render(truncateLine(p.Display(), textW))
		if scopes[p.Name] == scopeGlobal {
			line += "  " + mutedStyle.Render("⌁ default")
		}
		b.WriteString(line + "\n")
		b.WriteString("   " + detailStyle.Render(truncateLine(p.Detail, textW)) + "\n")
```

Also update the function's doc comment: the icon slot carries relevance-to-this-dir only (● cwd link / ● parent link / empty), and the global default renders a trailing `⌁ default` tag.

3d. `internal/ui/panes.go` — replace `scopeLegend`:

```go
// scopeLegend decodes the row marks, centered under the profiles list: two
// relevance dots plus the default tag.
func scopeLegend(w int) string {
	row := successStyle.Render("●") + mutedStyle.Render(" this dir   ") +
		lipgloss.NewStyle().Foreground(goldDeep).Render("●") + mutedStyle.Render(" parent dir   ") +
		mutedStyle.Render("⌁ default")
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, truncateLine(row, w))
}
```

- [ ] **Step 4: Run tests, fix stragglers**

Run: `go test ./internal/ui/ -v 2>&1 | tail -20`
Any test still asserting `🌐` or `scopeElsewhere` in the tab views fails here — update it to the new marks (the dashboard's own 🌐 in `ambientLine` is untouched by this plan).

- [ ] **Step 5: Full verification and commit**

Run: `go build ./... && gofmt -l . && go vet ./... && go test ./...`

```bash
git add internal/ui/styles.go internal/ui/panes.go internal/ui/frame.go internal/ui/provider_view.go internal/ui/aws_view_test.go
git commit -m "feat(ui): scope slot means relevance only — two dots + a default tag

Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq"
```

---

### Task 8: DETAILS gains a Linked row

**Files:**
- Modify: `internal/ui/panes.go` (profileInfoBlock), `internal/ui/provider_view.go` (call site)
- Test: `internal/ui/aws_view_test.go`

**Interfaces:**
- Consumes: `providerTabView.mappingDirs` (Task 7), `displayDir` (dirpicker.go:160).
- Produces: `profileInfoBlock(pr, st, browser, linked, driftNote string, w int)` — new `linked` param between `browser` and `driftNote`.

- [ ] **Step 1: Write the failing test** (append to `internal/ui/aws_view_test.go`)

```go
func TestDetailsShowsLinkedDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	clearAmbientEnv(t)
	ap := filepath.Join(home, ".aws-profiles")
	os.MkdirAll(ap, 0o755)
	os.WriteFile(filepath.Join(ap, "work.conf"),
		[]byte("AWS_SSO_START_URL=https://acme.awsapps.com/start\n"), 0o644)
	// Two linked dirs, each with a live pointer so ReadMappings keeps them.
	d1, d2 := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(d1, ".awsprofile"), []byte("work\n"), 0o644)
	os.WriteFile(filepath.Join(d2, ".awsprofile"), []byte("work\n"), 0o644)
	os.WriteFile(filepath.Join(ap, "mappings"),
		[]byte(d1+"\twork\tpointer\n"+d2+"\twork\tpointer\n"), 0o644)

	v := newAwsView()
	nm, _ := v.Update(tea.WindowSizeMsg{Width: 160, Height: 34})
	out := nm.(awsView).View()
	if !strings.Contains(out, "Linked") || !strings.Contains(out, "+ 1 more") {
		t.Fatalf("DETAILS should list the linked dirs:\n%s", out)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/ -run TestDetailsShowsLinkedDirs -v`
Expected: FAIL

- [ ] **Step 3: Implement**

3a. `internal/ui/panes.go` `profileInfoBlock` — new signature and row (after Browser):

```go
// profileInfoBlock renders the top of the DETAILS pane for one profile: a
// key/value sheet with a fixed key column — the conf detail plus the
// disk-only status (identity, linked dirs, expiry, last-used).
func profileInfoBlock(pr profile.Listed, st provider.Status, browser, linked, driftNote string, w int) string {
```

and in `rows`, insert after the Browser row:

```go
		row("Linked", linked),
```

3b. `internal/ui/provider_view.go` `View` — build `linked` beside the existing browser lookup and pass it:

```go
		linked := ""
		if dirs := v.mappingDirs[pr.Name]; len(dirs) > 0 {
			linked = displayDir(dirs[0])
			if len(dirs) > 1 {
				linked += fmt.Sprintf(" + %d more", len(dirs)-1)
			}
		}
		info = profileInfoBlock(pr, st, browser, linked, note, rightW)
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/ -v 2>&1 | tail -10`
Expected: PASS

- [ ] **Step 5: Full verification and commit**

Run: `go build ./... && gofmt -l . && go vet ./... && go test ./...`

```bash
git add internal/ui/panes.go internal/ui/provider_view.go internal/ui/aws_view_test.go
git commit -m "feat(ui): DETAILS shows where a profile is linked

Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq"
```

---

### Task 9: `?` help overlay on every screen

**Files:**
- Modify: `internal/ui/tabs.go`
- Test: `internal/ui/tabs_test.go`

**Interfaces:**
- Consumes: `overlayCenter` (panes.go), `keyHelp`/`keycap`/`paneTitleStyle` (styles.go), `activeCapturesInput` (tabs.go).
- Produces: `tabsModel.help bool`, `helpOverlay() string`.

- [ ] **Step 1: Write the failing test** (append to `internal/ui/tabs_test.go`)

```go
func TestHelpOverlayTogglesFromAnyTab(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	tm := nm.(tabsModel)
	if !tm.help {
		t.Fatal("'?' did not open the help overlay")
	}
	if v := tm.View(); !strings.Contains(v, "KEYS") || !strings.Contains(v, "link here") {
		t.Fatalf("help overlay content missing:\n%s", v)
	}
	// Any key closes it without leaking to the tab.
	nm2, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if nm2.(tabsModel).help {
		t.Fatal("a keypress should close the help overlay")
	}
	// While a text input is armed, '?' is text, not the overlay.
	nm3, _ := nm2.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyTab}) // Azure tab
	nm3, _ = nm3.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	nm4, _ := nm3.(tabsModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if nm4.(tabsModel).help {
		t.Fatal("'?' must reach the armed text input, not open the overlay")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/ui/ -run TestHelpOverlay -v`
Expected: FAIL — `tm.help undefined`

- [ ] **Step 3: Implement in `internal/ui/tabs.go`**

3a. Add `help bool` to `tabsModel`.

3b. Key routing — inside `case tea.KeyMsg:`, FIRST (before the barFocus block):

```go
		// The help overlay swallows its closing keypress.
		if m.help {
			m.help = false
			return m, nil
		}
```

In the barFocus switch, add:

```go
			case "?":
				m.help = true
				return m, nil
```

In the main (tab-focused) switch, add alongside the `d`/`o` cases:

```go
			case "?":
				if !m.activeCapturesInput() {
					m.help = true
					return m, nil
				}
```

3c. Render — in `View`, after the options overlay block:

```go
	if m.help {
		body = overlayCenter(body, helpOverlay(), m.width)
	}
```

3d. The overlay content, at the bottom of tabs.go:

```go
// helpOverlay is the full keymap reference, floated over any tab by '?'.
func helpOverlay() string {
	lines := []string{
		paneTitleStyle.Render("KEYS"),
		"",
		keyHelp("↑↓", "select", "↵", "open/run", "esc", "back"),
		keyHelp("⇥ ]", "next tab", "⇧⇥ [", "prev tab"),
		keyHelp("s", "sign in", "u", "link here", "n", "new profile"),
		keyHelp("a", "capture (empty state)", "b", "browser profile", "delete", "remove"),
		keyHelp("e", "write .envrc (azure)", "d", "change dir", "o", "options"),
		keyHelp("r", "refresh", "?", "close help", "q", "quit"),
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(azureBlue).
		Padding(1, 3).
		Render(strings.Join(lines, "\n"))
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/ui/ -v 2>&1 | tail -10`
Expected: PASS

- [ ] **Step 5: Full verification and commit**

Run: `go build ./... && gofmt -l . && go vet ./... && go test ./...`

```bash
git add internal/ui/tabs.go internal/ui/tabs_test.go
git commit -m "feat(ui): '?' floats a full keymap overlay on every screen

Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq"
```

---

### Task 10: language sweep, docs, final verification

**Files:**
- Modify: `internal/ui/provider_view.go` (straggler strings/comments), `CLAUDE.md`, `README.md`

**Interfaces:** none — cleanup and docs.

- [ ] **Step 1: Sweep the remaining "pin" language**

Run: `grep -rni "pin" internal/ui/*.go | grep -v _test.go`

Fix every hit with link language. Known hits and their exact replacements in `internal/ui/provider_view.go`:
- `useAction`'s success status: `v.status = successStyle.Render(fmt.Sprintf("linked this dir → %s", name))`
- `useAction`'s doc comment: `// useAction links the current directory to the selected profile. Shared by all providers.`
- `reload`'s doc comment: replace `(with its pin's scope)` with `(with its link's scope)`.
- `reload`'s inline comment about repo-local git config: replace `Resolved without a pointer` sentence's `pin` words if present (keep "pointer" — that's the file, not the verb).
- `loginAction` doc comment: `// loginAction hands off to the provider's interactive login flow for the selected profile (browser bridge included) — the recovery verb.`

Comments naming the pointer FILE (`.azprofile`, "pointer") stay; only the pin *verb/noun* changes. Re-run the grep — remaining hits must all be pointer-file references or zero.

- [ ] **Step 2: Update CLAUDE.md**

In the `internal/ui/` architecture bullet, replace the sentence beginning "One selection language:" through the end of the sentence "…falling back to manual entry on discovery failure)." with:

```
One selection language: bright `selBlockActive` in the focused level, dim `selBlockParent` on ancestors only (`selMode`), tri-level focus tab bar → PROFILES → DETAILS/ACTIONS. Every tab is one implementation — the shared `providerTabView` (`provider_view.go`); Azure is a thin `azureView` wrapper (`azure_view.go`) adding the live drift notice and the `e` write-.envrc hotkey, and `frame.go` holds the shared pane renderers. Every tab shares the header anatomy (`headerStrip`: provider icon `providerIcon` · 📁 cwd · 👤 effective identity via `effectiveIdentity`, plus an optional notice line) and pane frame (`renderPaneFrame`, legend `scopeLegend` bottom-anchored via the left-footer slot). Profile rows (`renderProfilePane`) lead with a relevance mark meaning exactly one thing — ● green linked in cwd / ● orange linked via a parent / empty otherwise — with the ambient default carrying a trailing `⌁ default` tag; the most-active profile is bold, renamed labels italic. The DETAILS pane shows a key/value sheet (`profileInfoBlock`: Name/Identity/Detail/Browser/Linked/Expiry/Last used/Drift) over an `ACTIONS (n)` radio with a **never-hide action model** (`enabledActions`): every verb always listed, inapplicable ones disabled with the reason as their hint. One keymap on every tab — `s` Sign in (visible even with a live session), `u` Link here, `n` New profile, `b` Browser profile (async discovery + fuzzy overlay picker `browserpicker.go`, manual-entry fallback), `delete` Remove (confirm dialog everywhere), `a` Capture (empty-state onboarding only, name-prompted), `r`/`f5` refresh, `?` full-keymap overlay (container-level), `e` write .envrc (Azure). Edit/Rename retired from the TUI (edit the .conf directly).
```

- [ ] **Step 3: Update README.md**

Find the TUI keys/actions description: `grep -n "Use here\|Sign in\|pin only" README.md`. Update every listed TUI verb/key to the unified set (`s` sign in · `u` link here · `n` new profile · `b` browser profile · `DEL` remove with confirm · `a` capture in the empty state · `r` refresh · `?` help), replace "pin" language with "link" in those TUI sections, and add one sentence after the action list:

```
Every verb is always visible: an action that doesn't apply (e.g. *Link here* on the already-linked profile) renders dimmed with the reason, instead of disappearing.
```

Leave non-TUI README sections (CLI flags, .envrc, config keys) untouched.

- [ ] **Step 4: Final verification**

```bash
go build ./... && gofmt -l . && go vet ./... && go test ./...
grep -rni "pin" internal/ui/*.go | grep -v _test.go
```

Read the second command's output by eye: every remaining hit must refer to a pointer FILE (`.azprofile`/`.awsprofile`/…, or the word "pointer"); any hit using pin as the verb/noun for the directory association is a miss — fix it. Real-machine TUI checks (visual layout, overlay rendering) go to the manual-verify list, not this task.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/provider_view.go CLAUDE.md README.md
git commit -m "docs: unified TUI action model — link language, keymap, never-hide

Claude-Session: https://claude.ai/code/session_01VTFFiCXfiCdvRVsaiWiKAq"
```
