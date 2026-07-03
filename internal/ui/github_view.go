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
		{key: "s", label: "Sign in", hint: "browser-bridge login", run: loginAction("gh", true)},
		{key: "u", label: "Use here", hint: "pin this dir", run: useAction},
		{key: "a", label: "New profile", hint: "create + sign in", run: loginAction("gh", false)},
		{key: "delete", label: "Remove", hint: "delete profile", run: removeAction},
	}
	return githubView{newProviderTabView(github.NewProvider(), header, actions)}
}

func (v githubView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	return v, cmd
}
