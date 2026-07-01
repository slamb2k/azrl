package browsercapture

import "testing"

func TestParseCallbackPort(t *testing.T) {
	cases := map[string]string{
		// encoded 127.0.0.1 loopback (GCM shape)
		"https://github.com/login/oauth/authorize?client_id=x&redirect_uri=http%3A%2F%2F127.0.0.1%3A52001%2F&state=y": "52001",
		// plain localhost loopback
		"https://login/x?redirect_uri=http://localhost:38149/&s=y": "38149",
		// unencoded 127.0.0.1
		"https://h/o?redirect_uri=http://127.0.0.1:44444/callback": "44444",
		// device flow — no loopback redirect
		"https://github.com/login/device": "",
		// authorize with no redirect_uri
		"https://github.com/login/oauth/authorize?client_id=x&scope=repo": "",
		// AWS SSO PKCE loopback: encoded 127.0.0.1 with an /oauth/callback path
		"https://oidc.us-east-1.amazonaws.com/authorize?response_type=code&redirect_uri=http%3A%2F%2F127.0.0.1%3A55021%2Foauth%2Fcallback&code_challenge=x": "55021",
		// AWS SSO device-code shape: a verification URL with no redirect_uri
		"https://device.sso.us-east-1.amazonaws.com/?user_code=ABCD-EFGH": "",
	}
	for in, want := range cases {
		if got := ParseCallbackPort(in); got != want {
			t.Errorf("ParseCallbackPort(%q)=%q want %q", in, got, want)
		}
	}
}

func TestClassify(t *testing.T) {
	if got := Classify("https://h/o?redirect_uri=http://127.0.0.1:44444/"); got != Loopback {
		t.Errorf("loopback URL classified as %v", got)
	}
	if got := Classify("https://github.com/login/device"); got != Device {
		t.Errorf("device URL classified as %v", got)
	}
	if got := Classify("https://github.com/login/oauth/authorize?client_id=x"); got != Device {
		t.Errorf("no-redirect authorize classified as %v", got)
	}
	if got := Classify("https://oidc.us-east-1.amazonaws.com/authorize?redirect_uri=http%3A%2F%2F127.0.0.1%3A55021%2Foauth%2Fcallback"); got != Loopback {
		t.Errorf("AWS SSO PKCE URL classified as %v, want Loopback", got)
	}
	if got := Classify("https://device.sso.us-east-1.amazonaws.com/?user_code=ABCD-EFGH"); got != Device {
		t.Errorf("AWS SSO device URL classified as %v, want Device", got)
	}
}
