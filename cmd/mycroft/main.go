package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mistakeknot/autarch/internal/mycroft"
	"github.com/mistakeknot/autarch/internal/mycroft/predispatch"
	"github.com/mistakeknot/autarch/internal/mycroft/spawn"
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

var rootCmd = &cobra.Command{Use: "mycroft", Short: "Start routed bb workers and run pre-dispatch checks"}

// dataDir returns the mycroft data directory without creating it.
func dataDir() string {
	dir := defaultDataDir
	if d := os.Getenv("MYCROFT_DATA_DIR"); d != "" {
		dir = d
	}
	return dir
}

// loadConfig loads config from the data directory.
func loadConfig() (mycroft.Config, error) {
	return mycroft.LoadConfig(filepath.Join(dataDir(), configFile))
}

func openDB() (*mycroft.ApprovalLog, func(), error) {
	if err := os.MkdirAll(dataDir(), 0755); err != nil {
		return nil, nil, fmt.Errorf("create mycroft data directory: %w", err)
	}
	db, err := mycroft.OpenDB(filepath.Join(dataDir(), dbFile))
	if err != nil {
		return nil, nil, fmt.Errorf("open decisions db: %w", err)
	}
	return mycroft.NewApprovalLog(db, "demarch"), func() { db.Close() }, nil
}

// newSpawner builds the bb-thread spawner from AUTARCH_BB_PROJECT and AUTARCH_BB_ENVIRONMENT.
func newSpawner() *spawn.BBSpawner {
	return spawn.NewBBSpawner(os.Getenv("AUTARCH_BB_PROJECT"), os.Getenv("AUTARCH_BB_ENVIRONMENT"), filepath.Join(dataDir(), "spawn-receipts.jsonl"))
}

var (
	spawnCard    string
	spawnTitle   string
	spawnApprove bool
)

// spawnCmd starts a coordinator-selected worker: without --approve it shows the plan and starts nothing.
var spawnCmd = &cobra.Command{
	Use:   "spawn <bead>",
	Short: "Show, or with --approve start, a bb-thread worker for a bead",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if spawnCard == "" {
			return fmt.Errorf("--card is required: a worker is attached to its card")
		}
		sp := newSpawner()
		bead := mycroft.BeadView{ID: args[0], Title: spawnTitle, Card: spawnCard}
		if !spawnApprove {
			if err := showPlan(sp, bead); err != nil {
				return err
			}
			fmt.Println("not started (pass --approve to start it)")
			return nil
		}
		// Open the decisions database before anything else: a worker is not started when the log cannot be opened.
		disp, closeDB, derr := openDB()
		if derr != nil {
			return fmt.Errorf("not started, dispatch log unavailable: %w", derr)
		}
		defer closeDB()
		return approveSpawn(disp, sp, bead)
	},
}

// workerSpawner is the part of the bb-thread spawner that an approval needs.
type workerSpawner interface {
	Plan(bead mycroft.BeadView) (string, spawn.Seat, string, error)
	Spawn(agentName string, bead mycroft.BeadView, contextFile string) (string, error)
	Detail(sessionID string) string
}

// showPlan prints the thread title and the seat routing resolved; it starts nothing.
func showPlan(sp workerSpawner, bead mycroft.BeadView) error {
	title, seat, _, err := sp.Plan(bead)
	if err != nil {
		return fmt.Errorf("no dispatch: %w", err)
	}
	fmt.Printf("thread: %s\nseat: role=%s model=%s effort=%s policy=%s\n", title, seat.Role, seat.Model, seat.Effort, seat.PolicyHash)
	return nil
}

// approveSpawn shows the plan, starts the worker and records the approval in dispatch_log. A plan that cannot be
// resolved and a failed start are logged as failures.
func approveSpawn(disp *mycroft.ApprovalLog, sp workerSpawner, bead mycroft.BeadView) error {
	if err := showPlan(sp, bead); err != nil {
		if lerr := disp.LogApprovalWith("mycroft", bead.ID, mycroft.OutcomeFailure, err.Error()); lerr != nil {
			return fmt.Errorf("%w (and the failure could not be logged: %v)", err, lerr)
		}
		return err
	}
	id, err := sp.Spawn("mycroft", bead, "")
	if err != nil {
		if lerr := disp.LogApprovalWith("mycroft", bead.ID, mycroft.OutcomeFailure, err.Error()); lerr != nil {
			return fmt.Errorf("%w (and the failure could not be logged: %v)", err, lerr)
		}
		return err
	}
	fmt.Printf("started thread %s\n", id)
	if lerr := disp.LogApprovalWith("mycroft", bead.ID, mycroft.OutcomeAccepted, sp.Detail(id)); lerr != nil {
		return fmt.Errorf("thread %s is running but the dispatch log write failed: %w", id, lerr)
	}
	return nil
}

var checkBead, checkAgent string
var preDispatchCmd = &cobra.Command{
	SilenceUsage:  true,
	SilenceErrors: true,
	Use:           "pre-dispatch",
	Short:         "Run the configured check for a proposed assignment",
	Long:          "Run pre_dispatch_check from the Mycroft config in the current working directory. No worker is started. An absent or empty command succeeds; a failed check returns a nonzero exit.",
	Args:          cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return predispatch.Run(ctx, cfg.PreDispatchCheck, checkBead, checkAgent)
	},
}

func init() {
	spawnCmd.Flags().StringVar(&spawnCard, "card", "", "task-board card key to attach")
	spawnCmd.Flags().StringVar(&spawnTitle, "title", "", "bead title for the thread title and brief")
	spawnCmd.Flags().BoolVar(&spawnApprove, "approve", false, "start the thread; without it nothing starts")
	preDispatchCmd.Flags().StringVar(&checkBead, "bead", "", "proposed bead ID (MYCROFT_BEAD_ID)")
	preDispatchCmd.Flags().StringVar(&checkAgent, "agent", "", "proposed agent name (MYCROFT_AGENT)")
	rootCmd.AddCommand(spawnCmd, preDispatchCmd)
}
