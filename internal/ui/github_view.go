package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/github"
)

// githubView is the GitHub provider tab.
type githubView struct{ providerTabView }

func newGithubView() githubView {
	header := paneTitleStyle.Render("GitHub") + mutedStyle.Render(" — github.com · *.ghe.com · GHES")
	actions := []providerAction{
		{"s", "Sign in", loginAction("gh", true)},
		{"u", "Use here", useAction},
		{"a", "New profile", loginAction("gh", false)},
		{"delete", "Remove", removeAction},
	}
	return githubView{newProviderTabView(github.NewProvider(), header, actions)}
}

func (v githubView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	return v, cmd
}
