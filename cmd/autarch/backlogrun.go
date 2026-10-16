package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/backlogrun"
)

// backlogRunCmd is the recurring backlog run (bead mk-omogp.2). Weekly: triage new or changed cards, cross-check the
// mk-gated ones against Home, message each coordinator. Daily: dispatch newly-ready cards only.
func backlogRunCmd() *cobra.Command {
	var mode, stateDir, homeDB, vizierTell string
	var dryRun, check bool
	cmd := &cobra.Command{
		Use:   "backlog-run",
		Short: "Triage the BB Tasks backlogs and message each coordinator (weekly or daily)",
		Long: `Snapshots every BB Tasks board's backlog and todo cards. The weekly run labels new or changed cards
(ready, needs-spec, mk-gated, close) with a cheap Clavain-routed seat, cross-checks mk-gated cards against Home's
card states (read-only), and sends each coordinator one message. The daily run only tells a coordinator about
newly-ready cards, and only when it has room and the pool has headroom. One message per run goes to the vizier.
Touch <state-dir>/PAUSE to stop all runs. --dry-run writes the digest and messages only the vizier.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			home, _ := os.UserHomeDir()
			if stateDir == "" {
				stateDir = filepath.Join(home, ".local", "state", "autarch", "backlog-run")
			}
			if homeDB == "" {
				homeDB = os.Getenv("AUTARCH_HOME_DB")
			}
			if vizierTell == "" {
				vizierTell = filepath.Join(home, ".local", "bin", "vizier-tell")
			}
			root := os.Getenv("CLAVAIN_SELECTED_ROOT")
			policy := os.Getenv("CLAVAIN_ROUTING_POLICY")
			if policy == "" && root != "" {
				policy = filepath.Join(root, "config", "routing.yaml")
			}
			// --check changes nothing under the state directory: its scratch space is a temporary directory.
			work := filepath.Join(stateDir, "work")
			if check {
				tmp, err := os.MkdirTemp("", "backlog-run-check-")
				if err != nil {
					return err
				}
				defer os.RemoveAll(tmp)
				work = tmp
			} else if err := os.MkdirAll(work, 0o700); err != nil {
				return err
			}
			bb := backlogrun.BB{Run: backlogrun.ExecRunner, VizierTell: vizierTell, TempDir: work}
			router := backlogrun.Router{Run: backlogrun.ExecRunner, Policy: policy, Dispatch: filepath.Join(root, "scripts", "dispatch.sh"), WorkDir: work, Repo: filepath.Join(work, "empty")}
			if check {
				return backlogCheck(ctx, stateDir, homeDB, vizierTell, root, policy, bb, router)
			}
			lock, err := os.OpenFile(filepath.Join(stateDir, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
			if err != nil {
				return err
			}
			defer lock.Close()
			if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
				return fmt.Errorf("another backlog run holds %s", lock.Name())
			}
			d := backlogrun.Deps{Board: bb, Triage: router, Home: backlogrun.Home{DB: homeDB}, Notify: bb, Pool: bb, State: backlogrun.State{Dir: stateDir}}
			res, err := backlogrun.Run(ctx, d, backlogrun.Options{Mode: backlogrun.Mode(mode), DryRun: dryRun})
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s %s\n", res.Receipt.Mode, res.Receipt.Outcome, res.Receipt.Detail)
			if res.Digest != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "digest: %s\n", res.Digest)
			}
			return err
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "weekly", "weekly or daily")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "write the digest and message only the vizier")
	cmd.Flags().BoolVar(&check, "check", false, "verify tools, routing and Home access; change nothing and message no one")
	cmd.Flags().StringVar(&stateDir, "state-dir", "", "state directory (default ~/.local/state/autarch/backlog-run)")
	cmd.Flags().StringVar(&homeDB, "home-db", "", "Home data.db, opened read-only (default $AUTARCH_HOME_DB)")
	cmd.Flags().StringVar(&vizierTell, "vizier-tell", "", "path of vizier-tell (default ~/.local/bin/vizier-tell)")
	return cmd
}

func backlogCheck(ctx context.Context, stateDir, homeDB, vizierTell, root, policy string, bb backlogrun.BB, r backlogrun.Router) error {
	var bad []string
	note := func(ok bool, what string, err error) {
		if ok {
			fmt.Printf("ok    %s\n", what)
			return
		}
		fmt.Printf("FAIL  %s: %v\n", what, err)
		bad = append(bad, what)
	}
	_, err := os.Stat(vizierTell)
	note(err == nil, "vizier-tell present", err)
	_, err = os.Stat(r.Dispatch)
	note(err == nil && root != "", "dispatch.sh present (CLAVAIN_SELECTED_ROOT)", err)
	note(policy != "", "routing policy set", fmt.Errorf("set CLAVAIN_ROUTING_POLICY or CLAVAIN_SELECTED_ROOT"))
	if _, err := os.Stat(filepath.Join(stateDir, "PAUSE")); err == nil {
		fmt.Printf("note  PAUSE file present: runs would do nothing\n")
	}
	_, _, err = bb.Headroom(ctx)
	note(err == nil, "bb pool status readable", err)
	cards, err := bb.Cards(ctx)
	note(err == nil, fmt.Sprintf("board snapshot (%d cards)", len(cards)), err)
	_, err = backlogrun.Home{DB: homeDB}.States(ctx, []string{"AUTA-1"})
	note(err == nil && homeDB != "", "Home data.db readable read-only", fmt.Errorf("%v (path %q)", err, homeDB))
	if policy != "" && root != "" {
		_, err = r.Seat(ctx)
		note(err == nil, "triage seat resolves through routing", err)
		err = r.RequireNoTools()
		note(err == nil, "dispatch wrapper implements --no-tools", err)
	}
	if len(bad) > 0 {
		return fmt.Errorf("check failed: %d item(s)", len(bad))
	}
	return nil
}
