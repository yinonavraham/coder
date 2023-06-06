package cli

import (
	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/clitiming"
)

type RootCmd struct {
	cli.RootCmd
}

func (r *RootCmd) enterpriseOnly() []*clibase.Cmd {
	clitiming.Record("enter enterpriseOnly")
	defer clitiming.Record("exit enterpriseOnly")

	return []*clibase.Cmd{
		r.server(),
		r.workspaceProxy(),
		r.features(),
		r.licenses(),
		r.groups(),
		r.provisionerDaemons(),
	}
}

func (r *RootCmd) EnterpriseSubcommands() []*clibase.Cmd {
	clitiming.Record("enter EnterpriseSubcommands")
	defer clitiming.Record("exit EnterpriseSubcommands")

	all := append(r.Core(), r.enterpriseOnly()...)
	return all
}
