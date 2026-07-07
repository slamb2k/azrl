# TUI Mouse + Dashboard Verbs-on-Rows Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Plans 4+5 of the TUI redesign spec: full mouse support (click selects, click-again runs, wheel scrolls, click-outside dismisses overlays, disabled clicks explain themselves) via bubblezone hit-testing, and dashboard rows that accept the whole verb keymap (`s`/`t`/`c`/`u`/`b`, `a` where adoptable) acting on that row's profile, with the cursor starting on the row governing the cwd.

**Architecture:** `zone.NewGlobal()` + `tea.WithMouseCellMotion()` in `run.go` (the setup wizard's separate program stays mouse-free). `zone.Scan` wraps ONLY the root `tabsModel.View` output; children `zone.Mark` their clickable cells (`tab:<i>`, `prof:<name>`, `act:<key>`, `dash:<i>`, overlay rows). The container translates raw `tea.MouseMsg` into **semantic messages** (`tabClickMsg`, forwarded mouse to the active tab) so almost all tests drive semantics directly — only a couple of integration tests touch real zones (bubblezone's `Scan` records bounds asynchronously; those tests sleep ~100ms, per the library's own test suite). Dashboard verbs reuse the exact argv builders the provider tabs use (`login`/`shell`/`console` handoffs), call `provider.Use` in-process for `u`, and route `b` to the owning tab via an extended `switchTabMsg` (the browser picker's sub-state lives there).

**Tech Stack:** Go, Bubble Tea v1.3.10, **bubblezone v1.0.0 exactly** (`github.com/lrstanley/bubblezone` — do NOT `@latest`: v2.0.0 requires the charm.land bubbletea-v2 line and is incompatible), lipgloss v1.1.0.

## Global Constraints

- Build/test/format gates before every commit: `go build ./...`, `go test ./...`, `gofmt -l .` (empty output = clean).
- Conventional commits with scope.
- **Pin the dependency**: `go get github.com/lrstanley/bubblezone@v1.0.0` (import as `zone "github.com/lrstanley/bubblezone"`). Its go.mod wants bubbletea ≥v1.3.4 + lipgloss v1.1.0 — azrl satisfies both.
- `zone.Scan` is called in exactly ONE place: the value returned by `tabsModel.View` (root model). Never in child views.
- Mouse semantics (spec): click **selects**; click-again (a click landing on the already-selected item) **runs**; wheel scrolls the active list; click outside an open overlay dismisses it; clicking a disabled action shows its reason in the status line. Double-click detection is NOT implemented (no timestamps) — click-again covers the spec's "click-again or double-click".
- While an overlay (help, options popup, dirpicker, browserpicker, confirm, naming) is open, underlying zones are inert — the container routes mouse only to the overlay. (Also load-bearing: `overlayCenter` slices background lines with `ansi.Truncate`, which can cut zone markers under the box — inertness makes that harmless.)
- Wheel = move the active list's cursor (dashboard cursor / provider profiles cursor). There is no viewport/offset scrolling in this codebase; wheel-moves-cursor is the whole feature.
- Dashboard verbs: `s` sign in, `t` shell, `c` console (the Plan-4 carry item), `u` link here, `b` browser profile, `a` adopt (existing) — acting on the row's provider+profile. `enter` still drills into the tab. No `delete` on dashboard rows (spec lists only t/c/s/u/b/a).
- Dashboard cursor starts on the first MAPPINGS row with `Scope == ScopeCwd` when one exists (spec: "Cursor starts on the row governing the cwd"), else row 0.
- Help overlay documents that terminal text-copy needs Shift while azrl is open.
- UI language "link", never "pin".
- The setup wizard (`internal/ui/setup.go`, own `tea.NewProgram`) is untouched.

## Recorded assumptions (surface to the user at the end)

1. **Click-again replaces double-click** — the spec offers either; timestamps for true double-click are extra state for zero added capability.
2. **Wheel moves the cursor** of the active tab's primary list ("the list under the pointer" simplification: one primary list per tab; the actions radio stays keyboard/click-driven).
3. **Dashboard `u` (Link here)** calls `provider.Use(profile, confdir, cwd)` in-process (the tab's own semantics) and reloads; rows with no managed profile show a status reason instead.
4. **Dashboard `b`** routes to the owning provider tab via `switchTabMsg` growing an `action string` field — the browser picker's async sub-state lives on the tab; duplicating it on the dashboard would be the two-implementations bug this redesign killed.
5. **Zone integration tests are few and sleep-synchronized** (~100ms, mirroring bubblezone's own tests — `Scan` records bounds via an async worker with no flush API); everything else tests the semantic-message layer without zones.
6. **Pathologically narrow terminals** may truncate a row's closing zone marker (the container truncates lines before `Scan`); the zone then simply doesn't register and the click is a no-op — degraded, never wrong.

---

### Task 1: bubblezone dependency + root wiring + clickable tab cells

**Files:**
- Modify: `go.mod`/`go.sum` (via `go get`), `internal/ui/run.go:16-26`, `internal/ui/tabs.go` (View tab-cell marks + Scan; Update `tea.MouseMsg` case)
- Test: `internal/ui/tabs_test.go` (append)

**Interfaces:**
- Produces: zone IDs `tab:<index>`; the container's `tea.MouseMsg` handling shape (left-release → hit-test; wheel + other → forward to active tab) that Tasks 4-6 extend; helper `leftRelease(msg tea.MouseMsg) bool`.
- Consumes: `zone.NewGlobal()`, `zone.Mark`, `zone.Scan`, `zone.Get(id).InBounds(msg)`.

- [ ] **Step 1: Add the dependency (pinned)**

```bash
go get github.com/lrstanley/bubblezone@v1.0.0
go mod tidy
```

Verify `go.mod` shows `github.com/lrstanley/bubblezone v1.0.0` (NOT v2).

- [ ] **Step 2: Write the failing test**

Append to `internal/ui/tabs_test.go`:

```go
func TestMouseClickOnTabCellSwitchesTab(t *testing.T) {
	m := seedTabs(t)
	// Render through the root View so zone.Scan records tab-cell bounds.
	_ = m.View()
	time.Sleep(120 * time.Millisecond) // bubblezone records zones asynchronously
	z := zone.Get("tab:2")
	if z == nil || z.IsZero() {
		t.Fatal("tab cell 2 has no zone — Mark/Scan not wired")
	}
	nm, _ := m.Update(tea.MouseMsg{
		X: z.StartX, Y: z.StartY,
		Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	if got := nm.(tabsModel).active; got != 2 {
		t.Fatalf("click on tab cell 2 should activate it, active = %d", got)
	}
}
```

Add imports: `"time"`, `zone "github.com/lrstanley/bubblezone"`. If the ui package has a TestMain, ensure `zone.NewGlobal()` runs before zone tests (calling it at the top of this test is fine too — it's idempotent).

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestMouseClickOnTabCell -v`
Expected: FAIL — no zone recorded / MouseMsg ignored.

- [ ] **Step 4: Implement**

**(a)** `internal/ui/run.go` — in `runTabs`:

```go
func runTabs(m tabsModel) error {
	zone.NewGlobal()
	defer zone.Close()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	...
}
```

Also call `zone.NewGlobal()` defensively in `NewTabs()` (idempotent) so View/Update never hit a nil manager in tests that skip runTabs.

**(b)** `internal/ui/tabs.go` View — wrap each tab cell and the final output:

```go
		cells = append(cells, zone.Mark(fmt.Sprintf("tab:%d", i), styled))
```

(where `styled` is the existing per-case rendered label), and at the very end of `View`, wrap the fully assembled, already-truncated output:

```go
	return zone.Scan(out)
```

(keep the existing truncateLine loop before Scan — Scan strips markers so downstream width math is unaffected).

**(c)** `internal/ui/tabs.go` Update — add a `tea.MouseMsg` case near the top of the message switch:

```go
	case tea.MouseMsg:
		return m.handleMouse(msg)
```

and:

```go
// leftRelease reports a completed left click (bubblezone's canonical event).
func leftRelease(msg tea.MouseMsg) bool {
	return msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft
}

// handleMouse routes mouse input: overlays swallow everything (later task),
// tab cells switch tabs, everything else is the active tab's business.
func (m tabsModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if leftRelease(msg) {
		for i := range m.tabs {
			if z := zone.Get(fmt.Sprintf("tab:%d", i)); z != nil && z.InBounds(msg) {
				m.active = i
				m.barFocus = false
				return m, nil
			}
		}
	}
	// Forward to the active tab (wheel + row clicks handled there in later tasks).
	return m.forwardMouse(msg)
}

// forwardMouse hands the event to the active tab's model, mirroring how key
// messages already reach it (inspect the existing key-forwarding path and use
// the same update/replace mechanics).
func (m tabsModel) forwardMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	upd, cmd := m.tabs[m.active].model.Update(msg)
	m.tabs[m.active].model = upd // adapt to the container's actual child-storage types
	return m, cmd
}
```

**Implementer note:** the exact child-update mechanics (`m.tabs[i].model` type, whether Update returns `tea.Model` needing assertion) must mirror how the container already forwards `tea.KeyMsg` — read that path first and copy it. Where the existing switch guards keys behind `activeCapturesInput()`/overlay states, the mouse case must come AFTER those guards or replicate them — for THIS task it's enough that overlays aren't yet mouse-aware: when `m.helpOpen`/options/dirpicker are open, `handleMouse` should do nothing (return `m, nil`) — the inertness contract; later tasks make overlays clickable.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/ui/ -count=1`
Expected: PASS (new test + whole package).

- [ ] **Step 6: Full gates + commit**

```bash
go build ./... && go test ./... && gofmt -l .
git add go.mod go.sum internal/ui/run.go internal/ui/tabs.go internal/ui/tabs_test.go
git commit -m "feat(ui): mouse mode + bubblezone root wiring; clickable tab cells"
```

---

### Task 2: Dashboard verbs-on-rows (keys) + status line + governing-row cursor

**Files:**
- Modify: `internal/ui/dashboard.go` (Update verb keys, new `status` field + render slot, `opDoneMsg` surfacing, cursor init, footer help)
- Test: `internal/ui/dashboard_test.go` (append)

**Interfaces:**
- Produces: dashboard handles `s`/`t`/`c`/`u` on the cursored row (`b` in Task 3); `dashboardModel.status string`; governing-row cursor init helper `governingIndex(ov Overview) int`.
- Consumes: `groupArgs`, `cliGroup`, `runHandoff`, `runShellHandoff`, `captureArgs` — all existing in package ui; `m.providers []provider.Provider` (already on the model) for `Use`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/dashboard_test.go`:

```go
func TestDashboardRowVerbsBuildHandoffs(t *testing.T) {
	m := dashboardModel{width: 100, items: []dashItem{{provider: "aws", profile: "prod"}}}
	for _, key := range []string{"s", "t", "c"} {
		mod, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = mod.(dashboardModel)
		if cmd == nil {
			t.Fatalf("%q on a managed row should hand off", key)
		}
	}
}

func TestDashboardRowVerbsExplainOnUnmanagedRow(t *testing.T) {
	// An ambient row with no managed profile can't sign in/shell/console/link.
	m := dashboardModel{width: 100, items: []dashItem{{provider: "aws", adopt: true}}}
	for _, key := range []string{"s", "t", "c", "u"} {
		mod, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = mod.(dashboardModel)
		if cmd != nil {
			t.Fatalf("%q on an unmanaged row must not exec", key)
		}
	}
	if m.status == "" || !strings.Contains(m.status, "adopt") {
		t.Fatalf("unmanaged-row verb should explain itself in the status line: %q", m.status)
	}
}

func TestDashboardLinkHereUsesProviderInProcess(t *testing.T) {
	seedDashHome(t)
	work := filepath.Join(os.Getenv("HOME"), "work")
	os.MkdirAll(work, 0o755)
	t.Chdir(work)
	m := newDashboard(provider.All())
	// Find the azure acme item seedDashHome creates.
	idx := -1
	for i, it := range m.items {
		if it.provider == "azure" && it.profile == "acme" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("no azure:acme item: %+v", m.items)
	}
	m.cursor = idx
	mod, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	m = mod.(dashboardModel)
	if b, err := os.ReadFile(filepath.Join(work, ".azprofile")); err != nil || strings.TrimSpace(string(b)) != "acme" {
		t.Fatalf("u should link the cwd to acme: %q, %v", b, err)
	}
	if !strings.Contains(m.status, "linked") {
		t.Fatalf("status should confirm the link: %q", m.status)
	}
}

func TestDashboardCursorStartsOnGoverningRow(t *testing.T) {
	ov := Overview{Mappings: []MappingRow{
		{Provider: "github", Dir: "/w/other", Profile: "oss", Scope: ScopeNone},
		{Provider: "azure", Dir: "/w/here", Profile: "acme", Scope: ScopeCwd},
	}}
	if got := governingIndex(ov); got != 1 {
		t.Fatalf("governingIndex = %d, want 1", got)
	}
	if got := governingIndex(Overview{}); got != 0 {
		t.Fatalf("empty overview should default to 0, got %d", got)
	}
}

func TestDashboardSurfacesOpDone(t *testing.T) {
	m := dashboardModel{width: 100}
	mod, _ := m.Update(opDoneMsg{msg: "shell exited"})
	if got := mod.(dashboardModel).status; !strings.Contains(got, "shell exited") {
		t.Fatalf("opDoneMsg should surface in the dashboard status: %q", got)
	}
}
```

(`TestDashboardLinkHereUsesProviderInProcess` needs the same PATH-isolation the other seedDashHome tests get for free; if azure `Use` shells out to anything, check the sibling `use`-path tests for required shims — `Scheme.Touch`/pointer writes are pure disk, so none should be needed.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestDashboardRow|TestDashboardLinkHere|TestDashboardCursor|TestDashboardSurfaces' -v`
Expected: FAIL — verbs unhandled, `status`/`governingIndex` undefined.

- [ ] **Step 3: Implement**

In `internal/ui/dashboard.go`:

**(a)** Add `status string` to `dashboardModel`.

**(b)** `opDoneMsg` case — surface it:

```go
	case opDoneMsg:
		m.reload()
		if msg.err != nil {
			m.status = failureStyle.Render("✗ " + msg.err.Error())
		} else if msg.msg != "" {
			m.status = successStyle.Render("✓ " + msg.msg)
		}
		return m, nil
```

**(c)** Verb keys in the KeyMsg switch (after the existing `"a"` case; clear `m.status` at the top of the key switch so stale messages don't linger):

```go
		case "s", "t", "c", "u":
			if m.cursor < 0 || m.cursor >= len(m.items) {
				return m, nil
			}
			it := m.items[m.cursor]
			if it.profile == "" {
				m.status = mutedStyle.Render("no managed profile on this row — a adopts it first")
				return m, nil
			}
			switch msg.String() {
			case "s":
				return m, runHandoff(append(groupArgs(cliGroup(it.provider), "login"), it.profile))
			case "t":
				return m, runShellHandoff(append(groupArgs(cliGroup(it.provider), "shell"), it.profile))
			case "c":
				return m, runHandoff(append(groupArgs(cliGroup(it.provider), "console"), it.profile))
			case "u":
				pwd, _ := os.Getwd()
				for _, p := range m.providers {
					if p.Name() != it.provider {
						continue
					}
					if err := p.Use(it.profile, p.ProfilesDir(), pwd); err != nil {
						m.status = failureStyle.Render("✗ " + err.Error())
					} else {
						m.status = successStyle.Render("✓ linked " + displayDir(pwd) + " → " + it.profile)
						m.reload()
					}
					return m, nil
				}
				return m, nil
			}
```

(Verify `Use`'s exact signature on the `provider.Provider` interface — `Use(name, confdir, pwd string) error` per the interface; adapt argument order to reality.)

**(d)** Governing-row cursor:

```go
// governingIndex returns the flat item index of the first mapping row that
// governs the cwd — where the cursor should wake up (spec: the dashboard is
// the command center; you land on the row that answers "what am I here?").
func governingIndex(ov Overview) int {
	for i, r := range ov.Mappings {
		if r.Scope == ScopeCwd {
			return i // mappings occupy the head of the flat item list
		}
	}
	return 0
}
```

In `newDashboard` (after the initial `reload()`): `m.cursor = governingIndex(m.ov)`. Do NOT re-apply it inside `reload()` — the user's cursor position survives refreshes; only initial placement is governed.

**(e)** Render `m.status`: in `View`, when `m.status != ""`, show it where the hint/notice line goes (mirror how the naming prompt already takes over that slot; status takes precedence over the "all good" hint but not over conflict/drift warnings — simplest: if `m.status != ""` replace the `short` chip with it for that frame).

**(f)** Footer help: extend the existing `keyHelpFit` call's primary list with `"s/t/c/u", "act on row"` (keep it in the fit-aware list so narrow widths drop it gracefully), and add the same verbs note to the container help overlay (tabs.go) if its dashboard section exists — check `helpOverlay` for a dashboard-specific line and extend it; report what you found.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -count=1`
Expected: PASS.

- [ ] **Step 5: Full gates + commit**

```bash
go build ./... && go test ./... && gofmt -l .
git add internal/ui/dashboard.go internal/ui/dashboard_test.go internal/ui/tabs.go
git commit -m "feat(ui): dashboard rows take the verb keymap; status line; governing-row cursor"
```

---

### Task 3: Dashboard `b` routes to the owning tab

**Files:**
- Modify: `internal/ui/tabs.go` (`switchTabMsg` gains `action string`; handler triggers the tab's action after switching), `internal/ui/dashboard.go` (`b` key)
- Test: `internal/ui/dashboard_test.go`, `internal/ui/tabs_test.go` (append)

**Interfaces:**
- Produces: `switchTabMsg{provider, profile, action}` — `action` is an accelerator key the container dispatches on the target tab after selecting the profile ("" = none, current behavior).
- Consumes: the container's existing `switchTabMsg` handling (find it in tabs.go — it switches to the provider's tab and selects the profile) and `providerTabView.dispatch(key)`.

- [ ] **Step 1: Write the failing tests**

```go
// dashboard_test.go
func TestDashboardBRoutesToTab(t *testing.T) {
	m := dashboardModel{width: 100, items: []dashItem{{provider: "github", profile: "oss"}}}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if cmd == nil {
		t.Fatal("b should emit a switchTabMsg")
	}
	msg := cmd()
	sw, ok := msg.(switchTabMsg)
	if !ok || sw.provider != "github" || sw.profile != "oss" || sw.action != "b" {
		t.Fatalf("switchTabMsg = %+v", msg)
	}
}

// tabs_test.go
func TestSwitchTabMsgWithActionTriggersIt(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(switchTabMsg{provider: "github", profile: "work", action: "b"})
	tm := nm.(tabsModel)
	// The github tab is active and its browser flow started (discovery pending
	// or picker/manual state visible). Assert on the View: the browser flow's
	// UI copy appears (inspect browserpicker.go for its prompt/status text and
	// assert a stable substring), OR at minimum the tab switched and the
	// profile is selected.
	if tm.tabs[tm.active].title != "GitHub" {
		t.Fatalf("should land on the GitHub tab, got %q", tm.tabs[tm.active].title)
	}
	_ = tm // extend with the browser-flow assertion per the implementer note
}
```

**Implementer note:** read the existing `switchTabMsg` handler in tabs.go first — it already switches tab + selects the profile. Your change: after that, if `action != ""`, dispatch it on the target tab (the same way the tab's own accelerator loop would — for a providerTabView, call its dispatch path; check the concrete type juggling the container uses). The browser flow starts an async `discoverBrowsersCmd`; the returned tea.Cmd must be executed/returned by the container so discovery actually runs — assert the cmd is non-nil and/or the view shows the browser flow's status copy (read browserpicker.go for the exact string, e.g. a "discovering" status). Strengthen the second test to pin real observable behavior once you see the actual strings; do not leave it at tab-switch-only.

- [ ] **Step 2: RED**

Run: `go test ./internal/ui/ -run 'TestDashboardBRoutes|TestSwitchTabMsgWithAction' -v`
Expected: FAIL — no `action` field.

- [ ] **Step 3: Implement**

- `switchTabMsg` struct gains `action string`.
- Dashboard `b` case (managed rows only; unmanaged → same status reason as Task 2):

```go
		case "b":
			if m.cursor >= 0 && m.cursor < len(m.items) {
				it := m.items[m.cursor]
				if it.profile == "" {
					m.status = mutedStyle.Render("no managed profile on this row — a adopts it first")
					return m, nil
				}
				return m, func() tea.Msg {
					return switchTabMsg{provider: it.provider, profile: it.profile, action: "b"}
				}
			}
			return m, nil
```

- Container handler: after the existing switch+select, `if msg.action != "" { dispatch it on the target tab and return the resulting cmd }`.

- [ ] **Step 4: GREEN** — `go test ./internal/ui/ -count=1` PASS.

- [ ] **Step 5: Full gates + commit**

```bash
git add internal/ui/tabs.go internal/ui/dashboard.go internal/ui/dashboard_test.go internal/ui/tabs_test.go
git commit -m "feat(ui): dashboard b routes to the owning tab's browser flow"
```

---

### Task 4: Provider-tab mouse — row/action clicks + wheel

**Files:**
- Modify: `internal/ui/frame.go` (`renderProfilePane` marks rows), `internal/ui/provider_view.go` (action rows marked where the radio renders — likely radio.go or the actions pane assembly; find it; plus a `handleMouse` on the view), `internal/ui/radio.go` if that's where option rows render
- Test: `internal/ui/provider_view_mouse_test.go` (create)

**Interfaces:**
- Produces: zone IDs `prof:<profileName>` and `act:<actionKey>` (namespaced per tab is unnecessary — only the active tab's zones are in the last Scan); providerTabView mouse handling: click profile row → select (`cursor`+focus), click-again → enter semantics; click action row → select, click-again → run enabled / status-reason disabled; wheel up/down → cursor up/down in the profiles list.
- Consumes: `leftRelease` (Task 1), the container's `forwardMouse` (Task 1) which delivers `tea.MouseMsg` to the active tab.

- [ ] **Step 1: Write the failing tests**

Create `internal/ui/provider_view_mouse_test.go` — drive the SEMANTIC layer where possible. Structure (adapt scaffolding to the aws-view test idioms as previous plans did; behaviors are the requirements):

```go
func TestWheelMovesProfileCursor(t *testing.T) {
	v := /* aws view with ≥2 profiles, cursor 0 (existing constructor) */
	nv, _ := v.update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if nv.cursor != 1 {
		t.Fatalf("wheel down should move the cursor, got %d", nv.cursor)
	}
	nv, _ = nv.update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if nv.cursor != 0 {
		t.Fatalf("wheel up should move back, got %d", nv.cursor)
	}
}

func TestClickProfileRowSelectsThenEnters(t *testing.T) {
	// Integration test through real zones: render the container, sleep, click.
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")}) // to a provider tab
	tm := nm.(tabsModel)
	_ = tm.View()
	time.Sleep(120 * time.Millisecond)
	z := zone.Get("prof:acme") // seedTabs seeds azure acme — adapt to whichever tab/profile is active
	if z == nil || z.IsZero() {
		t.Skipf("profile zone not recorded at this size — adapt the id/profile to the active tab")
	}
	click := tea.MouseMsg{X: z.StartX, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ = tm.Update(click)
	// First click selects; second click (same row) behaves like enter.
	nm, _ = nm.(tabsModel).Update(click)
	_ = nm // assert focus/selection observable in View() — pin real strings per scaffolding
}

func TestClickDisabledActionExplains(t *testing.T) {
	v := /* view where an action is disabled, e.g. "u" already-linked state per enabledActions tests */
	nv := /* deliver a semantic action-click for the disabled key — via the view's mouse handler with a synthetic zone hit, or expose clickAction(key) and call it */
	if !strings.Contains(nv.status, "already linked") {
		t.Fatalf("disabled action click must surface its reason: %q", nv.status)
	}
}
```

**Implementer note:** implement a small semantic entry point on the view — `func (v providerTabView) clickAction(key string) (providerTabView, tea.Cmd)` and `clickProfile(name string)` — the MouseMsg handler resolves zones and calls these; tests call them directly (no zones, no sleep) except the one integration test above. `clickAction` mirrors the existing accelerator loop exactly (select + run enabled / status reason disabled). `clickProfile`: if the name is already the selection, apply the same behavior as `enter` on the profiles pane (focus to actions); else select it. If a test can't reach a private method through the established test idioms, the tests live in package ui so private access is fine.

- [ ] **Step 2: RED** — run the new tests; expected FAIL (no mouse handling on the view).

- [ ] **Step 3: Implement**

- `renderProfilePane` (frame.go): wrap each row's final line with `zone.Mark("prof:"+p.Name, line)` (the whole row line, before it's joined; the pane is inside the root Scan so no extra Scan).
- The actions radio rows: find where each action row line renders (radio.go or provider_view) and wrap with `zone.Mark("act:"+key, line)` — the key must be the `providerAction.key`.
- `providerTabView` update: add a `tea.MouseMsg` case:
  - wheel up/down → move `cursor` within bounds (same mutation as `up`/`down` keys; keep focus behavior sane — wheel shouldn't hand focus to the tab bar at the top; clamp instead).
  - left release → iterate the view's own zone IDs: profiles (`prof:<name>` for each listed profile), actions (`act:<key>` for each action). On hit, call `clickProfile`/`clickAction`.
  - While any sub-state is active (`capturesInput()` true, confirm, pickers), ignore row/action clicks (return unchanged) — overlays own input (Task 6 handles their own zones).

- [ ] **Step 4: GREEN** — `go test ./internal/ui/ -count=1` PASS.

- [ ] **Step 5: Full gates + commit**

```bash
git add internal/ui/
git commit -m "feat(ui): provider-tab mouse — row and action clicks, wheel cursor"
```

---

### Task 5: Dashboard mouse — row clicks + wheel

**Files:**
- Modify: `internal/ui/dashboard.go` (row marks in View; `tea.MouseMsg` case in Update)
- Test: `internal/ui/dashboard_test.go` (append)

**Interfaces:**
- Produces: zone IDs `dash:<index>` over each selectable row line; click selects, click-again = `enter` (drill into tab); wheel moves cursor.
- Consumes: `leftRelease`, container forwarding.

- [ ] **Step 1: Write the failing tests**

```go
func TestDashboardWheelMovesCursor(t *testing.T) {
	m := dashboardModel{width: 100, items: []dashItem{{provider: "a"}, {provider: "b"}}}
	mod, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if mod.(dashboardModel).cursor != 1 {
		t.Fatal("wheel down should move the dashboard cursor")
	}
}

func TestDashboardClickAgainDrills(t *testing.T) {
	// Semantic layer: clickRow(i) — first click selects, second drills.
	m := dashboardModel{width: 100, items: []dashItem{{provider: "azure", profile: "acme"}, {provider: "aws", profile: "prod"}}}
	m2, cmd := m.clickRow(1)
	if m2.cursor != 1 || cmd != nil {
		t.Fatalf("first click selects only: cursor=%d cmd=%v", m2.cursor, cmd)
	}
	m3, cmd := m2.clickRow(1)
	if cmd == nil {
		t.Fatal("click-again should drill into the tab")
	}
	if sw, ok := cmd().(switchTabMsg); !ok || sw.provider != "aws" {
		t.Fatalf("drill should emit switchTabMsg: %+v", cmd())
	}
	_ = m3
}
```

- [ ] **Step 2: RED**; **Step 3: Implement**

- In `View`, wrap each selectable row line with `zone.Mark(fmt.Sprintf("dash:%d", idx), line)` — inside the same `marker()`/idx flow that already tracks the flat index.
- `clickRow(i int) (dashboardModel, tea.Cmd)`: out-of-range → no-op; `i != cursor` → select; `i == cursor` → the exact `enter` behavior (emit `switchTabMsg{provider, profile}`).
- `tea.MouseMsg` case in Update: wheel → cursor ±1 clamped (no `focusTabsMsg` at top — clamp); left release → scan `dash:<i>` zones for a hit → `clickRow(i)`. Ignore mouse while `m.naming`.

- [ ] **Step 4: GREEN**; **Step 5: gates + commit**

```bash
git add internal/ui/dashboard.go internal/ui/dashboard_test.go
git commit -m "feat(ui): dashboard mouse — row clicks and wheel"
```

---

### Task 6: Overlay mouse — clickable rows + click-outside dismiss

**Files:**
- Modify: `internal/ui/tabs.go` (`handleMouse` overlay routing; help overlay dismiss; options popup rows), `internal/ui/options.go` (mark rows + box), `internal/ui/dirpicker.go` (mark rows + box), `internal/ui/browserpicker.go` (mark rows + box)
- Test: `internal/ui/tabs_test.go` / picker tests (append)

**Interfaces:**
- Produces: zone IDs `box:options`, `box:help`, `box:dir`, `box:browser` (the overlay boxes) and `opt:<i>`, `dir:<i>`, `bp:<i>` row IDs; overlay-open mouse routing in `handleMouse`: row click selects, click-again confirms (the overlay's enter), click outside the box dismisses (the overlay's esc); help overlay: any click dismisses.
- Consumes: everything prior.

- [ ] **Step 1: Write the failing tests** (semantic where possible)

```go
func TestClickOutsideOptionsDismisses(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")}) // open options
	tm := nm.(tabsModel)
	_ = tm.View()
	time.Sleep(120 * time.Millisecond)
	z := zone.Get("box:options")
	if z == nil || z.IsZero() {
		t.Fatal("options box zone missing")
	}
	// Click one cell left of the box = outside.
	outside := tea.MouseMsg{X: z.StartX - 1, Y: z.StartY, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
	nm, _ = tm.Update(outside)
	if /* options still open — inspect the model's options-open field name */ {
		t.Fatal("click outside the options popup should dismiss it")
	}
}

func TestHelpOverlayClickDismisses(t *testing.T) {
	m := seedTabs(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	tm := nm.(tabsModel)
	nm, _ = tm.Update(tea.MouseMsg{X: 1, Y: 1, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if /* help still open */ {
		t.Fatal("any click should dismiss the help overlay")
	}
}
```

(Adapt the open/closed field assertions to the container's real field names. For row-click tests on dirpicker/browserpicker, follow their existing tests' construction and use semantic click methods mirroring Tasks 4-5; add one zone-integration test per overlay ONLY if the semantic layer can't observe the behavior.)

- [ ] **Step 2: RED**; **Step 3: Implement**

- Each overlay's box render wraps the whole box with `zone.Mark("box:<name>", box)` — CAREFUL: `overlayCenter` slices the BACKGROUND under the box; the box string itself is placed intact, so its markers survive; background zones under it may be cut, which is fine because overlay-open routing never consults them.
- `handleMouse` (tabs.go) gains overlay-first routing: when help open → any left release closes it; when options/dirpicker open → row hit selects (click-again = enter), outside `box:` → esc path; wheel → move the overlay's own cursor. Browserpicker lives inside the provider view — route mouse to the active tab when its overlay is open (`capturesInput()`-style check) and implement its row/outside handling in provider_view/browserpicker with the same semantics.
- Help overlay content: add the line documenting Shift-for-copy (e.g. `hold Shift to select/copy terminal text while azrl is open`) — this is the spec's help requirement; extend the existing overlay test that asserts keymap entries.

- [ ] **Step 4: GREEN**; **Step 5: gates + commit**

```bash
git add internal/ui/
git commit -m "feat(ui): overlay mouse — clickable rows, click-outside dismiss, Shift-copy note"
```

---

### Task 7: Docs + manual-verify + final verification

**Files:**
- Modify: `README.md` (mouse paragraph in the TUI section), `CLAUDE.md` (ui bullet: mouse + bubblezone + dashboard verbs + governing-row cursor; commands/deps note), `specs/tui-ux-redesign.manual-verify.md` (Plan 5 section)

**Steps:**

- [ ] **Step 1: README** — in the TUI section, add a short paragraph: the TUI is mouse-aware (click selects, click again runs, wheel scrolls, click outside a popup closes it); dashboard rows take `s/t/c/u/b/a` directly; hold Shift to select/copy terminal text while azrl is open.

- [ ] **Step 2: CLAUDE.md** — splice in-voice: `internal/ui/` bullet gains: mouse support via bubblezone v1 (`zone.Scan` at the root `tabsModel.View` only; `tab:/prof:/act:/dash:/box:` zones; click-selects/click-again-runs; wheel moves cursors; overlays swallow mouse and dismiss on outside-click), dashboard rows accept the verb keymap (`s/t/c/u` handoffs + in-process `Use`, `b` routed to the tab via `switchTabMsg.action`, new dashboard status line), and the cursor starts on the cwd-governing row. Note the new dependency in whatever sentence lists key libs (if none exists, the ui bullet mention suffices).

- [ ] **Step 3: manual-verify** — append:

```markdown
## Mouse + dashboard verbs (Plan 5)

- [ ] Click/click-again across tab cells, profile rows, actions, dashboard rows
      behaves in a real terminal (tmux + plain) as in tests.
- [ ] Wheel scrolls the focused list; no runaway scrolling in tmux passthrough.
- [ ] Click outside options/dirpicker/browserpicker dismisses; help closes on any click.
- [ ] Shift+drag selects terminal text while azrl runs (per the help note).
- [ ] Dashboard s/t/c/u/b on a real profile row round-trip correctly (t suspends, c opens browser).
```

- [ ] **Step 4: Whole-branch verification**

```bash
go build ./... && go test ./... && gofmt -l .
git diff main --stat
grep -rn "bubblezone" go.mod   # expect exactly v1.0.0
```

- [ ] **Step 5: Commit**

```bash
git add README.md CLAUDE.md specs/tui-ux-redesign.manual-verify.md
git commit -m "docs: TUI mouse support + dashboard verbs-on-rows"
```

---

## Post-plan checklist (for the executor)

- Surface the six **Recorded assumptions** with the final report.
- Final whole-branch review before shipping (pay attention to: zone marker survival through the truncation pipeline at narrow widths; mouse-while-overlay inertness; the async-Scan sleeps being confined to few tests); ship via /ship.
- This completes the redesign spec's phasing (1-5); note that in the ledger.
