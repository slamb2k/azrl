package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/config"
	"github.com/slamb2k/azrl/internal/profile"
)

// accountShowFn is overridable in tests; it reports the az identity for a
// specific profile config dir.
var accountShowFn = azure.AccountShowIn

// azureView is the Azure provider tab: the shared providerTabView plus the
// live drift check (az account show against the dir-linked profile's isolated
// config dir vs the ambient session) and the `e` write-.envrc recovery hotkey.
type azureView struct {
	providerTabView
	signedIn     string
	ambientWho   string
	drift        bool
	ambientEmpty bool
}

func newAzureView() azureView {
	return azureView{providerTabView: newProviderTabView(azure.NewProvider(), providerActions(""))}
}

// identityMsg carries the signed-in identity of this dir's profile session
// ("" when that profile has no live session), the ambient session's identity,
// and whether they differ with no .envrc linking them together (drift). Both
// identities are tenant-qualified, so a B2B guest signed into two tenants
// with one UPN still reads as drift.
type identityMsg struct {
	who          string
	ambientWho   string
	drift        bool
	ambientEmpty bool
}

// identityCmd reads the account from the resolved profile's token dir, so the
// strip reflects who you'd be in this dir — not the ambient ~/.azure session.
// When a profile is resolved but the ambient `az` shows a different identity
// and no .envrc links it, it flags drift so the UI can offer to write one.
func identityCmd() tea.Cmd {
	pwd, _ := os.Getwd()
	return func() tea.Msg {
		name, rErr := profile.Resolve("", pwd)
		dir := ""
		if rErr == nil {
			dir = filepath.Join(config.ProfilesDir(), name)
		}
		who := identityOf(accountShowFn(dir))
		msg := identityMsg{who: who}
		envrcDir := pwd
		if d, ok := profile.LocateAzprofile(pwd); ok {
			envrcDir = d
		}
		if rErr == nil && who != "" && !profile.HasEnvrc(envrcDir) {
			msg.ambientWho = identityOf(accountShowFn(""))
			msg.drift = msg.ambientWho != who
			msg.ambientEmpty = msg.ambientWho == ""
		}
		return msg
	}
}

// identityOf extracts the tenant-qualified identity from `az account show`
// output — the same composition the disk-only readers use, so comparisons
// stay tenant-aware (B2B guests share a UPN across tenants).
func identityOf(b []byte, err error) string {
	if err != nil {
		return ""
	}
	var a profile.AccountJSON
	if json.Unmarshal(b, &a) != nil {
		return ""
	}
	return azure.QualifiedIdentity(a.User.Name, a.TenantDefaultDomain, a.TenantID)
}

func (v azureView) Init() tea.Cmd { return identityCmd() }

// syncHeader projects the async drift state into the shared view's header
// fields: the freshest identity for the linked dir, plus the warning notice.
func (v *azureView) syncHeader() {
	v.identityOverride = v.signedIn
	v.notice = ""
	if v.drift {
		what := "is " + v.ambientWho
		if v.ambientEmpty {
			what = "has no active session"
		}
		v.notice = failureStyle.Render("⚠ shell az "+what+" — this dir expects "+v.signedIn) +
			mutedStyle.Render(" · ") + keycap("e") + mutedStyle.Render(" writes .envrc")
	}
}

func (v azureView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case identityMsg:
		v.signedIn = msg.who
		v.ambientWho = msg.ambientWho
		v.drift = msg.drift
		v.ambientEmpty = msg.ambientEmpty
		v.syncHeader()
		return v, nil
	case tea.KeyMsg:
		if msg.String() == "e" && !v.capturesInput() {
			pwd, _ := os.Getwd()
			if _, err := profile.Resolve("", pwd); err != nil {
				v.status = failureStyle.Render("✗ no profile here to link")
				return v, nil
			}
			v.status = ""
			return v, runWriteEnvrc()
		}
	}
	nv, cmd := v.providerTabView.update(msg)
	v.providerTabView = nv
	switch m := msg.(type) {
	case cwdChangedMsg, opDoneMsg:
		// Disk state changed under us; re-check the live identity + drift.
		return v, tea.Batch(cmd, identityCmd())
	case tea.KeyMsg:
		if (m.String() == "r" || m.String() == "f5") && !v.capturesInput() {
			// Explicit refresh should also clear a stale drift notice.
			return v, tea.Batch(cmd, identityCmd())
		}
	}
	return v, cmd
}
