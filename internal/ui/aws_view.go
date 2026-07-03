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
		{key: "s", label: "Sign in", hint: "session only — no pin", run: loginAction("aws")},
		{key: "u", label: "Use here", hint: "pin only — no login", run: useAction},
		{key: "a", label: "New profile", hint: "sign in + pin here", run: newProfileAction},
		{key: "delete", label: "Remove", hint: "delete profile", run: removeAction},
	}
	return awsView{newProviderTabView(aws.NewProvider(), header, actions)}
}

func (v awsView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	return v, cmd
}
