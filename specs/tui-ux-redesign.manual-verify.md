# TUI UX redesign — manual verification (real laptop + VM)

Items only a real environment can exercise; extend this file as later
redesign plans (console, mouse) ship.

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
- [ ] Dashboard s/t/c/u/b on a real profile row round-trip correctly (t suspends, c opens browser).
