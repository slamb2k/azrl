package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/aws"
)

// awsView is the AWS provider tab. AWS has no active-profile file (only a cwd pin
// plus the ambient AWS_PROFILE), so there is no Switch action.
type awsView struct{ providerTabView }

func newAwsView() awsView {
	header := paneTitleStyle.Render("AWS") + mutedStyle.Render(" — IAM Identity Center · SSO")
	actions := []providerAction{
		{"s", "Sign in", func(v *providerTabView) {
			v.status = accentStyle.Render("Run `azrl aws login` in a terminal to sign in (interactive).")
		}},
		{"u", "Use here", useAction},
		{"a", "New profile", func(v *providerTabView) {
			v.status = accentStyle.Render("Run `azrl aws login <name>` to create and sign into a new profile.")
		}},
		{"d", "Remove", removeAction},
	}
	return awsView{newProviderTabView(aws.NewProvider(), header, actions)}
}

func (v awsView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	return v, cmd
}
