package aws_test

import (
	"testing"

	"github.com/slamb2k/azrl/internal/aws"
	"github.com/slamb2k/azrl/internal/provider/providertest"
)

func TestAWSProviderContract(t *testing.T) {
	providertest.RunContract(t, aws.NewProvider())
}
