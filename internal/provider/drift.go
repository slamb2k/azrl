package provider

import (
	"os"

	"github.com/slamb2k/azrl/internal/profile"
)

// Drifted reports whether the ambient session for a provider differs from the
// profile pinned by the cwd. Only the cwd-pinned profile can drift: when the
// pointer here names name but the provider's ambient env var doesn't point at
// name's isolated dir (including when it's unset, so the tool falls back to its
// global config), the shell would act as a different profile than this dir pins.
func Drifted(s profile.Scheme, envVar, name, isolated string) bool {
	pwd, err := os.Getwd()
	if err != nil {
		return false
	}
	pinned, _ := s.Resolve("", pwd)
	ambient := os.Getenv(envVar)
	return pinned == name && ambient != isolated
}
