package builder

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuilder(t *testing.T) {
	out, err := buildTemplate(TemplateInput{
		TemplateName: "test",
		Kubernetes: KubeOptions{
			Os:        "",
			Arch:      "",
			Namespace: Variable{},
			Image:     "",
			Resources: Resources{},
			Env:       nil,
			HomePVC:   false,
		},
	})
	require.NoError(t, err)
	// This output is a tar file
	fmt.Println(string(out))
}
