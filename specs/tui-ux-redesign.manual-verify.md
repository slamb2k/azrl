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
