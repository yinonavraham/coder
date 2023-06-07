package main

import (
	"os"
	"runtime"
	"runtime/pprof"
	_ "time/tzdata"

	"github.com/coder/coder/cli/clitiming"
	entcli "github.com/coder/coder/enterprise/cli"
)

func main() {
	var rootCmd entcli.RootCmd
	clitiming.Record("enter main")
	defer clitiming.Record("exit main")

	pprofFi, err := os.OpenFile("/tmp/cpu.pprof", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		panic(err)
	}

	runtime.SetCPUProfileRate(2000)
	err = pprof.StartCPUProfile(pprofFi)
	if err != nil {
		panic(err)
	}

	defer pprof.StopCPUProfile()
	rootCmd.RunMain(rootCmd.EnterpriseSubcommands())
}
