package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/github"
)

// githubView is the GitHub provider tab — the shared view with the gh CLI group.
type githubView struct{ providerTabView }

func newGithubView() githubView {
	return githubView{newProviderTabView(github.NewProvider(), providerActions("gh"))}
}

func (v githubView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	return v, cmd
}
