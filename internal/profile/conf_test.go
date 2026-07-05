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
	c.BrowserCmd = `google-chrome --profile-directory="Profile 2"`
	c.BrowserLabel = "Edge — Work"
	if c.Tenant != "onenrg.onmicrosoft.com" || c.TenantID != "guid-1" || c.DefaultSub != "sub-9" {
		t.Fatalf("got %+v", c)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "nrg.conf")
	if err := c.Write(path); err != nil {
		t.Fatal(err)
	}
	rd, _ := LoadConf("nrg", dir)
	if rd.Tenant != "onenrg.onmicrosoft.com" || rd.ExpectUser != "u@onenrg.onmicrosoft.com" ||
		rd.BrowserCmd != `google-chrome --profile-directory="Profile 2"` ||
		rd.BrowserLabel != "Edge — Work" {
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

func TestLabelRoundTripAndSetLabel(t *testing.T) {
	confdir := t.TempDir()
	os.WriteFile(filepath.Join(confdir, "acme.conf"), []byte("AZ_TENANT=acme.com\n"), 0o644)

	// no label yet: List.Display falls back to the slug.
	profs, _ := List(confdir)
	if len(profs) != 1 || profs[0].Display() != "acme" {
		t.Fatalf("display fallback: %+v", profs)
	}

	// set a label with spaces; the slug (filename) is untouched.
	if err := SetLabel("acme", confdir, "Acme Production"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(confdir, "acme.conf")); err != nil {
		t.Fatal("slug conf must remain acme.conf after relabel")
	}
	c, err := LoadConf("acme", confdir)
	if err != nil || c.Label != "Acme Production" {
		t.Fatalf("label not persisted: label=%q err=%v", c.Label, err)
	}
	if c.Tenant != "acme.com" {
		t.Fatalf("relabel clobbered tenant: %q", c.Tenant)
	}
	profs, _ = List(confdir)
	if profs[0].Display() != "Acme Production" || profs[0].Name != "acme" {
		t.Fatalf("list display/slug: %+v", profs[0])
	}

	// empty label reverts display to the slug.
	if err := SetLabel("acme", confdir, ""); err != nil {
		t.Fatal(err)
	}
	profs, _ = List(confdir)
	if profs[0].Display() != "acme" {
		t.Fatalf("empty label should revert to slug: %+v", profs[0])
	}
}
