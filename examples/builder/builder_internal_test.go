package builder

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuilder(t *testing.T) {
	out, err := buildTemplate(TemplateInput{
		TemplateName: "test",
		Docker:       &DockerOptions{},
	})
	require.NoError(t, err)
	// This output is a tar file
	fmt.Println(string(out))
}
