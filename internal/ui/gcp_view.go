package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/gcp"
)

// gcpView is the GCP provider tab. GCP has no active-profile file (only a cwd pin
// plus the ambient CLOUDSDK_ACTIVE_CONFIG_NAME), so there is no Switch action.
type gcpView struct{ providerTabView }

func newGcpView() gcpView {
	header := paneTitleStyle.Render("Google Cloud") + mutedStyle.Render(" — gcloud configurations · OAuth")
	actions := []providerAction{
		{key: "s", label: "Sign in", hint: "session only — no pin", run: loginAction("gcp")},
		{key: "u", label: "Use here", hint: "pin only — no login", run: useAction},
		{key: "a", label: "New profile", hint: "sign in + pin here", run: newProfileAction},
		{key: "delete", label: "Remove", hint: "delete profile", run: removeAction},
	}
	return gcpView{newProviderTabView(gcp.NewProvider(), header, actions)}
}

func (v gcpView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	return v, cmd
}
