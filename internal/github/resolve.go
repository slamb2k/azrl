package github

import (
	"os/exec"
	"strings"
)

// Resolution sources: the repo-local git credential username vs the .ghprofile
// pointer walk-up.
const (
	SourceGitConfig = "gitconfig"
	SourcePointer   = "pointer"
)

// Resolution reports how a directory maps to a GitHub profile. Profile is the
// winning profile name ("" when none resolves) and Source says where it came
// from. Unmanaged carries a repo-local git-config identity (<user>@<host>)
// that matches no saved profile (adoptable); it may coexist with a
// pointer-sourced Profile. Conflict is set when the git config and a
// .ghprofile disagree in the same repo — the git config wins.
type Resolution struct {
	Profile   string
	Source    string
	Unmanaged string
	Conflict  *Conflict
}

// Conflict carries both sides of a git-config/.ghprofile disagreement so
// callers can render the warning: the winning profile and identity from the
// repo-local git config, and the losing profile named by the pointer.
type Conflict struct {
	GitConfigProfile string
	GitConfigUser    string
	PointerProfile   string
}

// ResolveDir resolves the profile governing dir, native-first: the enclosing
// repo's local git config credential.https://<host>.username mapped to the
// profile whose conf GH_USER+GH_HOST match, then the .ghprofile walk-up, then
// none.
func ResolveDir(dir, confdir string) Resolution {
	pointer, err := scheme.Resolve("", dir)
	if err != nil {
		pointer = ""
	}
	var res Resolution
	if creds := credentialUsernames(dir); len(creds) > 0 {
		if name, identity, ok := matchProfile(creds, confdir); ok {
			res.Profile = name
			res.Source = SourceGitConfig
			if pointer != "" && pointer != name {
				res.Conflict = &Conflict{GitConfigProfile: name, GitConfigUser: identity, PointerProfile: pointer}
			}
			return res
		}
		res.Unmanaged = creds[0].user + "@" + creds[0].host
	}
	if pointer != "" {
		res.Profile = pointer
		res.Source = SourcePointer
	}
	return res
}

// credentialEntry is one repo-local credential.https://<host>.username value.
type credentialEntry struct {
	host string
	user string
}

// credentialUsernames reads every repo-local credential.https://<host>.username
// from the git repo enclosing dir (the exact key SetupRepo writes). No repo,
// no git, or no entries all yield nil.
func credentialUsernames(dir string) []credentialEntry {
	out, err := exec.Command("git", "-C", dir, "config", "--local", "--get-regexp",
		`^credential\.https://.*\.username$`).Output()
	if err != nil {
		return nil
	}
	var entries []credentialEntry
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		key, user, ok := strings.Cut(line, " ")
		if !ok || user == "" {
			continue
		}
		if !strings.HasPrefix(key, "credential.https://") || !strings.HasSuffix(key, ".username") {
			continue
		}
		host := strings.TrimSuffix(strings.TrimPrefix(key, "credential.https://"), ".username")
		if host == "" {
			continue
		}
		entries = append(entries, credentialEntry{host: host, user: user})
	}
	return entries
}

// matchProfile maps the first git-config identity whose host and user equal a
// saved profile's GH_HOST+GH_USER. identity is "<user>@<host>".
func matchProfile(creds []credentialEntry, confdir string) (name, identity string, ok bool) {
	profiles, err := scheme.List(confdir)
	if err != nil {
		return "", "", false
	}
	for _, e := range creds {
		for _, p := range profiles {
			c, cerr := LoadConf(p.Name, confdir)
			if cerr != nil {
				continue
			}
			if c.Host == e.host && c.User != "" && c.User == e.user {
				return p.Name, e.user + "@" + e.host, true
			}
		}
	}
	return "", "", false
}
