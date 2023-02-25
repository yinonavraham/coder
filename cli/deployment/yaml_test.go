package deployment_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/cli/deployment"
)

func TestMarshalYAML(t *testing.T) {
	t.Parallel()

	t.Run("Default", func(t *testing.T) {
		t.Parallel()
		config := deployment.DefaultConfig()
		// For testing array marshaling.
		config.ProxyTrustedHeaders.Value = []string{"X-Forwarded-For", "X-Forwarded-Proto"}
		byt, err := deployment.MarshalYAML(config)
		t.Logf("yaml:\n%s", string(byt))
		require.NoError(t, err)
	})

	t.Run("TestYAMLDecode", func(t *testing.T) {
		t.Parallel()
		var a struct {
			B string
			C string
		}
		a.B = "dog"
		a.C = "cat"

		b, err := yaml.Marshal(a)
		require.NoError(t, err)

		t.Logf("yaml\n%s", string(b))

		var n yaml.Node
		err = yaml.Unmarshal(b, &n)
		require.NoError(t, err)

		_, err = yaml.Marshal(n.Content[0])
		require.NoError(t, err)

		t.Logf("len(n.Content) = %d", len(n.Content))
		t.Logf("n = %+v", n)

		t.Logf("n.Content[0] = %+v", n.Content[0])
		t.Logf("n.Content[0].Content[0] = %+v", n.Content[0].Content[0])
		t.Logf("n.Content[0].Content[1] = %+v", n.Content[0].Content[1])
	})
}
