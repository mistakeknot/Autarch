package main

import (
	"github.com/mistakeknot/autarch/internal/homefooter"
	"github.com/spf13/cobra"
)

func needsMkExportCmd() *cobra.Command {
	var db string
	var days int
	cmd := &cobra.Command{
		Use: "export", Short: "Export stored card picks by mk as read-only JSON Lines",
		Long: "Export historical card picks by mk, oldest first, over the last --days days (default 7). Reads Home read-only without moving a cursor, including soft-deleted cards. In WAL mode, SQLite may create -wal/-shm side files (the -wal can be empty); run export as the same OS user as Home. Unavailable round, auto_close and revision counts are null; kind is the stored ask kind.",
		Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if days < 1 || days > 365 {
				return &usageError{"--days must be between 1 and 365"}
			}
			if db == "" {
				var err error
				db, err = homefooter.DefaultDB()
				if err != nil {
					return &usageError{err.Error()}
				}
			}
			return homefooter.ExportPicks(cmd.OutOrStdout(), homefooter.ExportOptions{DB: db, Days: days})
		},
	}
	cmd.Flags().StringVar(&db, "db", "", "Home data.db (default: found from the environment)")
	cmd.Flags().IntVar(&days, "days", 7, "Window in days (1–365)")
	return cmd
}
