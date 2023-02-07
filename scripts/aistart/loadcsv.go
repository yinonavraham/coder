package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/coder/flog"
	"github.com/spf13/cobra"
)

func connectDB() (*sql.DB, error) {
	const envKey = "POSTGRES_URL"
	url := os.Getenv(envKey)
	if url == "" {
		return nil, fmt.Errorf("no $%v provided", envKey)
	}

	return sql.Open("postgres", url)
}

func loadTrainingCSV() *cobra.Command {
	return &cobra.Command{
		Use: "load-training-csv",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := connectDB()
			if err != nil {
				return err
			}
			const q = `SELECT
			date_trunc('hour', created_at) at_hour
		FROM
			agent_stats
		GROUP BY
			workspace_id, at_hour;
		`
			rows, err := db.Query(q)
			if err != nil {
				return err
			}

			var times []time.Time
			for rows.Next() {
				var t time.Time
				err = rows.Scan(&t)
				if err != nil {
					return nil
				}
				times = append(times, t)
			}
			flog.Info("times: %+v", times)
			return nil
		},
	}
}
