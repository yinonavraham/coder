package main

import (
	_ "time/tzdata"

	"github.com/coder/coder/cli/clitiming"
	entcli "github.com/coder/coder/enterprise/cli"
)

func main() {
	var rootCmd entcli.RootCmd
	clitiming.Record("enter main")
	defer clitiming.Record("exit main")
	rootCmd.RunMain(rootCmd.EnterpriseSubcommands())
}
