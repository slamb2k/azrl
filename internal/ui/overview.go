package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/profile"
	"github.com/slamb2k/azrl/internal/provider"
)

// Scope values for a mapping row relative to the cwd the TUI/CLI ran from.
const (
	ScopeCwd      = "cwd"      // the mapping's dir is the cwd
	ScopeAncestor = "ancestor" // the mapping's dir is the nearest governing ancestor
	ScopeNone     = "none"     // the mapping does not govern the cwd
)

// MappingRow is one directory→profile association for the landing view and
// `azrl status`. Profile is "" for an unmanaged row (Unmanaged then carries
// the git-config identity, adoptable); Conflict is set when a repo's git
// config and .ghprofile disagree (git wins). Pointer is the provider's
// pointer filename, used as the pointer-source icon.
type MappingRow struct {
	Provider  string
	Title     string
	Dir       string
	Profile   string
	Source    string // "pointer" | "gitconfig"
	Scope     string // ScopeCwd | ScopeAncestor | ScopeNone
	Drifted   bool
	Expiry    *time.Time // the mapped profile's token expiry; nil = none/unknown
	Unmanaged string
	Conflict  *github.Conflict
	Pointer   string
}

// AmbientRow is one provider's native default identity: what its CLI would use
// right now with no azrl involvement. Profile is the matching managed profile
// name, "" when the identity is unmanaged.
type AmbientRow struct {
	Provider string
	Title    string
	Identity string
	Source   string
	Profile  string
}

// UnmappedRow is a saved profile appearing in no mapping, kept visible so its
// expiry warnings survive the mapping-first restructure.
type UnmappedRow struct {
	Provider string
	Title    string
	Status   provider.Status
}

// Overview is the three-section aggregation shared by the TUI landing view and
// `azrl status`: MAPPINGS → AMBIENT → UNMAPPED PROFILES.
type Overview struct {
	Mappings []MappingRow
	Ambient  []AmbientRow
	Unmapped []UnmappedRow
}

// hasProfiles reports whether any saved profile exists anywhere in the
// overview (mapped or not), so empty states can distinguish "fresh machine"
// from "everything mapped".
func (ov Overview) hasProfiles() bool {
	if len(ov.Unmapped) > 0 {
		return true
	}
	for _, r := range ov.Mappings {
		if r.Profile != "" {
			return true
		}
	}
	return false
}

// BuildOverview aggregates every provider's mapping index, native ambient
// identity, and unmapped saved profiles into the shared three-section shape.
// cwd anchors the scope markers ("" disables them). Disk + process-env only,
// except the GitHub conflict/unmanaged read-back which shells to local git;
// it never touches the network and every read is best-effort.
func BuildOverview(provs []provider.Provider, cwd string) Overview {
	var ov Overview
	for _, p := range provs {
		confdir := p.ProfilesDir()
		statuses := map[string]provider.Status{}
		var listed []provider.Status
		if names, err := p.ListProfiles(confdir); err == nil {
			for _, l := range names {
				st, serr := p.Status(l.Name, confdir)
				if serr != nil {
					st = provider.Status{ProfileName: l.Name, Identity: "⚠ error"}
				}
				statuses[l.Name] = st
				listed = append(listed, st)
			}
		}

		selfHealCwd(p, confdir, cwd)
		rows := mappingRows(p, confdir, cwd, statuses)
		ov.Mappings = append(ov.Mappings, rows...)

		if amb, err := p.Ambient(); err == nil && amb.Identity != "" {
			ov.Ambient = append(ov.Ambient, AmbientRow{
				Provider: p.Name(), Title: p.Title(), Identity: amb.Identity,
				Source: amb.Source, Profile: provider.MatchProfile(listed, amb.Identity),
			})
		}

		mapped := map[string]bool{}
		for _, r := range rows {
			if r.Profile != "" {
				mapped[r.Profile] = true
			}
			if r.Conflict != nil {
				mapped[r.Conflict.PointerProfile] = true
			}
		}
		for _, st := range listed {
			if !mapped[st.ProfileName] {
				ov.Unmapped = append(ov.Unmapped, UnmappedRow{Provider: p.Name(), Title: p.Title(), Status: st})
			}
		}
	}
	sort.SliceStable(ov.Unmapped, func(i, j int) bool {
		return ov.Unmapped[i].Status.LastUsed.After(ov.Unmapped[j].Status.LastUsed)
	})
	return ov
}

// selfHealCwd records the mapping that governs cwd into the provider's index
// when azrl resolves it (REQ-022/AC-007), so hand-made pointers appear after
// `azrl status` or opening the TUI: the nearest pointer walk-up hit for every
// provider (only when it names an existing profile conf), plus GitHub's
// repo-local gitconfig resolution recorded against the repo root. Best-effort;
// RecordMapping is a no-op when the entry is already indexed.
func selfHealCwd(p provider.Provider, confdir, cwd string) {
	if cwd == "" {
		return
	}
	s := p.Scheme()
	if pdir, ok := s.Locate(cwd); ok {
		if b, err := os.ReadFile(filepath.Join(pdir, s.Pointer)); err == nil {
			if name := strings.TrimSpace(string(b)); name != "" {
				if _, err := os.Stat(filepath.Join(confdir, name+".conf")); err == nil {
					_ = profile.RecordMapping(confdir, profile.Mapping{Dir: pdir, Profile: name, Source: "pointer"})
				}
			}
		}
	}
	if p.Name() != "github" {
		return
	}
	res := github.ResolveDir(cwd, confdir)
	if res.Source != github.SourceGitConfig || res.Profile == "" {
		return
	}
	dir := cwd
	if out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output(); err == nil {
		if top := strings.TrimSpace(string(out)); top != "" {
			dir = top
		}
	}
	_ = profile.RecordMapping(confdir, profile.Mapping{Dir: dir, Profile: res.Profile, Source: github.SourceGitConfig})
}

// mappingRows renders one provider's mapping index into rows. GitHub dirs go
// through ResolveDir so git config outranks the pointer (conflict warning) and
// unmanaged git identities surface as adoptable rows; other providers render
// the index entries directly (pointer-only mechanisms). The Scheme-aware read
// self-heals pointer-sourced entries whose pointer changed underneath azrl;
// gitconfig-sourced entries heal here from the same resolution that renders
// them — a repointed credential username is re-recorded and a dir whose git
// config no longer carries one is dropped (best-effort, no-op when unchanged).
func mappingRows(p provider.Provider, confdir, cwd string, statuses map[string]provider.Status) []MappingRow {
	entries := p.Scheme().ReadMappings(confdir)
	pointer := p.Scheme().Pointer
	var rows []MappingRow
	if p.Name() == "github" {
		seen := map[string]bool{}
		for _, e := range entries {
			res := github.ResolveDir(e.Dir, confdir)
			if e.Source == github.SourceGitConfig {
				if res.Source == github.SourceGitConfig && res.Profile != "" {
					_ = profile.RecordMapping(confdir, profile.Mapping{Dir: e.Dir, Profile: res.Profile, Source: github.SourceGitConfig})
				} else if res.Unmanaged == "" {
					_ = profile.RemoveMapping(confdir, e.Dir, github.SourceGitConfig)
				}
			}
			if seen[e.Dir] {
				continue
			}
			seen[e.Dir] = true
			if res.Profile != "" {
				rows = append(rows, MappingRow{
					Provider: p.Name(), Title: p.Title(), Dir: e.Dir, Profile: res.Profile,
					Source: res.Source, Drifted: statuses[res.Profile].Drifted,
					Expiry: statuses[res.Profile].Expiry, Conflict: res.Conflict, Pointer: pointer,
				})
			}
			if res.Unmanaged != "" {
				rows = append(rows, MappingRow{
					Provider: p.Name(), Title: p.Title(), Dir: e.Dir,
					Source: github.SourceGitConfig, Unmanaged: res.Unmanaged, Pointer: pointer,
				})
			}
		}
	} else {
		for _, e := range entries {
			rows = append(rows, MappingRow{
				Provider: p.Name(), Title: p.Title(), Dir: e.Dir, Profile: e.Profile,
				Source: e.Source, Drifted: statuses[e.Profile].Drifted,
				Expiry: statuses[e.Profile].Expiry, Pointer: pointer,
			})
		}
	}
	markScope(rows, cwd)
	return rows
}

// markScope stamps each row's Scope relative to cwd. Per provider only the
// governing mapping gets a marker: the row whose dir is the cwd (ScopeCwd) or,
// failing that, the nearest ancestor of the cwd (ScopeAncestor, matching the
// pointer walk-up's nearest-wins semantics); every other row is ScopeNone.
func markScope(rows []MappingRow, cwd string) {
	for i := range rows {
		rows[i].Scope = ScopeNone
	}
	if cwd == "" {
		return
	}
	cwd = filepath.Clean(cwd)
	best, bestLen := -1, -1
	for i, r := range rows {
		d := filepath.Clean(r.Dir)
		if d != cwd && !strings.HasPrefix(cwd, d+string(filepath.Separator)) {
			continue
		}
		if len(d) > bestLen {
			best, bestLen = i, len(d)
		}
	}
	if best < 0 {
		return
	}
	if filepath.Clean(rows[best].Dir) == cwd {
		rows[best].Scope = ScopeCwd
	} else {
		rows[best].Scope = ScopeAncestor
	}
}
