package ui

import (
	"strings"
	"testing"
	"time"
)

func TestExpiryWordPerProvider(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	live := time.Now().Add(30 * time.Minute)

	// Azure/GCP: a stale access token is refreshed silently on next use.
	for _, prov := range []string{"azure", "gcp"} {
		if w := expiryWord(prov, &past); !strings.Contains(w, "token stale · refreshes on next use") {
			t.Fatalf("%s stale word = %q", prov, w)
		}
		if w := expiryWord(prov, &live); !strings.Contains(w, "left") {
			t.Fatalf("%s live expiry should still show the countdown: %q", prov, w)
		}
	}
	// AWS: expired means sign in again.
	if w := expiryWord("aws", &past); !strings.Contains(w, "expired") {
		t.Fatalf("aws expired word = %q", w)
	}
	// GitHub / none tracked: empty.
	if w := expiryWord("github", nil); w != "" {
		t.Fatalf("nil expiry should render empty, got %q", w)
	}
}
