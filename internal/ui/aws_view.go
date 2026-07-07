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
