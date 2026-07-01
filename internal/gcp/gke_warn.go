package gcp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gkeWarningText is the warning surfaced when full config-dir isolation is on
// AND GKE usage is detected — because gke-gcloud-auth-plugin does not respect
// CLOUDSDK_CONFIG (kubernetes/cloud-provider-gcp#554, closed "not planned"), so
// kubectl/GKE credential resolution can silently use the wrong cached account.
const gkeWarningText = "gcp: WARNING — GKE detected with an isolated CLOUDSDK_CONFIG. " +
	"gke-gcloud-auth-plugin ignores CLOUDSDK_CONFIG (cloud-provider-gcp#554), so " +
	"kubectl may authenticate as the wrong cached account. Verify the active GKE credential."

// GKEIsolationWarning returns the GKE isolation warning when isolate is true and
// GKE usage is detected on this machine (the plugin on PATH or a GKE kubeconfig
// context), else "". It is the exported, side-effecting entry point.
func GKEIsolationWarning(isolate bool) string {
	return gkeWarning(isolate, exec.LookPath, kubeconfigHasGKEContext)
}

// gkeWarning is the pure, injectable core: lookPath resolves executables and
// hasGKE reports a GKE kubeconfig context, so both signals are testable with
// synthetic fixtures.
func gkeWarning(isolate bool, lookPath func(string) (string, error), hasGKE func() bool) string {
	if !isolate {
		return ""
	}
	if _, err := lookPath("gke-gcloud-auth-plugin"); err == nil {
		return gkeWarningText
	}
	if hasGKE() {
		return gkeWarningText
	}
	return ""
}

// kubeconfigHasGKEContext reports whether the active kubeconfig (KUBECONFIG or
// ~/.kube/config) names a GKE context (gke_ / gke-gcloud). Best-effort: any read
// error reads as "no GKE".
func kubeconfigHasGKEContext() bool {
	path := os.Getenv("KUBECONFIG")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		path = filepath.Join(home, ".kube", "config")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	body := string(b)
	return strings.Contains(body, "gke_") || strings.Contains(body, "gke-gcloud")
}
