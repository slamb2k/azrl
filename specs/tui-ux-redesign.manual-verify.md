# TUI UX redesign — manual verification (real laptop + VM)

Items only a real environment can exercise; all redesign plans (shell,
console, mouse) have shipped.

## azrl shell (Plan 3)

- [ ] `azrl shell <azure-profile>` from a linked repo: subshell `az account show`
      shows the profile's account; `exit` restores the outer identity.
- [ ] Dead-session path: expire/clear the profile's session, `azrl shell` runs
      the full bridged login (browser pops on the local machine) before the
      subshell starts; a failed/cancelled login starts no shell.
- [ ] `git push` inside a subshell with a mapped browser profile opens the
      mapped browser (AZRL_BROWSER_CMD narrowing of the GCM limitation).
- [ ] Nested `azrl shell` warns and the innermost identity wins.
- [ ] Prompt snippet (bash or starship) shows `[azure:work]` inside the shell.
- [ ] TUI `t` suspends into the subshell and reloads cleanly on exit; the
      header chip `⌁ shell: <name>` shows inside a subshell-launched TUI and
      the Azure drift warning stays quiet.

## azrl console (Plan 4)

- [ ] `azrl console <azure-profile>` on a remote VM opens the tenant-scoped
      portal in the mapped browser profile on the laptop.
- [ ] `azrl gcp console <name>` lands on the right project AND the right
      Google account (authuser honored with multiple signed-in accounts).
- [ ] No-browser config and unreachable-host paths print the URL cleanly.
- [ ] TUI `c` opens the console and returns without disturbing the TUI.

## Mouse + dashboard verbs (Plan 5)

- [ ] Click/click-again across tab cells, profile rows, actions, dashboard rows
      behaves in a real terminal (tmux + plain) as in tests.
- [ ] Wheel scrolls the focused list; no runaway scrolling in tmux passthrough.
- [ ] Click outside options/dirpicker/browserpicker dismisses; help closes on any click.
- [ ] Shift+drag selects terminal text while azrl runs (per the help note).
- [ ] Dashboard s/t/c/u/b/U on a real profile row round-trip correctly (t suspends, c opens browser).

## Entity model — personas on tabs, edges on the dashboard (entity-model-cleanup)

- [ ] **Delete-with-links round-trip:** link a profile to two real
      directories, then `delete` it from its tab. Confirm both dirs are
      listed and the three-option radio appears. Pick `Unmap N dir(s) +
      delete`: both `.azprofile`/`.ghprofile`/etc pointers are gone, the
      profile's conf and token dir are gone, and `azrl status` no longer
      lists either directory. Repeat picking `Replace mappings with…` against a
      second profile instead: both directories now point at the replacement
      (`cat .azprofile` in each) and the original profile is gone.
- [ ] **Unlink refusal on a parent-governed dir:** `cd` into a subdirectory
      of a directory that's linked (no pointer of its own), run `azrl
      unmap` (or `gh`/`aws`/`gcp unmap`). It refuses, naming the parent
      directory and profile exactly ("run unmap there"); the parent's
      pointer is untouched. Now `cd` to the parent itself and `azrl unmap`
      — the link is removed, the profile is untouched.
- [ ] **No-link create then dashboard link:** from an unlinked directory,
      use the TUI's `NEW ＋` title-bar button / `n` key (or `azrl login
      <name> --no-map`) to create and sign in. Confirm no `.azprofile` was written
      here and the profile doesn't show as linked on this tab. Open the
      Dashboard, find the new profile's UNMAPPED row, press `m` to map it
      to the cwd — the row moves to MAPPINGS and `azrl status` now shows the
      directory mapped to it. Press `⇧M` on that same row afterward and
      confirm it unmaps cleanly (profile kept, dashboard status line
      confirms).
