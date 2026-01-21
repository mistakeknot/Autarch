package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
	"github.com/spf13/cobra"
)

func LockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lock",
		Short: "Manage file reservations",
	}
	cmd.AddCommand(lockReserveCmd(), lockReleaseCmd(), lockRenewCmd(), lockForceReleaseCmd())
	return cmd
}

func lockReserveCmd() *cobra.Command {
	var owner string
	var reason string
	var ttl string
	var exclusive bool
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "reserve",
		Short: "Reserve file paths",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("lock reserve", err)
				}
			}()
			if strings.TrimSpace(owner) == "" {
				return fmt.Errorf("owner required")
			}
			if len(args) == 0 {
				return fmt.Errorf("paths required")
			}
			dur := time.Hour
			if strings.TrimSpace(ttl) != "" {
				parsed, err := time.ParseDuration(ttl)
				if err != nil {
					return err
				}
				dur = parsed
			}

			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			res, err := storage.ReservePaths(db, owner, args, exclusive, reason, dur)
			if err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"granted":   res.Granted,
					"conflicts": res.Conflicts,
				}
				return writeJSON(cmd, payload)
			}
			for _, g := range res.Granted {
				fmt.Fprintf(cmd.OutOrStdout(), "granted %s owner=%s\n", g.Path, g.Owner)
			}
			for _, c := range res.Conflicts {
				fmt.Fprintf(cmd.OutOrStdout(), "conflict %s holder=%s\n", c.Path, c.Holder)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&owner, "owner", "", "Reservation owner")
	cmd.Flags().BoolVar(&exclusive, "exclusive", false, "Exclusive reservation")
	cmd.Flags().StringVar(&reason, "reason", "", "Reservation reason")
	cmd.Flags().StringVar(&ttl, "ttl", "1h", "Reservation TTL (duration)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func lockReleaseCmd() *cobra.Command {
	var owner string
	var ids []int64
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "release",
		Short: "Release reserved paths",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("lock release", err)
				}
			}()
			if strings.TrimSpace(owner) == "" {
				return fmt.Errorf("owner required")
			}
			if len(ids) == 0 && len(args) == 0 {
				return fmt.Errorf("paths or ids required")
			}
			if len(ids) > 0 && len(args) > 0 {
				return fmt.Errorf("use ids or paths, not both")
			}
			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			var released int
			if len(ids) > 0 {
				released, err = storage.ReleaseReservationsByID(db, owner, ids)
			} else {
				released, err = storage.ReleasePaths(db, owner, args)
			}
			if err != nil {
				return err
			}
			if jsonOut {
				payload := map[string]interface{}{
					"released": released,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "released %d\n", released)
			return nil
		},
	}

	cmd.Flags().StringVar(&owner, "owner", "", "Reservation owner")
	cmd.Flags().Int64SliceVar(&ids, "id", nil, "Reservation id to release")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func lockRenewCmd() *cobra.Command {
	var owner string
	var extend string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "renew",
		Short: "Extend reservation expiry",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("lock renew", err)
				}
			}()
			if strings.TrimSpace(owner) == "" {
				return fmt.Errorf("owner required")
			}
			dur := time.Hour
			if strings.TrimSpace(extend) != "" {
				parsed, err := time.ParseDuration(extend)
				if err != nil {
					return err
				}
				dur = parsed
			}

			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			renewals, err := storage.RenewReservations(db, owner, args, dur)
			if err != nil {
				return err
			}

			if jsonOut {
				payload := struct {
					Renewed      int `json:"renewed"`
					Reservations []struct {
						ID        int64  `json:"id"`
						Path      string `json:"path"`
						OldExpiry string `json:"old_expires_ts"`
						NewExpiry string `json:"new_expires_ts"`
					} `json:"reservations"`
				}{Renewed: len(renewals), Reservations: make([]struct {
					ID        int64  `json:"id"`
					Path      string `json:"path"`
					OldExpiry string `json:"old_expires_ts"`
					NewExpiry string `json:"new_expires_ts"`
				}, 0, len(renewals))}
				for _, r := range renewals {
					payload.Reservations = append(payload.Reservations, struct {
						ID        int64  `json:"id"`
						Path      string `json:"path"`
						OldExpiry string `json:"old_expires_ts"`
						NewExpiry string `json:"new_expires_ts"`
					}{
						ID:        r.ID,
						Path:      r.Path,
						OldExpiry: r.OldExpiresAt,
						NewExpiry: r.NewExpiresAt,
					})
				}
				return writeJSON(cmd, payload)
			}
			for _, r := range renewals {
				fmt.Fprintf(cmd.OutOrStdout(), "renewed %s old=%s new=%s\n", r.Path, r.OldExpiresAt, r.NewExpiresAt)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "renewed %d\n", len(renewals))
			return nil
		},
	}

	cmd.Flags().StringVar(&owner, "owner", "", "Reservation owner")
	cmd.Flags().StringVar(&extend, "extend", "1h", "Extend expiry by duration")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}

func lockForceReleaseCmd() *cobra.Command {
	var id int64
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "force-release",
		Short: "Force release a reservation",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer func() {
				if err != nil {
					err = wrapCommandError("lock force-release", err)
				}
			}()
			if id <= 0 {
				return fmt.Errorf("reservation id required")
			}

			db, closeDB, err := openStateDB()
			if err != nil {
				return err
			}
			defer closeDB()

			if err := storage.ForceReleaseReservation(db, id); err != nil {
				return err
			}

			if jsonOut {
				payload := map[string]interface{}{
					"id":       id,
					"released": true,
				}
				return writeJSON(cmd, payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "released %d\n", id)
			return nil
		},
	}

	cmd.Flags().Int64Var(&id, "id", 0, "Reservation id")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON output")
	return cmd
}
