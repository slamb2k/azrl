package aws

import "testing"

func TestAssertAccount(t *testing.T) {
	ok := `{"Account":"123456789012","Arn":"arn:aws:sts::123456789012:assumed-role/AWSReservedSSO_AdminAccess_a1b2c3d4/simon"}`
	if err := AssertAccount([]byte(ok), "123456789012", "AWSReservedSSO_AdminAccess"); err != nil {
		t.Fatalf("matching account+role: %v", err)
	}
	if err := AssertAccount([]byte(ok), "999999999999", "AWSReservedSSO_AdminAccess"); err == nil {
		t.Fatal("wrong account should error")
	}
	if err := AssertAccount([]byte(ok), "123456789012", "AWSReservedSSO_ReadOnly"); err == nil {
		t.Fatal("wrong role should error")
	}
	if err := AssertAccount([]byte(ok), "123456789012", ""); err != nil {
		t.Fatalf("empty expectARN should skip role check: %v", err)
	}
	if err := AssertAccount([]byte(`{"Account":"1","Arn":"arn:aws:iam::1:user/bob"}`), "1", "AWSReservedSSO_AdminAccess"); err == nil {
		t.Fatal("non-assumed-role ARN with a role expectation should error")
	}
}

// TestAssertAccountRoleBoundary guards against a fail-open prefix match: the
// expected permission set must line up on the role segment's '_' boundary, so
// "AWSReservedSSO_Admin" matches "AWSReservedSSO_Admin_<hash>" but not the
// unrelated "AWSReservedSSO_AdminReadOnly_<hash>".
func TestAssertAccountRoleBoundary(t *testing.T) {
	admin := `{"Account":"123456789012","Arn":"arn:aws:sts::123456789012:assumed-role/AWSReservedSSO_Admin_a1b2c3d4/simon"}`
	if err := AssertAccount([]byte(admin), "123456789012", "AWSReservedSSO_Admin"); err != nil {
		t.Fatalf("AWSReservedSSO_Admin should match AWSReservedSSO_Admin_<hash>: %v", err)
	}
	readonly := `{"Account":"123456789012","Arn":"arn:aws:sts::123456789012:assumed-role/AWSReservedSSO_AdminReadOnly_a1b2c3d4/simon"}`
	if err := AssertAccount([]byte(readonly), "123456789012", "AWSReservedSSO_Admin"); err == nil {
		t.Fatal("AWSReservedSSO_Admin must NOT match AWSReservedSSO_AdminReadOnly_<hash>")
	}
}
