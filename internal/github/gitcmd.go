package github

import (
	"os"
	"os/exec"
	"strings"
)

// GitCmd builds a `git -C dir <args>` invocation with GIT_DIR, GIT_WORK_TREE,
// and GIT_INDEX_FILE stripped from its environment. Git prioritizes those env
// vars over -C's target directory, so any process that inherits them (a git
// hook, an editor's git integration) would otherwise silently run every -C
// dir command against the wrong repository instead of dir.
func GitCmd(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	env := os.Environ()
	out := env[:0]
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_DIR=") || strings.HasPrefix(e, "GIT_WORK_TREE=") || strings.HasPrefix(e, "GIT_INDEX_FILE=") {
			continue
		}
		out = append(out, e)
	}
	cmd.Env = out
	return cmd
}
