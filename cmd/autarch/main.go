package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/autarch/local"
	"github.com/mistakeknot/autarch/internal/autarch/setup"
	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/internal/bigend/daemon"
	"github.com/mistakeknot/autarch/internal/bigend/discovery"
	bigendTui "github.com/mistakeknot/autarch/internal/bigend/tui"
	"github.com/mistakeknot/autarch/internal/bigend/web"
	coldwineCli "github.com/mistakeknot/autarch/internal/coldwine/cli"
	coldwineConfig "github.com/mistakeknot/autarch/internal/coldwine/config"
	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	gurgehCli "github.com/mistakeknot/autarch/internal/gurgeh/cli"
	internalIntermute "github.com/mistakeknot/autarch/internal/intermute"
	"github.com/mistakeknot/autarch/internal/planstatus"
	pollardCli "github.com/mistakeknot/autarch/internal/pollard/cli"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/internal/tui/views"
	"github.com/mistakeknot/autarch/pkg/autarch"
	"github.com/mistakeknot/autarch/pkg/clavain"
	"github.com/mistakeknot/autarch/pkg/events"
	"github.com/mistakeknot/autarch/pkg/intercore"
	"github.com/mistakeknot/autarch/pkg/intermute"
	"github.com/mistakeknot/autarch/pkg/signals"
	"github.com/mistakeknot/autarch/pkg/timeout"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

func main() {
	root := &cobra.Command{
		Use:   "autarch",
		Short: "Unified AI agent development tools",
		Long: `Autarch - Unified monorepo for AI agent development tools.

Available tools:
  bigend    Multi-project agent mission control (web + TUI)
  gurgeh    TUI-first PRD generation and validation
  coldwine  Task orchestration for human-AI collaboration
  pollard   General-purpose research intelligence`,
	}

	root.AddCommand(tuiCmd())
	root.AddCommand(bigendCmd())
	root.AddCommand(gurgehCmd())
	root.AddCommand(coldwineCmd())
	root.AddCommand(pollardCmd())
	root.AddCommand(setupCmd())
	root.AddCommand(migrateCmd())
	root.AddCommand(reconcileCmd())
	root.AddCommand(eventsCmd())
	root.AddCommand(planstatus.NewCommand())
	root.AddCommand(statusCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func tuiCmd() *cobra.Command {
	var (
		port        int
		dataDir     string
		skipOnboard bool
		inlineMode  bool
		toolFlag    string
	)

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch unified TUI with Intermute backend",
		Long: `Launch the unified Autarch TUI with all tools accessible via tabs.

The TUI connects to an existing Intermute server if one is running,
or starts a standalone server automatically. All domain data (specs,
epics, tasks, insights, sessions) is stored in a local SQLite database.

New users start with the onboarding flow to create their first project.
Use --skip-onboard to go directly to the dashboard.

Navigation:
  /big, /gur, etc  Switch between tabs (Bigend, Gurgeh, Coldwine, Pollard)
  Ctrl+Left/Right   Cycle tabs
  Ctrl+P         Open command palette
  /bigend, etc.  Switch to tool by name (/big, /gur, /cold, /pol)
  ?              Show help
  Ctrl+C         Quit`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Auto-setup on first run
			if setup.NeedsSetup() {
				fmt.Println("First run detected. Setting up Autarch...")
				if err := setup.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: setup incomplete: %v\n", err)
				} else {
					fmt.Println("Setup complete!")
				}
			}

			// Resolve data directory
			if dataDir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				dataDir = filepath.Join(home, ".autarch")
			}

			// Create Intermute manager
			mgr, err := internalIntermute.NewManager(internalIntermute.Config{
				Port:    port,
				DataDir: dataDir,
			})
			if err != nil {
				return fmt.Errorf("failed to create intermute manager: %w", err)
			}

			// Create client connecting to Intermute server
			client := autarch.NewClient(mgr.URL())

			// Configure local file fallback for when Intermute is unreachable.
			// Uses CWD as project root (same as TUI's project discovery).
			if projectPath, err := os.Getwd(); err == nil {
				client.WithFallback(local.NewLocalSource(projectPath))
			}

			// Create unified app (serves both onboarding and skip-onboard paths)
			app := tui.NewUnifiedApp(client)
			app.SetIntermuteManager(mgr)

			if skipOnboard {
				app.SetSkipOnboarding(true)
				fmt.Fprintln(os.Stderr, "Warning: --skip-onboard is deprecated. Use --tool=gurgeh or omit the flag.")
			}

			// Build GurgehConfig with all onboarding view factories
			intermuteURL := mgr.URL()
			researchCoord := research.NewCoordinator(nil)
			gurgehCfg := &tui.GurgehConfig{
				ResearchCoord: researchCoord,
				CreateKickoffView: func() tui.View {
					v := views.NewKickoffView()
					v.SetProjectStartCallback(func(project *views.Project) tea.Cmd {
						return func() tea.Msg {
							return tui.ProjectCreatedMsg{
								ProjectID:   project.ID,
								ProjectName: project.Name,
								Description: project.Description,
								ScanResult:  project.ScanResult,
							}
						}
					})
					v.SetScanCodebaseCallback(func(path string) tea.Cmd {
						return func() tea.Msg {
							return tui.ScanCodebaseMsg{Path: path}
						}
					})
					return v
				},
				CreateSpecSummaryView: func(spec *tui.SpecSummary, coord *research.Coordinator) tui.View {
					return views.NewSpecSummaryView(spec, coord)
				},
				CreateEpicReviewView: func(proposals []epics.EpicProposal) tui.View {
					v := views.NewEpicReviewView(proposals)
					v.SetCallbacks(
						func(accepted []epics.EpicProposal) tea.Cmd {
							return func() tea.Msg {
								return tui.EpicsAcceptedMsg{Epics: accepted}
							}
						},
						nil, // regenerate callback
						func() tea.Cmd {
							return func() tea.Msg {
								return tui.NavigateBackMsg{}
							}
						},
					)
					return v
				},
				CreateTaskReviewView: func(taskList []tasks.TaskProposal) tui.View {
					v := views.NewTaskReviewView(taskList)
					v.SetAcceptCallback(func(accepted []tasks.TaskProposal) tea.Cmd {
						return func() tea.Msg {
							return tui.TasksAcceptedMsg{Tasks: accepted}
						}
					})
					v.SetBackCallback(func() tea.Cmd {
						return func() tea.Msg {
							return tui.NavigateBackMsg{}
						}
					})
					return v
				},
				CreateTaskDetailView: func(task tasks.TaskProposal, coord *research.Coordinator) tui.View {
					v := views.NewTaskDetailView(task, coord)
					v.SetCallbacks(
						func(t tasks.TaskProposal, agent views.AgentType, worktree bool) tea.Cmd {
							return func() tea.Msg {
								return tui.StartAgentMsg{
									Task:     t,
									Agent:    string(agent),
									Worktree: worktree,
								}
							}
						},
						func() tea.Cmd {
							return func() tea.Msg {
								return tui.NavigateBackMsg{}
							}
						},
					)
					return v
				},
				CreateSprintView: func(projectPath string) tui.View {
					v := views.NewSprintView(projectPath, views.SprintViewOpts{
						IntermuteURL: intermuteURL,
					})
					v.SetCallbacks(func() tea.Cmd {
						return func() tea.Msg { return tui.NavigateBackMsg{} }
					})
					return v
				},
			}

			// Create Intercore client (optional — nil if ic unavailable).
			iclient, _ := intercore.New()
			cclient, _ := clavain.New() // nil when clavain-cli absent — falls back to direct ic
			app.SetKernelAvailable(iclient != nil)

			// Start dispatch watcher if Intercore is available.
			if iclient != nil {
				app.SetDispatchWatcher(tui.NewDispatchWatcher(iclient, 5*time.Second))
			}

			// Wire signal broker and event watcher for Intercore → signals overlay.
			signalBroker := signals.NewBroker()
			app.SetSignalBroker(signalBroker)
			if iclient != nil {
				app.SetEventWatcher(tui.NewEventWatcher(iclient, signalBroker))
			}

			// Create scanner for Bigend project discovery
			cwd, _ := os.Getwd()
			scanRoots := []string{cwd}
			if home, err := os.UserHomeDir(); err == nil {
				scanRoots = append(scanRoots, filepath.Join(home, "projects"))
			}
			scanner := discovery.NewScanner(config.DiscoveryConfig{
				ScanRoots:       scanRoots,
				ExcludePatterns: []string{"node_modules", ".git", "vendor", "target"},
			})

			// Wire dashboard factory (GurgehConfig flows into GurgehView)
			app.SetDashboardViewFactory(func(c *autarch.Client) []tui.View {
				bigend := views.NewBigendView(c)
				bigend.SetIntercore(iclient)
				bigend.SetScanner(scanner)

				// Parse layout mode from coldwine config
				var coldwineOpts []views.ColdwineOpt
				if cwCfg, err := coldwineConfig.LoadFromProject("."); err == nil {
					switch cwCfg.TUI.LayoutMode {
					case "inline":
						coldwineOpts = append(coldwineOpts, views.WithLayoutMode(views.LayoutInline))
					case "split":
						coldwineOpts = append(coldwineOpts, views.WithLayoutMode(views.LayoutSplit))
					}
				}
				coldwine := views.NewColdwineView(c, coldwineOpts...)
				coldwine.SetClavain(cclient)
				coldwine.SetIntercore(iclient)
				// Sprint tab removed — merged into Coldwine as a mode toggle
				// Skip onboarding if specs already exist from a prior session
				gcfg := gurgehCfg
				if specs, err := c.ListSpecs(""); err == nil && len(specs) > 0 {
					gcfg = nil
				}
				return []tui.View{
					bigend,
					views.NewGurgehView(c, gcfg),
					coldwine,
					views.NewPollardView(c, researchCoord),
				}
			})

			return tui.Run(client, app, tui.RunOpts{
				InlineMode:    inlineMode,
				InitialTool:   toolFlag,
				ResearchCoord: researchCoord,
			})
		},
	}

	cmd.Flags().IntVar(&port, "port", 7338, "Intermute server port")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory (default: ~/.autarch)")
	cmd.Flags().BoolVar(&skipOnboard, "skip-onboard", false, "Skip onboarding and go directly to dashboard")
	cmd.Flags().BoolVar(&inlineMode, "inline", false, "Enable inline mode with log pane (preserves scrollback)")
	cmd.Flags().StringVar(&toolFlag, "tool", "", "Jump directly to a tool tab (bigend, gurgeh, coldwine, pollard)")

	return cmd
}

func bigendCmd() *cobra.Command {
	var (
		port       int
		host       string
		scanRoot   string
		cfgPath    string
		tuiMode    bool
		daemonMode bool
		daemonAddr string
	)

	cmd := &cobra.Command{
		Use:     "bigend",
		Aliases: []string{"vauxhall"},
		Short:   "Multi-project agent mission control",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Setup logging — TUI mode routes through LogHandler to log pane.
			// TODO(bigend-deprecation): Remove this block when the bigend --tui path is deleted.
			// Duplicates the logging setup in cmd/bigend/main.go intentionally (deprecated code
			// does not justify a new abstraction).
			var logHandler *pkgtui.LogHandler
			if tuiMode {
				logHandler = pkgtui.NewLogHandler(slog.LevelDebug)
				slog.SetDefault(slog.New(logHandler))
			} else {
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
					Level: slog.LevelInfo,
				})))
			}

			registerCtx, cancelRegister := context.WithTimeout(context.Background(), timeout.HTTPDefault)
			defer cancelRegister()
			if stop, err := intermute.RegisterTool(registerCtx, "bigend"); err != nil {
				slog.Warn("intermute registration failed", "error", err)
			} else if stop != nil {
				defer stop()
			}

			// Load config
			cfg, err := config.Load(cfgPath)
			if err != nil {
				slog.Error("failed to load config", "error", err)
				return err
			}

			// Override with flags
			if port != 8099 {
				cfg.Server.Port = port
			}
			if host != "0.0.0.0" {
				cfg.Server.Host = host
			}
			if scanRoot != "" {
				cfg.Discovery.ScanRoots = []string{scanRoot}
			}

			scanner := discovery.NewScanner(cfg.Discovery)

			// Open events store for signal persistence (default path).
			// On failure, signals still flow via broker — only persistence is lost.
			evStore, err := events.OpenStore("")
			if err != nil {
				slog.Warn("signal persistence disabled — events store unavailable", "error", err)
				evStore = nil
			}
			agg := aggregator.New(scanner, cfg, evStore)

			if !tuiMode {
				slog.Info("scanning for projects", "roots", cfg.Discovery.ScanRoots)
			}
			refreshCtx, cancelRefresh := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancelRefresh()
			if err := agg.Refresh(refreshCtx); err != nil {
				slog.Error("initial scan failed", "error", err)
			}

			if daemonMode {
				return runBigendDaemon(daemonAddr, cfg.Discovery.ScanRoots)
			} else if tuiMode {
				return runBigendTUI(agg, logHandler)
			}
			return runBigendWeb(cfg, agg)
		},
	}

	cmd.Flags().IntVar(&port, "port", 8099, "HTTP server port")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "HTTP server bind address")
	cmd.Flags().StringVar(&scanRoot, "scan-root", "", "Root directory to scan for projects")
	cmd.Flags().StringVar(&cfgPath, "config", "", "Path to config file")
	cmd.Flags().BoolVar(&tuiMode, "tui", false, "Run in TUI mode instead of web server")
	cmd.Flags().BoolVar(&daemonMode, "daemon", false, "Run as daemon with HTTP API")
	cmd.Flags().StringVar(&daemonAddr, "daemon-addr", "127.0.0.1:8100", "Daemon HTTP API address")

	return cmd
}

func runBigendDaemon(addr string, scanRoots []string) error {
	srv := daemon.NewServer(daemon.Config{
		Addr:        addr,
		ProjectDirs: scanRoots,
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		slog.Info("shutting down daemon")
		ctx, cancel := context.WithTimeout(context.Background(), timeout.Shutdown)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.Start(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func runBigendTUI(agg *aggregator.Aggregator, logHandler *pkgtui.LogHandler) error {
	defer pkgtui.RestoreTerminalOnPanic()
	// Deprecation warning for standalone TUI
	fmt.Fprintln(os.Stderr, "\033[33m⚠ Deprecation warning: bigend --tui is deprecated.\033[0m")
	fmt.Fprintln(os.Stderr, "  Use: autarch tui --tool=bigend")
	fmt.Fprintln(os.Stderr, "  Web server mode (bigend without --tui) remains available.")
	fmt.Fprintln(os.Stderr)

	m := bigendTui.New(agg, buildInfoString())
	p := tea.NewProgram(m, tea.WithAltScreen())

	if logHandler != nil {
		logHandler.SetProgram(p)
		defer logHandler.Close()
	}

	_, err := p.Run()
	return err
}

func buildInfoString() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		var rev, ts, modified string
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				rev = setting.Value
			case "vcs.time":
				ts = setting.Value
			case "vcs.modified":
				modified = setting.Value
			}
		}
		if rev != "" {
			short := rev
			if len(short) > 7 {
				short = short[:7]
			}
			stamp := short
			if ts != "" {
				if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
					stamp = stamp + " " + parsed.Format("2006-01-02 15:04")
				}
			}
			if modified == "true" {
				stamp = stamp + "*"
			}
			return "build " + strings.TrimSpace(stamp)
		}
	}
	return ""
}

func runBigendWeb(cfg *config.Config, agg *aggregator.Aggregator) error {
	srv := web.NewServer(cfg.Server, agg)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(cfg.Discovery.ScanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := agg.Refresh(ctx); err != nil {
					slog.Error("refresh failed", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	slog.Info("starting server", "addr", addr)

	go func() {
		if err := srv.ListenAndServe(addr); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), timeout.Shutdown)
	defer shutdownCancel()

	return srv.Shutdown(shutdownCtx)
}

func gurgehCmd() *cobra.Command {
	cmd := gurgehCli.NewRoot()
	cmd.Use = "gurgeh"
	cmd.Aliases = []string{"praude"}
	cmd.Short = "TUI-first PRD generation and validation"

	// Wrap to add intermute
	originalRunE := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		registerCtx, cancelRegister := context.WithTimeout(context.Background(), timeout.HTTPDefault)
		defer cancelRegister()
		if stop, err := intermute.RegisterTool(registerCtx, "gurgeh"); err != nil {
			// Log but don't fail
		} else if stop != nil {
			defer stop()
		}
		if originalRunE != nil {
			return originalRunE(c, args)
		}
		return nil
	}

	return cmd
}

func coldwineCmd() *cobra.Command {
	cmd := coldwineCli.RootCmd()
	cmd.Use = "coldwine"
	cmd.Aliases = []string{"tandemonium"}

	// Wrap to add intermute
	originalRunE := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		registerCtx, cancelRegister := context.WithTimeout(context.Background(), timeout.HTTPDefault)
		defer cancelRegister()
		if stop, err := intermute.RegisterTool(registerCtx, "coldwine"); err != nil {
			// Log but don't fail
		} else if stop != nil {
			defer stop()
		}
		if originalRunE != nil {
			return originalRunE(c, args)
		}
		return nil
	}

	return cmd
}

func pollardCmd() *cobra.Command {
	cmd := pollardCli.RootCmd()
	cmd.Use = "pollard"

	// Wrap to add intermute
	originalRunE := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		registerCtx, cancelRegister := context.WithTimeout(context.Background(), timeout.HTTPDefault)
		defer cancelRegister()
		if stop, err := intermute.RegisterTool(registerCtx, "pollard"); err != nil {
			// Log but don't fail
		} else if stop != nil {
			defer stop()
		}
		if originalRunE != nil {
			return originalRunE(c, args)
		}
		return nil
	}

	return cmd
}

func setupCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure Autarch hooks and directories",
		Long: `Set up Autarch for first-time use.

This command:
  - Creates ~/.autarch/ directory structure
  - Installs agent state hooks for Claude Code and Codex CLI
  - Verifies required dependencies (tmux, etc.)

Run this once after installing Autarch, or use --force to reconfigure.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			status := setup.Check()

			if !force && !setup.NeedsSetup() {
				fmt.Println("Autarch is already configured:")
				printSetupStatus(status)
				fmt.Println("\nUse --force to reconfigure.")
				return nil
			}

			fmt.Println("Setting up Autarch...")
			if err := setup.Run(); err != nil {
				return fmt.Errorf("setup failed: %w", err)
			}

			fmt.Println("\nSetup complete!")
			printSetupStatus(setup.Check())
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force reconfiguration even if already set up")

	return cmd
}

func printSetupStatus(s setup.Status) {
	check := func(b bool) string {
		if b {
			return "✓"
		}
		return "✗"
	}

	fmt.Printf("  %s Data directory (~/.autarch/)\n", check(s.DataDirExists))
	fmt.Printf("  %s Hook scripts installed\n", check(s.HooksInstalled))
	fmt.Printf("  %s Claude Code configured\n", check(s.ClaudeConfigured))
	fmt.Printf("  %s Codex CLI configured\n", check(s.CodexConfigured))
	fmt.Printf("  %s tmux available\n", check(s.TmuxAvailable))
}
