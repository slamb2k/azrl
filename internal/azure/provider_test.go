package azure_test

import (
	"testing"

	"github.com/slamb2k/azrl/internal/azure"
	"github.com/slamb2k/azrl/internal/provider/providertest"
)

func TestAzureProviderContract(t *testing.T) {
	providertest.RunContract(t, azure.NewProvider())
}
