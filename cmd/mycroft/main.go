package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
	"github.com/mistakeknot/autarch/internal/mycroft/patrol"
	"github.com/mistakeknot/autarch/internal/mycroft/scheduler"
	"github.com/mistakeknot/autarch/internal/mycroft/spawn"
	"github.com/mistakeknot/autarch/internal/mycroft/tier"
	"github.com/mistakeknot/autarch/pkg/fleet"
	"github.com/spf13/cobra"
)

const (
	defaultDataDir = ".autarch/mycroft"
	configFile     = "config.yaml"
	dbFile         = "decisions.db"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "mycroft",
	Short: "Fleet orchestrator — observe, rank, dispatch",
	Long: `Mycroft coordinates AI agent sessions with escalating autonomy:
  T0: Observe and shadow-suggest (no actions)
  T1: Suggest dispatches, user approves/rejects
  T2: Auto-dispatch low-risk work (v0.2)
  T3: Full autonomous dispatch (v0.2)`,
}

// dataDir returns the mycroft data directory, creating it if needed.
func dataDir() string {
	dir := defaultDataDir
	if d := os.Getenv("MYCROFT_DATA_DIR"); d != "" {
		dir = d
	}
	os.MkdirAll(dir, 0755)
	return dir
}

// loadConfig loads config from the data directory.
func loadConfig() (mycroft.Config, error) {
	return mycroft.LoadConfig(filepath.Join(dataDir(), configFile))
}

// openDB opens the decisions database from the data directory.
func openDB() (*scheduler.Dispatcher, func(), error) {
	db, err := mycroft.OpenDB(filepath.Join(dataDir(), dbFile))
	if err != nil {
		return nil, nil, fmt.Errorf("open decisions db: %w", err)
	}
	d := scheduler.NewDispatcher(db, nil, "demarch")
	cleanup := func() { db.Close() }
	return d, cleanup, nil
}

// newSource creates a data source — prefers AggregatorSource (pkg/fleet),
// falls back to PatrolSource (internal).
func newSource() mycroft.DataSource {
	if regPath := fleet.DiscoverRegistryPath(); regPath != "" {
		return fleet.NewAggregatorSource(regPath)
	}
	return patrol.NewPatrolSource("")
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the patrol loop",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		// Open decisions DB for dispatch logging.
		db, err := mycroft.OpenDB(filepath.Join(dataDir(), dbFile))
		if err != nil {
			return fmt.Errorf("open decisions db: %w", err)
		}
		defer db.Close()

		// Create spawner and orchestrator.
		spawner := spawn.NewClaudeCodeSpawner("", "Demarch")
		orch := scheduler.NewOrchestrator(db, spawner, cfg, "demarch")

		src := newSource()
		p := patrol.New(src, cfg, filepath.Join(dataDir(), "heartbeat"),
			patrol.WithOnCycle(func(v mycroft.FleetView) {
				fmt.Printf("[%s] patrol: %d agents, %d beads, tier: %s\n",
					time.Now().Format("15:04:05"),
					len(v.Agents), len(v.Work), cfg.Tier)
				orch.OnCycle(v)
			}),
		)

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		fmt.Printf("mycroft patrol starting (tier: %s)\n", cfg.Tier)
		return p.Run(ctx)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show fleet status and current tier",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return err
		}

		src := newSource()
		p := patrol.New(src, cfg, "")
		view := p.RunOnce(context.Background())

		fmt.Printf("Tier: %s\n\n", cfg.Tier)

		// Agents table.
		if len(view.Agents) == 0 {
			fmt.Println("Agents: none detected")
		} else {
			fmt.Println("Agents:")
			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "  NAME\tSTATUS\tRUNTIME\tCAPABILITIES")
			for _, a := range view.Agents {
				caps := "—"
				if len(a.Capabilities) > 0 {
					caps = strings.Join(a.Capabilities, ", ")
				}
				fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n",
					a.Name, a.Status, a.Runtime, caps)
			}
			tw.Flush()
		}

		// Work queue.
		fmt.Println()
		if len(view.Work) == 0 {
			fmt.Println("Work queue: empty")
		} else {
			fmt.Printf("Work queue: %d beads\n", len(view.Work))
			tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "  ID\tPRI\tTYPE\tCOMPLEXITY\tTITLE")
			for _, b := range view.Work {
				title := b.Title
				if len(title) > 50 {
					title = title[:47] + "..."
				}
				fmt.Fprintf(tw, "  %s\tP%d\t%s\t%s\t%s\n",
					b.ID, b.Priority, b.Type, b.Complexity, title)
			}
			tw.Flush()
		}

		// Conflicts.
		if len(view.Conflicts) > 0 {
			fmt.Printf("\nConflicts: %d\n", len(view.Conflicts))
			for _, c := range view.Conflicts {
				fmt.Printf("  %s — held by %s\n", c.File, strings.Join(c.Holders, ", "))
			}
		}

		// Freshness.
		fmt.Println()
		for source, ts := range view.Freshness {
			age := time.Since(ts).Truncate(time.Second)
			fmt.Printf("  %s: %s ago\n", source, age)
		}

		return nil
	},
}

var shadowsCmd = &cobra.Command{
	Use:   "shadows",
	Short: "Show shadow suggestion digest",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, cleanup, err := openDB()
		if err != nil {
			return err
		}
		defer cleanup()

		limit, _ := cmd.Flags().GetInt("limit")
		entries, err := d.ShadowDigest(limit)
		if err != nil {
			return fmt.Errorf("shadow digest: %w", err)
		}

		if len(entries) == 0 {
			fmt.Println("No shadow suggestions recorded yet.")
			fmt.Println("Run 'mycroft run' to start observing fleet activity.")
			return nil
		}

		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "TIME\tAGENT\tBEAD\tCONTEXT")
		for _, e := range entries {
			ts := time.Unix(e.Timestamp, 0).Format("01-02 15:04")
			ctx := e.Context
			if len(ctx) > 60 {
				ctx = ctx[:57] + "..."
			}
			if ctx == "" {
				ctx = "—"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", ts, e.Agent, e.Bead, ctx)
		}
		tw.Flush()

		return nil
	},
}

var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause dispatching (in-flight agents continue)",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, cleanup, err := openDB()
		if err != nil {
			return err
		}
		defer cleanup()

		if err := d.LogPause(); err != nil {
			return fmt.Errorf("log pause: %w", err)
		}
		fmt.Println("Dispatching paused. In-flight agents will continue.")
		fmt.Println("Use 'mycroft resume' to resume.")
		return nil
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume dispatching at current tier",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, cleanup, err := openDB()
		if err != nil {
			return err
		}
		defer cleanup()

		if err := d.LogResume(); err != nil {
			return fmt.Errorf("log resume: %w", err)
		}

		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		fmt.Printf("Dispatching resumed at %s.\n", cfg.Tier)
		return nil
	},
}

var overrideCmd = &cobra.Command{
	Use:   "override <bead> <agent>",
	Short: "Manually assign a bead to an agent",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		beadID, agent := args[0], args[1]

		d, cleanup, err := openDB()
		if err != nil {
			return err
		}
		defer cleanup()

		reason, _ := cmd.Flags().GetString("reason")
		if reason == "" {
			reason = "manual override via CLI"
		}
		if err := d.LogOverride(agent, beadID, reason); err != nil {
			return fmt.Errorf("log override: %w", err)
		}
		fmt.Printf("Override recorded: %s → %s\n", beadID, agent)
		return nil
	},
}

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Delete old dispatch log entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, cleanup, err := openDB()
		if err != nil {
			return err
		}
		defer cleanup()

		olderThan, _ := cmd.Flags().GetString("older-than")
		age, err := time.ParseDuration(olderThan)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", olderThan, err)
		}

		pruned, err := d.PruneOlderThan(age)
		if err != nil {
			return err
		}
		fmt.Printf("Pruned %d dispatch log entries older than %s.\n", pruned, age)
		return nil
	},
}

var promoteCmd = &cobra.Command{
	Use:   "promote",
	Short: "Promote to the next autonomy tier",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := mycroft.OpenDB(filepath.Join(dataDir(), dbFile))
		if err != nil {
			return fmt.Errorf("open decisions db: %w", err)
		}
		defer db.Close()

		fsm := tier.New(db, "demarch")
		current, err := fsm.Current()
		if err != nil {
			return err
		}

		reason, _ := cmd.Flags().GetString("reason")
		if reason == "" {
			reason = "manual promotion via CLI"
		}
		if err := fsm.Promote(tier.Evidence{Reason: reason}); err != nil {
			return err
		}
		fmt.Printf("Promoted: %s → %s\n", current, current+1)
		return nil
	},
}

var demoteCmd = &cobra.Command{
	Use:   "demote",
	Short: "Demote to the previous autonomy tier",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := mycroft.OpenDB(filepath.Join(dataDir(), dbFile))
		if err != nil {
			return fmt.Errorf("open decisions db: %w", err)
		}
		defer db.Close()

		fsm := tier.New(db, "demarch")
		current, err := fsm.Current()
		if err != nil {
			return err
		}

		reason, _ := cmd.Flags().GetString("reason")
		if reason == "" {
			reason = "manual demotion via CLI"
		}
		if err := fsm.Demote("manual", tier.Evidence{Reason: reason}); err != nil {
			return err
		}
		fmt.Printf("Demoted: %s → %s\n", current, current-1)
		return nil
	},
}

var tierCmd = &cobra.Command{
	Use:   "tier",
	Short: "Show current tier and recent transitions",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := mycroft.OpenDB(filepath.Join(dataDir(), dbFile))
		if err != nil {
			return fmt.Errorf("open decisions db: %w", err)
		}
		defer db.Close()

		fsm := tier.New(db, "demarch")
		current, err := fsm.Current()
		if err != nil {
			return err
		}
		fmt.Printf("Current tier: %s\n", current)

		transitions, err := fsm.Transitions(10)
		if err != nil {
			return err
		}

		if len(transitions) == 0 {
			fmt.Println("\nNo tier transitions recorded yet.")
			return nil
		}

		fmt.Println("\nRecent transitions:")
		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "  TIME\tFROM\tTO\tTRIGGER\tREASON")
		for _, t := range transitions {
			ts := time.Unix(t.Timestamp, 0).Format("01-02 15:04")
			reason := t.Evidence.Reason
			if len(reason) > 50 {
				reason = reason[:47] + "..."
			}
			if reason == "" {
				reason = "—"
			}
			fmt.Fprintf(tw, "  %s\tT%d\tT%d\t%s\t%s\n",
				ts, t.FromTier, t.ToTier, t.Trigger, reason)
		}
		tw.Flush()

		return nil
	},
}

func init() {
	pauseCmd.Flags().Bool("drain", false, "Also signal in-flight agents to checkpoint and stop")
	shadowsCmd.Flags().Int("limit", 20, "Maximum number of shadow suggestions to show")
	overrideCmd.Flags().String("reason", "", "Reason for the manual override")
	pruneCmd.Flags().String("older-than", "720h", "Delete entries older than this duration (default 30 days)")
	promoteCmd.Flags().String("reason", "", "Reason for the promotion")
	demoteCmd.Flags().String("reason", "", "Reason for the demotion")
	rootCmd.AddCommand(runCmd, statusCmd, shadowsCmd, pauseCmd, resumeCmd, overrideCmd, pruneCmd, promoteCmd, demoteCmd, tierCmd)
}
