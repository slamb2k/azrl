package envdetect

import "testing"

func hasNone(string) bool  { return false }
func hasXdg(b string) bool { return b == "xdg-open" }

func TestDeriveVMHost(t *testing.T) {
	cases := map[string]string{
		"198.51.100.2 51000 203.0.113.10 22": "203.0.113.10",
		"":                                   "",
		"just one":                           "",
		"a b c":                              "", // three fields is malformed (need 4)
		"a b c d e":                          "c",
	}
	for in, want := range cases {
		if got := DeriveVMHost(in); got != want {
			t.Errorf("DeriveVMHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// find returns the first candidate of the given mode, or nil.
func find(cands []Candidate, mode Mode) *Candidate {
	for i := range cands {
		if cands[i].Mode == mode {
			return &cands[i]
		}
	}
	return nil
}

func TestDetectWSL(t *testing.T) {
	c := Detect(Env{WSLDistro: "Ubuntu", GOOS: "linux", Has: hasNone})
	if len(c) != 1 || c[0].Mode != Local || !c[0].Recommended {
		t.Fatalf("WSL should yield a single recommended local candidate: %+v", c)
	}
	if c[0].BrowserCmd != "wslview" || c[0].BrowserHost != "localhost" {
		t.Fatalf("WSL candidate = %+v", c[0])
	}
}

func TestDetectWSLViaProcVersion(t *testing.T) {
	c := Detect(Env{ProcVersion: "Linux version 5.15.0-microsoft-standard-WSL2", GOOS: "linux", Has: hasNone})
	if len(c) != 1 || c[0].BrowserCmd != "wslview" {
		t.Fatalf("proc-version WSL detection failed: %+v", c)
	}
}

func TestDetectDarwin(t *testing.T) {
	c := Detect(Env{GOOS: "darwin", Has: hasNone})
	if len(c) != 1 || c[0].BrowserCmd != "open" || c[0].BrowserHost != "localhost" {
		t.Fatalf("darwin candidate = %+v", c)
	}
}

func TestDetectLinuxDesktop(t *testing.T) {
	c := Detect(Env{GOOS: "linux", Display: ":0", Has: hasXdg})
	if len(c) != 1 || c[0].BrowserCmd != "xdg-open" {
		t.Fatalf("linux desktop candidate = %+v", c)
	}
	// Without xdg-open on PATH there is no local signal → single remote candidate.
	c = Detect(Env{GOOS: "linux", Display: ":0", Has: hasNone})
	if len(c) != 1 || c[0].Mode != Remote {
		t.Fatalf("no xdg-open should fall through to remote: %+v", c)
	}
}

func TestDetectSSHOnly(t *testing.T) {
	c := Detect(Env{GOOS: "linux", SSHConnection: "198.51.100.2 51000 203.0.113.10 22", Has: hasNone})
	if len(c) != 1 || c[0].Mode != Remote || !c[0].Recommended {
		t.Fatalf("SSH-only should yield a recommended remote candidate: %+v", c)
	}
	if c[0].VMSSHHost != "203.0.113.10" || c[0].BrowserCmd != "" {
		t.Fatalf("remote candidate = %+v", c[0])
	}
}

func TestDetectWSLPlusSSH(t *testing.T) {
	c := Detect(Env{WSLDistro: "Ubuntu", GOOS: "linux", SSHConnection: "198.51.100.2 51000 203.0.113.10 22", Has: hasNone})
	if len(c) != 2 {
		t.Fatalf("WSL+SSH should yield two candidates: %+v", c)
	}
	if c[0].Mode != Remote || !c[0].Recommended {
		t.Fatalf("remote must be first and recommended: %+v", c)
	}
	if l := find(c, Local); l == nil || l.Recommended {
		t.Fatalf("local must be present and not recommended: %+v", c)
	}
}

func TestDetectEmpty(t *testing.T) {
	c := Detect(Env{GOOS: "linux", Has: hasNone})
	if len(c) != 1 || c[0].Mode != Remote || c[0].VMSSHHost != "" || !c[0].Recommended {
		t.Fatalf("empty env should yield one blank recommended remote candidate: %+v", c)
	}
}
