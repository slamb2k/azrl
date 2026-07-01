package gcp

import (
	"errors"
	"testing"
)

func TestGKEWarningPureLogic(t *testing.T) {
	found := func(string) (string, error) { return "/usr/bin/gke-gcloud-auth-plugin", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	yes := func() bool { return true }
	no := func() bool { return false }

	// Not isolated → never warn, even with the plugin and a GKE context present.
	if got := gkeWarning(false, found, yes); got != "" {
		t.Fatalf("non-isolate must stay silent, got %q", got)
	}
	// Isolated + plugin on PATH → warn.
	if got := gkeWarning(true, found, no); got == "" {
		t.Fatal("isolate with the plugin on PATH should warn")
	}
	// Isolated + GKE kubeconfig context → warn.
	if got := gkeWarning(true, missing, yes); got == "" {
		t.Fatal("isolate with a GKE kubeconfig context should warn")
	}
	// Isolated but neither signal → silent (the probe-env case).
	if got := gkeWarning(true, missing, no); got != "" {
		t.Fatalf("isolate without GKE should stay silent, got %q", got)
	}
}
