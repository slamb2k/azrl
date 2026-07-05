package browserpick

import "testing"

const edgeState = `{"profile":{"info_cache":{
  "Default":{"name":"Personal","user_name":"me@gmail.com"},
  "Profile 2":{"name":"Work","user_name":"simon@acme.com"},
  "Profile 3":{"name":"Unsigned"}}}}`

func TestParseLocalState(t *testing.T) {
	ps := parseLocalState("edge", "linux", []byte(edgeState))
	if len(ps) != 3 {
		t.Fatalf("want 3 profiles, got %+v", ps)
	}
	// sorted by Dir: Default, Profile 2, Profile 3
	if ps[1].Dir != "Profile 2" || ps[1].Name != "Work" || ps[1].Email != "simon@acme.com" {
		t.Fatalf("got %+v", ps[1])
	}
	if ps[2].Email != "" {
		t.Fatalf("unsigned profile should have empty email: %+v", ps[2])
	}
	if parseLocalState("edge", "linux", []byte("not json")) != nil {
		t.Fatal("malformed JSON must yield nil")
	}
}

func TestClassify(t *testing.T) {
	for _, tc := range []struct{ path, browser, os string }{
		{"/home/u/.config/microsoft-edge/Local State", "edge", "linux"},
		{"/home/u/.config/google-chrome/Local State", "chrome", "linux"},
		{"/Users/u/Library/Application Support/Microsoft Edge/Local State", "edge", "macos"},
		{"/mnt/c/Users/u/AppData/Local/Google/Chrome/User Data/Local State", "chrome", "wsl"},
	} {
		b, o := classify(tc.path)
		if b != tc.browser || o != tc.os {
			t.Fatalf("%s: got (%s,%s) want (%s,%s)", tc.path, b, o, tc.browser, tc.os)
		}
	}
}

func TestCommandPerOS(t *testing.T) {
	for _, tc := range []struct {
		p    Profile
		want string
	}{
		{Profile{Browser: "edge", OS: "linux", Dir: "Profile 2"}, `microsoft-edge --profile-directory="Profile 2"`},
		{Profile{Browser: "chrome", OS: "linux", Dir: "Default"}, `google-chrome --profile-directory="Default"`},
		{Profile{Browser: "edge", OS: "macos", Dir: "Profile 2"}, `open -na "Microsoft Edge" --args --profile-directory="Profile 2"`},
		{Profile{Browser: "edge", OS: "wsl", Dir: "Profile 2"}, `"/mnt/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" --profile-directory="Profile 2"`},
		{Profile{Browser: "chrome", OS: "windows", Dir: "Profile 2"}, `"C:\Program Files\Google\Chrome\Application\chrome.exe" --profile-directory="Profile 2"`},
	} {
		if got := tc.p.Command(); got != tc.want {
			t.Fatalf("Command() = %q, want %q", got, tc.want)
		}
	}
}

func TestLabelAndKeys(t *testing.T) {
	if l := (Profile{Browser: "edge", Name: "Work"}).Label(); l != "Edge — Work" {
		t.Fatalf("label %q", l)
	}
	if c, l := Keys("gcp"); c != "GCP_BROWSER_CMD" || l != "GCP_BROWSER_LABEL" {
		t.Fatalf("gcp keys %q %q", c, l)
	}
	if c, _ := Keys("nope"); c != "" {
		t.Fatal("unknown provider must yield empty keys")
	}
}
