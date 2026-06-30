package profile

import "testing"

func TestExtractPort(t *testing.T) {
	cases := map[string]string{
		"https://login/x?redirect_uri=http%3A%2F%2Flocalhost%3A38149%2F&s=y": "38149",
		"https://login/x?redirect_uri=http://localhost:55322/&s=y":           "55322",
		"https://login/no-port": "",
	}
	for in, want := range cases {
		if got := ExtractPort(in); got != want {
			t.Errorf("ExtractPort(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"Contoso Migration": "contoso-migration",
		"  --Foo__Bar!!  ":  "foo__bar",
	}
	for in, want := range cases {
		if got := SanitizeName(in); got != want {
			t.Errorf("SanitizeName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDefaultName(t *testing.T) {
	if got := DefaultName("My Profile", "/home/x/whatever"); got != "My Profile" {
		t.Errorf("explicit arg: got %q", got)
	}
	if got := DefaultName("", "/home/x/Contoso Migration"); got != "contoso-migration" {
		t.Errorf("fallback: got %q", got)
	}
}
