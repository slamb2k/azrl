package gcp

import "testing"

func TestAssertAccount(t *testing.T) {
	if err := AssertAccount("simon@acme.com", "simon@acme.com"); err != nil {
		t.Fatalf("matching account: %v", err)
	}
	if err := AssertAccount("  simon@acme.com\n", "simon@acme.com"); err != nil {
		t.Fatalf("whitespace should be trimmed: %v", err)
	}
	if err := AssertAccount("other@acme.com", "simon@acme.com"); err == nil {
		t.Fatal("wrong account should error")
	}
	if err := AssertAccount("", "simon@acme.com"); err == nil {
		t.Fatal("empty active account against an expectation should error")
	}
	if err := AssertAccount("anyone@acme.com", ""); err != nil {
		t.Fatalf("empty expectation should skip the check: %v", err)
	}
}
