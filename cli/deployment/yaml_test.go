package deployment_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/deployment"
)

func TestMarshalYAML(t *testing.T) {
	t.Parallel()

	t.Run("Default", func(t *testing.T) {
		t.Parallel()
		byt, err := deployment.MarshalYAML(deployment.DefaultConfig())
		t.Logf("yaml:\n%s", string(byt))
		require.NoError(t, err)
	})
}
