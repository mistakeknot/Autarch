package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/homefooter"
)

var callerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// footerCursorPath is the caller's cursor: $XDG_STATE_HOME/autarch/needs-mk-footer/<caller>.cursor, one file per caller
// so two threads never move each other's "since".
func footerCursorPath(caller string) (string, error) {
	if !callerName.MatchString(caller) {
		return "", &usageError{"--caller must be letters, digits, dot, dash or underscore"}
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "autarch", "needs-mk-footer", caller+".cursor"), nil
}

func needsMkFooterCmd() *cobra.Command {
	var db, caller, cursor string
	var peek bool
	cmd := &cobra.Command{
		Use:   "footer",
		Short: "Print what is open for mk, what is parked, new picks and owed obligations, read from Home",
		Long: `Reads Home's data.db read-only and prints four sections: To decide (open cards minus Later),
Parked (Later), New picks since the caller's cursor, and Undelivered obligations (not done, dismissed or voided).

Each run moves the caller's cursor to the time the run began, so the next run shows only newer picks. The caller is
--caller, else $BB_THREAD_ID, else "default". --peek prints the same footer and leaves the cursor alone.
The database is --db, else $AUTARCH_HOME_DB, else $HOME_BB_DATA/plugins/autarch/data.db.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if db == "" {
				var err error
				if db, err = homefooter.DefaultDB(); err != nil {
					return &usageError{err.Error()}
				}
			}
			if cursor == "" {
				if caller == "" {
					caller = os.Getenv("BB_THREAD_ID")
				}
				if caller == "" {
					caller = "default"
				}
				var err error
				if cursor, err = footerCursorPath(caller); err != nil {
					return err
				}
			}
			if err := homefooter.Print(cmd.OutOrStdout(), homefooter.Options{DB: db, Cursor: cursor, Peek: peek}); err != nil {
				return fmt.Errorf("footer: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&db, "db", "", "Home data.db (default: found from the environment)")
	cmd.Flags().StringVar(&caller, "caller", "", "Whose cursor to use (default: $BB_THREAD_ID, else \"default\")")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Cursor file, overriding --caller")
	cmd.Flags().BoolVar(&peek, "peek", false, "Leave the cursor where it is")
	return cmd
}
