package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"text/tabwriter"

	"github.com/gocarina/gocsv"
	"github.com/patrikeh/go-deep"
	"github.com/patrikeh/go-deep/training"
	"github.com/spf13/cobra"

	"github.com/coder/flog"
)

func combineSlices[T any](ss ...[]T) []T {
	var r []T
	for _, s := range ss {
		r = append(r, s...)
	}
	return r
}

func repeat[T any](v T, n int) []T {
	var r []T
	for i := 0; i < n; i++ {
		r = append(r, v)
	}
	return r
}

func nHotNorm(fs []float64) []float64 {
	for i, f := range fs {
		fs[i] = f / float64(len(fs))
	}
	return fs
}

func vectorizeTrainingRows(rs []trainingRow) training.Examples {
	var es training.Examples

	var hoursSinceUseds []float64
	// First pass for normalization.
	for _, r := range rs {
		hoursSinceUseds = append(hoursSinceUseds, float64(r.HoursSinceUsed))
	}
	deep.Normalize(hoursSinceUseds)

	for i, r := range rs {
		_ = i
		es = append(es,
			training.Example{
				Input: combineSlices(
					[]float64{
						hoursSinceUseds[i],
						float64(r.HourOfDay) / 23,
						float64(r.DayOfWeek) / 6,
					},
					// nHotNorm(r.vectorizeWeekday()),
					// nHotNorm(r.vectorizeHourOfDay()),
				),
				Response: []float64{
					float64(r.Used),
				},
			},
		)
	}
	return es
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

			// Need determinism
			rand.Seed(0)
			train, test := all.Split(0.5)

			numInputNeurons := len(
				vectorizeTrainingRows([]trainingRow{{}})[0].Input,
			)
			nn := deep.NewNeural(&deep.Config{
				/* Input dimensionality */
				Inputs: numInputNeurons,
				/* Two hidden layers consisting of two neurons each, and a single output */
				Layout: []int{numInputNeurons, 10, 1},
				/* Activation functions: Sigmoid, Tanh, ReLU, Linear */
				Activation: deep.ActivationSigmoid,
				/* Determines output layer activation & loss function:
				ModeRegression: linear outputs with MSE loss
				ModeMultiClass: softmax output with Cross Entropy loss
				ModeMultiLabel: sigmoid output with Cross Entropy loss
				ModeBinary: sigmoid output with binary CE loss */
				Mode: deep.ModeBinary,
				/* Weight initializers: {deep.NewNormal(μ, σ), deep.NewUniform(μ, σ)} */
				Weight: deep.NewNormal(1.0, 0.0),
				/* Apply bias */
				Bias: true,
			})

			flog.Info("split train test: %v/%v", len(train), len(test))

			const iterations = 200
			// params: learning rate, momentum, alpha decay, nesterov
			optimizer := training.NewSGD(0.05, 0.1, 1e-6, true)

			// params: optimizer, verbosity
			trainer := training.NewTrainer(optimizer, iterations/10)
			trainer.Train(nn, train, test, iterations)

			flog.Info("first: %+v", train[:10])
			var (
				// confusionMatrix has actual values in the first index with
				// predicted values in the second.
				confusionMatrix [2][2]int
			)
			for i, v := range test {
				want := v.Response[0]
				got := nn.Predict(v.Input)[0]
				confusionMatrix[0][int(math.Round(want))]++
				confusionMatrix[1][int(math.Round(got))]++

				if i < 5 {
					flog.Info("Sample Prediction %v: %+v (got %v)", i, v, got)
				}
			}
			twr := tabwriter.NewWriter(os.Stderr, 0, 4, 3, ' ', 0)
			_, _ = fmt.Fprintf(twr, "-\tOff\tOn\n")
			_, _ = fmt.Fprintf(twr, "Actual\t%v\t%v\n", confusionMatrix[0][0], confusionMatrix[0][1])
			_, _ = fmt.Fprintf(twr, "Predicted\t%v\t%v\n", confusionMatrix[1][0], confusionMatrix[1][1])
			err = twr.Flush()
			return err
		},
	}
}
