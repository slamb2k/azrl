// Package azure drives the az/ssh login lifecycle for azrl.
package azure

import (
	"encoding/json"
	"fmt"

	"github.com/slamb2k/azrl/internal/profile"
)

// AssertAccount verifies the signed-in account matches the expected tenant
// (by GUID or default domain) and, when expUser is non-empty, the user.
func AssertAccount(acctJSON []byte, expTenant, expUser string) error {
	var a profile.AccountJSON
	if err := json.Unmarshal(acctJSON, &a); err != nil {
		return fmt.Errorf("azrl: could not parse account json: %w", err)
	}
	if expTenant != a.TenantID && expTenant != a.TenantDefaultDomain {
		return fmt.Errorf("azrl: TENANT MISMATCH — expected %q, got tenantId=%q domain=%q",
			expTenant, a.TenantID, a.TenantDefaultDomain)
	}
	if expUser != "" && expUser != a.User.Name {
		return fmt.Errorf("azrl: USER MISMATCH — expected %q, got %q", expUser, a.User.Name)
	}
	return nil
}
