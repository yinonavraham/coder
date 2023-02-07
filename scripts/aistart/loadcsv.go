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
	DayOfWeek int `csv:"day"`
	Used      int `csv:"used"`
}

func (t trainingRow) vectorize() vector {
	return [][]float64{
		{float64(t.HourOfDay) / 23, float64(t.DayOfWeek / 6)},
		{float64(t.Used)},
	}
}

type dbRow struct {
	Time        time.Time
	WorkspaceID string
}

func (db dbRow) convert(used int) trainingRow {
	return trainingRow{
		WorkspaceID: db.WorkspaceID,
		HourOfDay:   db.Time.Hour(),
		DayOfWeek:   int(db.Time.Weekday()),
		Used:        used,
	}
}

// generateTrainingRows accepts sparse input data from the DB and creates
// trainingRows suitable to enter a prediction model.
func generateTrainingRows(rs []dbRow) []trainingRow {
	workspaceIDs := make(map[string]struct{})
	for _, r := range rs {
		workspaceIDs[r.WorkspaceID] = struct{}{}
	}

	var trainingRows []trainingRow

	last := rs[0].Time
	for _, r := range rs {
		if !r.Time.Equal(last) && !last.IsZero() {
			// We just skipped a time-slot, we must fill in the blanks.
			for last.Before(r.Time) {
				last = last.Add(time.Hour)
				for wid := range workspaceIDs {
					trainingRows = append(trainingRows, trainingRow{
						WorkspaceID: wid,
						HourOfDay:   last.Hour(),
						DayOfWeek:   int(last.Weekday()),
						Used:        0,
					})
				}
			}
		}
		trainingRows = append(trainingRows, r.convert(1))
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
			NOT w.deleted
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
