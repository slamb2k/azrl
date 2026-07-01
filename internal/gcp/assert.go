package gcp

import (
	"fmt"
	"strings"
)

// AssertAccount verifies the live active account (from `gcloud auth list`)
// matches the expected account email (exact match). An empty expectation skips
// the check, mirroring the AWS assert.
func AssertAccount(activeAccount, expectAccount string) error {
	if expectAccount == "" {
		return nil
	}
	got := strings.TrimSpace(activeAccount)
	if got != expectAccount {
		return fmt.Errorf("gcp: ACCOUNT MISMATCH — expected %q, got %q", expectAccount, got)
	}
	return nil
}
