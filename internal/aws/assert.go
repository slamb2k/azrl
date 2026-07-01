package aws

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// assumedRoleRe extracts the role name from an STS assumed-role ARN, e.g.
// arn:aws:sts::123456789012:assumed-role/AWSReservedSSO_Admin_abc123/user.
var assumedRoleRe = regexp.MustCompile(`assumed-role/([^/]+)/`)

// AssertAccount verifies an `aws sts get-caller-identity` result matches the
// expected account (exact match) and permission-set role. expectARN holds the
// role's stable prefix (AWSReservedSSO_<permset>); the live ARN's role segment
// carries an extra _<hash> suffix, so a prefix match is what identifies it.
func AssertAccount(callerIdentityJSON []byte, expectAccount, expectARN string) error {
	var id struct {
		Account string `json:"Account"`
		Arn     string `json:"Arn"`
	}
	if err := json.Unmarshal(callerIdentityJSON, &id); err != nil {
		return fmt.Errorf("aws: could not parse caller identity json: %w", err)
	}
	if expectAccount != "" && expectAccount != id.Account {
		return fmt.Errorf("aws: ACCOUNT MISMATCH — expected %q, got %q", expectAccount, id.Account)
	}
	if expectARN != "" {
		m := assumedRoleRe.FindStringSubmatch(id.Arn)
		if len(m) < 2 {
			return fmt.Errorf("aws: could not extract assumed-role from %q", id.Arn)
		}
		if m[1] != expectARN && !strings.HasPrefix(m[1], expectARN+"_") {
			return fmt.Errorf("aws: ROLE MISMATCH — expected role prefix %q, got %q", expectARN, m[1])
		}
	}
	return nil
}
