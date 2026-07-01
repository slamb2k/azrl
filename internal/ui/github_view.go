package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/github"
)

// githubView is the GitHub provider tab. Unlike AWS/GCP it has an active-profile
// concept, so it offers a Switch action alongside use/remove.
type githubView struct{ providerTabView }

func newGithubView() githubView {
	header := paneTitleStyle.Render("GitHub") + mutedStyle.Render(" — github.com · *.ghe.com · GHES")
	actions := []providerAction{
		{"s", "Sign in", func(v *providerTabView) {
			v.status = accentStyle.Render("Run `ghrl login` in a terminal to sign in (interactive).")
		}},
		{"u", "Use here", useAction},
		{"w", "Switch", switchAction},
		{"a", "New profile", func(v *providerTabView) {
			v.status = accentStyle.Render("Run `ghrl login <name>` to create and sign into a new profile.")
		}},
		{"d", "Remove", removeAction},
	}
	return githubView{newProviderTabView(github.NewProvider(), header, actions)}
}

func (v githubView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	return v, cmd
}

// switchAction flips GitHub's active profile (its cross-provider distinctive).
func switchAction(v *providerTabView) {
	name := v.selected()
	if name == "" {
		v.status = mutedStyle.Render("no profile selected")
		return
	}
	if err := github.Switch(v.prov.ProfilesDir(), name); err != nil {
		v.status = failureStyle.Render(err.Error())
	} else {
		v.status = successStyle.Render(fmt.Sprintf("switched active profile to %q", name))
	}
}
