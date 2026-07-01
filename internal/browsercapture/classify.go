package browsercapture

import (
	"regexp"
	"strings"
)

// Kind is the flavour of a URL a tool asked the shim to open.
type Kind int

const (
	// Device is a device-code or plain URL with no loopback callback — relay it
	// to the local browser and let the VM tool poll for the token.
	Device Kind = iota
	// Loopback is an OAuth URL whose redirect_uri points at 127.0.0.1/localhost —
	// tunnel the callback port back to the VM.
	Loopback
)

// callbackPortRe matches a loopback callback host:port in a redirect_uri, after
// %3A/%2F decoding, for both 127.0.0.1 and localhost.
var callbackPortRe = regexp.MustCompile(`redirect_uri=[^&]*(?:127\.0\.0\.1|localhost):(\d+)`)

// ParseCallbackPort returns the loopback callback port from a URL's redirect_uri
// (decoding the common %3A/%2F encodings first), or "" when there is none.
func ParseCallbackPort(url string) string {
	d := strings.ReplaceAll(url, "%3A", ":")
	d = strings.ReplaceAll(d, "%2F", "/")
	m := callbackPortRe.FindStringSubmatch(d)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// Classify reports whether url needs a loopback tunnel or a device-style relay.
func Classify(url string) Kind {
	if ParseCallbackPort(url) != "" {
		return Loopback
	}
	return Device
}
