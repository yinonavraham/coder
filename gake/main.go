package main

import (
	"time"

	"github.com/coder/flog"
	"github.com/coder/gake"
)

var allGoFiles = gake.CachedFileIndex(&gake.FileIndex{
	Match: []string{".*\\.go$"},
})

func fmt() *gake.Target {
	return &gake.Target{
		Name:  "fmt",
		Takes: nil,
		Gives: allGoFiles,
		Run: func(ctx *gake.Context) error {
			var n int
			start := time.Now()
			err := ctx.Gives.IndexFiles(func(f gake.File) error {
				flog.Infof("fmt %v", f)
				n++
				return nil
			})
			if err != nil {
				return err
			}
			flog.Infof("formatted %v files in %v", n, time.Since(start))
			return nil
		},
	}
}

func main() {
	tr := gake.NewTree()
	tr.Add(fmt())
	err := tr.Run("fmt")
	if err != nil {
		flog.Fatalf("error: %v", err)
	}
}
