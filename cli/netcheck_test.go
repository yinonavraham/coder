package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/healthcheck"
	"github.com/coder/coder/pty/ptytest"
)

func TestNetcheck(t *testing.T) {
	t.Parallel()

	pty := ptytest.New(t)
	config := login(t, pty)

	var out bytes.Buffer
	inv, _ := clitest.New(t, "netcheck", "--global-config", string(config))
	inv.Stdout = &out

	clitest.StartWithWaiter(t, inv).RequireSuccess()

	b := out.Bytes()
	t.Log(string(b))
	var report healthcheck.DERPReport
	require.NoError(t, json.Unmarshal(b, &report))

	assert.True(t, report.Healthy)
	require.Len(t, report.Regions, 1)
	for _, v := range report.Regions {
		require.Len(t, v.NodeReports, len(v.Region.Nodes))
	}
}
