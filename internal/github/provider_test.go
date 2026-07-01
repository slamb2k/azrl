package github_test

import (
	"testing"

	"github.com/slamb2k/azrl/internal/github"
	"github.com/slamb2k/azrl/internal/provider/providertest"
)

func TestGitHubProviderContract(t *testing.T) {
	providertest.RunContract(t, github.NewProvider())
}
