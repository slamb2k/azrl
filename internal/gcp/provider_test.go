package gcp_test

import (
	"testing"

	"github.com/slamb2k/azrl/internal/gcp"
	"github.com/slamb2k/azrl/internal/provider/providertest"
)

func TestGCPProviderContract(t *testing.T) {
	providertest.RunContract(t, gcp.NewProvider())
}
