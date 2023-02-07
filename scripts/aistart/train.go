package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"text/tabwriter"

	"github.com/gocarina/gocsv"
	"github.com/goml/gobrain"
	"github.com/spf13/cobra"

	"github.com/coder/flog"
)

type vector [][]float64

type pattern []vector

func (p pattern) floats() [][][]float64 {
	var r [][][]float64
	for _, v := range p {
		r = append(r, [][]float64(v))
	}
	return r
}

func vectorizeTrainingRows(rs []trainingRow) pattern {
	var p pattern
	for _, r := range rs {
		p = append(p, r.vectorize())
	}
	return p
}

func splitTrainTest(rat float64, p pattern) (train, test pattern) {
	perms := rand.Perm(len(p))
	for i, v := range p {
		if float64(perms[i])/float64(len(p)) > rat {
			test = append(test, v)
		} else {
			train = append(train, v)
		}
	}
	return train, test
}

func train() *cobra.Command {
	return &cobra.Command{
		Use: "train",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var rs []trainingRow

			err := gocsv.Unmarshal(os.Stdin, &rs)
			if err != nil {
				return err
			}

			all := vectorizeTrainingRows(rs)

			train, test := splitTrainTest(0.5, all)

			flog.Info("split train test: %v/%v", len(train), len(test))

			ff := &gobrain.FeedForward{}
			ff.Init(2, 2, 1)
			ff.Train(train.floats(), 50, 0.001, 0.4, true)
			var (
				// confusionMatrix has actual values in the first index with
				// predicted values in the second.
				confusionMatrix [2][2]int
			)
			for _, v := range train {
				want := v[1][0]
				gotArr := ff.Update(v[0])
				got := gotArr[0]
				confusionMatrix[0][int(math.Round(want))]++
				confusionMatrix[1][int(math.Round(got))]++
			}
			twr := tabwriter.NewWriter(os.Stderr, 0, 4, 3, ' ', 0)
			_, _ = fmt.Fprintf(twr, "-\tOff\tOn\n")
			_, _ = fmt.Fprintf(twr, "Actual\t%v\t%v\n", confusionMatrix[0][0], confusionMatrix[0][1])
			_, _ = fmt.Fprintf(twr, "Predicted\t%v\t%v\n", confusionMatrix[1][0], confusionMatrix[1][1])
			twr.Flush()
			return nil
		},
	}
}
