package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/gcp"
)

// gcpView is the GCP provider tab — the shared view with the gcp CLI group.
type gcpView struct{ providerTabView }

func newGcpView() gcpView {
	return gcpView{newProviderTabView(gcp.NewProvider(), providerActions("gcp"))}
}

func (v gcpView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	return v, cmd
}
