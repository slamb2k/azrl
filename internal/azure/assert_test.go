package azure

import "testing"

func TestAssertAccount(t *testing.T) {
	ok := `{"tenantId":"g","tenantDefaultDomain":"fiig.com.au","user":{"name":"simon@fiig.com.au"}}`
	if err := AssertAccount([]byte(ok), "fiig.com.au", "simon@fiig.com.au"); err != nil {
		t.Fatalf("domain+user: %v", err)
	}
	if err := AssertAccount([]byte(ok), "g", ""); err != nil {
		t.Fatalf("by guid: %v", err)
	}
	guest := `{"tenantId":"96e360c3","tenantDefaultDomain":null,"user":{"name":"S@velrada.com"}}`
	if err := AssertAccount([]byte(guest), "96e360c3", "S@velrada.com"); err != nil {
		t.Fatalf("guest guid: %v", err)
	}
	if err := AssertAccount([]byte(ok), "other.com", ""); err == nil {
		t.Fatal("tenant mismatch should error")
	}
	if err := AssertAccount([]byte(ok), "fiig.com.au", "wrong@x"); err == nil {
		t.Fatal("user mismatch should error")
	}
}
