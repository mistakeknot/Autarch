package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/pollard/api"
	"github.com/mistakeknot/autarch/internal/pollard/config"
	"github.com/mistakeknot/autarch/internal/pollard/watch"
	"github.com/mistakeknot/autarch/pkg/signals"
)

var watchOnce bool

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Run continuous competitor monitoring",
	Long: `Watch mode runs configured hunters on a schedule, diffs results
against previous scans, and emits signals for new findings.

Use --once for a single scan cycle (cron-friendly).`,
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().BoolVar(&watchOnce, "once", false, "Run a single watch cycle and exit")
	rootCmd.AddCommand(watchCmd)
}

func runWatch(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	scanner, err := api.NewScanner(cwd)
	if err != nil {
		return fmt.Errorf("creating scanner: %w", err)
	}

	watchCfg := watch.WatchConfig{
		Enabled:  cfg.Watch.Enabled,
		Interval: cfg.Watch.Interval,
		Hunters:  cfg.Watch.Hunters,
		NotifyOn: cfg.Watch.NotifyOn,
	}
	if watchCfg.Interval == "" {
		watchCfg.Interval = "24h"
	}

	publisher := signals.NewClient(signals.DefaultServerURL())
	w := watch.NewWatcher(cwd, scanner, cfg, watchCfg, publisher)

	ctx := context.Background()

	if watchOnce {
		result, err := w.RunOnce(ctx)
		if err != nil {
			return err
		}
		if result.IsFirst {
			fmt.Println("First watch scan completed — baseline established")
		}
		if result.Diff != nil {
			fmt.Println(result.Diff.Summary)
			if result.Diff.HasChanges() {
				for _, f := range result.Diff.NewFiles {
					fmt.Printf("  new: %s\n", f)
				}
			}
		}
		return nil
	}

	fmt.Printf("Starting watch mode (interval: %s)\n", watchCfg.Interval)
	fmt.Println("Press Ctrl+C to stop")
	return w.Run(ctx)
}
