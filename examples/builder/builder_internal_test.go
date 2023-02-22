package builder

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuilder(t *testing.T) {
	out, err := buildTemplate(ContainerInput{})
	require.NoError(t, err)
	fmt.Println(out)
}
