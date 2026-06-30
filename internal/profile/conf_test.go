package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExplicitAndWalkUp(t *testing.T) {
	if got, _ := Resolve("fiig", "/tmp"); got != "fiig" {
		t.Fatalf("explicit: %q", got)
	}
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".azprofile"), []byte("digital-it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve("", deep)
	if err != nil || got != "digital-it" {
		t.Fatalf("walk-up: got %q err %v", got, err)
	}
	if _, err := Resolve("", t.TempDir()); err == nil {
		t.Fatal("expected error when no .azprofile")
	}
}

func TestLoadConf(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "fiig.conf"), []byte("AZ_TENANT=fiig.com.au\nAZ_DEFAULT_SUB=sub-123\n"), 0o644)
	c, err := LoadConf("fiig", dir)
	if err != nil || c.Tenant != "fiig.com.au" || c.DefaultSub != "sub-123" {
		t.Fatalf("got %+v err %v", c, err)
	}
	os.WriteFile(filepath.Join(dir, "bad.conf"), []byte("AZ_DEFAULT_SUB=x\n"), 0o644)
	if _, err := LoadConf("bad", dir); err == nil {
		t.Fatal("expected error for missing AZ_TENANT")
	}
}

func TestBuildAndWriteConf(t *testing.T) {
	acct := AccountJSON{TenantID: "guid-1", ID: "sub-9", User: struct {
		Name string `json:"name"`
	}{Name: "u@onenrg.onmicrosoft.com"}}
	doms := DomainsJSON{Value: []Domain{{ID: "onenrg.mail.onmicrosoft.com"}, {ID: "onenrg.onmicrosoft.com", IsDefault: true}}}
	c := BuildConf(acct, doms)
	if c.Tenant != "onenrg.onmicrosoft.com" || c.TenantID != "guid-1" || c.DefaultSub != "sub-9" {
		t.Fatalf("got %+v", c)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "nrg.conf")
	if err := c.Write(path); err != nil {
		t.Fatal(err)
	}
	rd, _ := LoadConf("nrg", dir)
	if rd.Tenant != "onenrg.onmicrosoft.com" || rd.ExpectUser != "u@onenrg.onmicrosoft.com" {
		t.Fatalf("roundtrip got %+v", rd)
	}
}

func TestBuildConfFallsBackToTenantID(t *testing.T) {
	c := BuildConf(AccountJSON{TenantID: "guid-2"}, DomainsJSON{})
	if c.Tenant != "guid-2" || c.TenantID != "guid-2" {
		t.Fatalf("got %+v", c)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "fiig.conf"), []byte("AZ_TENANT=fiig.com.au\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "nrg.conf"), []byte("AZ_TENANT=onenrg.onmicrosoft.com\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "azrl.conf"), []byte("LOCAL_HOST=x\n"), 0o644)
	got, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 profiles, got %d: %+v", len(got), got)
	}
	for _, p := range got {
		if p.Name == "azrl" {
			t.Fatal("azrl.conf must be excluded")
		}
	}
}
