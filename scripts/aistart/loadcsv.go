package main

import (
	"database/sql"
	"os"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/flog"
)

func connectDB() (*sql.DB, error) {
	const envKey = "POSTGRES_URL"
	url := os.Getenv(envKey)
	if url == "" {
		return nil, xerrors.Errorf("no $%v provided", envKey)
	}

	return sql.Open("postgres", url)
}

type trainingRow struct {
	WorkspaceID string `csv:"workspace_id"`
	// HourOfDay ranges from 0 to 23
	HourOfDay int `csv:"hour"`
	// Day of Week ranges from 0 to 6
	DayOfWeek      int `csv:"day"`
	HoursSinceUsed int `csv:"hours_since_used"`
	Used           int `csv:"used"`
}

func (t trainingRow) vectorizeHourOfDay() []float64 {
	var fs []float64
	for i := 0; i < 24; i++ {
		if i == t.HourOfDay {
			fs = append(fs, 1)
		} else {
			fs = append(fs, 0)
		}
	}
	return fs
}

func (t trainingRow) vectorize() vector {
	return [][]float64{
		append(
			[]float64{
				float64(t.HoursSinceUsed) / 61,
				float64(t.HourOfDay) / 23,
				float64(t.DayOfWeek),
			},
			t.vectorizeHourOfDay()...,
		),
		{float64(t.Used)},
	}
}

type dbRow struct {
	Time        time.Time
	WorkspaceID string
}

// generateTrainingRows accepts sparse input data from the DB and creates
// trainingRows suitable to enter a prediction model.
func generateTrainingRows(rs []dbRow) []trainingRow {
	// WorkspaceIDs maps workspaces to the time they were previously seen.
	// We first generate a map of IDs to zero so we can easily fill in
	// missing hours.
	workspaceIDs := make(map[string]time.Time)
	for _, r := range rs {
		workspaceIDs[r.WorkspaceID] = time.Time{}
	}

	var trainingRows []trainingRow

	last := rs[0].Time
	for _, r := range rs {
		if !r.Time.Equal(last) && !last.IsZero() {
			// We just skipped a time-slot, we must fill in the blanks.
			for {
				last = last.Add(time.Hour)
				if !last.Before(r.Time) {
					break
				}
				for wid := range workspaceIDs {
					var hoursSinceLastUsed int
					if !workspaceIDs[wid].IsZero() {
						hoursSinceLastUsed = int(last.Sub(workspaceIDs[wid]) / time.Hour)
					}
					trainingRows = append(trainingRows, trainingRow{
						WorkspaceID:    wid,
						HourOfDay:      last.Hour(),
						DayOfWeek:      int(last.Weekday()),
						HoursSinceUsed: hoursSinceLastUsed,
						Used:           0,
					})
				}
			}
		}
		workspaceLastSeen := workspaceIDs[r.WorkspaceID]
		workspaceIDs[r.WorkspaceID] = r.Time

		var hoursSinceLastSeen int
		if !workspaceLastSeen.IsZero() {
			hoursSinceLastSeen = int(r.Time.Sub(workspaceLastSeen) / time.Hour)
		}
		trainingRows = append(trainingRows, trainingRow{
			WorkspaceID:    r.WorkspaceID,
			HourOfDay:      r.Time.Hour(),
			DayOfWeek:      int(r.Time.Weekday()),
			HoursSinceUsed: hoursSinceLastSeen,
			Used:           1,
		})
	}

	return trainingRows
}

func loadTrainingCSV() *cobra.Command {
	return &cobra.Command{
		Use: "load-training-csv",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := connectDB()
			if err != nil {
				return err
			}
			const q = `
			SELECT
			date_trunc('hour', ag.created_at) at_hour,
			 workspace_id
		FROM
					agent_stats ag
		JOIN workspaces w ON
			w.id = ag.workspace_id
		WHERE
			NOT w.deleted AND w.id = '0170be1c-735f-4a69-8223-8ef86af56ef5'
		GROUP BY
			workspace_id,
			user_id,
			at_hour
		ORDER BY
			at_hour ASC;
		`
			rows, err := db.Query(q)
			if err != nil {
				return err
			}

			var rs []dbRow
			for rows.Next() {
				var r dbRow
				err = rows.Scan(&r.Time, &r.WorkspaceID)
				if err != nil {
					return err
				}
				rs = append(rs, r)
			}
			err = rows.Err()
			if err != nil {
				return err
			}

			flog.Info("loaded %v rows", len(rs))
			trainingRows := generateTrainingRows(rs)
			flog.Info("generated %v training rows", len(trainingRows))
			err = gocsv.Marshal(trainingRows, os.Stdout)
			return err
		},
	}
}
