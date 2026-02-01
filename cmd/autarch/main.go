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

	"github.com/mistakeknot/autarch/internal/autarch/setup"
	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/internal/bigend/daemon"
	"github.com/mistakeknot/autarch/internal/bigend/discovery"
	bigendTui "github.com/mistakeknot/autarch/internal/bigend/tui"
	"github.com/mistakeknot/autarch/internal/bigend/web"
	coldwineCli "github.com/mistakeknot/autarch/internal/coldwine/cli"
	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/planstatus"
	gurgehCli "github.com/mistakeknot/autarch/internal/gurgeh/cli"
	internalIntermute "github.com/mistakeknot/autarch/internal/intermute"
	pollardCli "github.com/mistakeknot/autarch/internal/pollard/cli"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/internal/tui/views"
	"github.com/mistakeknot/autarch/pkg/autarch"
	"github.com/mistakeknot/autarch/pkg/intermute"
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
	root.AddCommand(reconcileCmd())
	root.AddCommand(eventsCmd())
	root.AddCommand(planstatus.NewCommand())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func tuiCmd() *cobra.Command {
	var (
		port        int
		dataDir     string
		skipOnboard bool
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
  1-4       Switch between tabs (Bigend, Gurgeh, Coldwine, Pollard)
  Ctrl+P    Open command palette
  ?         Show help
  Ctrl+C    Quit`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Suppress logging in TUI mode
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelError,
			}))
			slog.SetDefault(logger)

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

			// Ensure Intermute is running (detect existing or start new)
			cleanup, err := mgr.EnsureRunning(context.Background())
			if err != nil {
				return fmt.Errorf("failed to ensure intermute running: %w", err)
			}
			defer cleanup()

			// Create client connecting to Intermute server
			client := autarch.NewClient(mgr.URL())

			if skipOnboard {
				var selector *pkgtui.AgentSelector
				if cwd, err := os.Getwd(); err == nil {
					if options, err := tui.LoadAgentOptions(cwd); err == nil {
						filtered := make([]pkgtui.AgentOption, 0, len(options))
						for _, opt := range options {
							switch strings.ToLower(opt.Name) {
							case "codex", "claude":
								filtered = append(filtered, opt)
							}
						}
						if len(filtered) > 0 {
							selector = pkgtui.NewAgentSelector(filtered)
						}
					}
				}

				// Skip onboarding, go directly to dashboard
				bigendView := views.NewBigendView(client)
				signalsView := views.NewSignalsView(client)
				gurgehView := views.NewGurgehView(client)
				coldwineView := views.NewColdwineView(client)
				pollardView := views.NewPollardView(client)

				if selector != nil {
					if setter, ok := any(gurgehView).(interface{ SetAgentSelector(*pkgtui.AgentSelector) }); ok {
						setter.SetAgentSelector(selector)
					}
					if setter, ok := any(coldwineView).(interface{ SetAgentSelector(*pkgtui.AgentSelector) }); ok {
						setter.SetAgentSelector(selector)
					}
					if setter, ok := any(pollardView).(interface{ SetAgentSelector(*pkgtui.AgentSelector) }); ok {
						setter.SetAgentSelector(selector)
					}
				}

				return tui.Run(client,
					bigendView,
					signalsView,
					gurgehView,
					coldwineView,
					pollardView,
				)
			}

			// Create unified app with onboarding flow
			app := tui.NewUnifiedApp(client)

			// Set up view factories for state transitions
			app.SetViewFactories(
				// Kickoff view factory
				func() tui.View {
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
					v.SetResolveOpenQuestionsCallback(func(req tui.OpenQuestionsRequest) tea.Cmd {
						return func() tea.Msg {
							return req
						}
					})
					return v
				},
				// Spec summary view factory
				func(spec *tui.SpecSummary, coord *research.Coordinator) tui.View {
					return views.NewSpecSummaryView(spec, coord)
				},
				// Epic review view factory
				func(proposals []epics.EpicProposal) tui.View {
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
				// Task review view factory
				func(taskList []tasks.TaskProposal) tui.View {
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
				// Task detail view factory
				func(task tasks.TaskProposal, coord *research.Coordinator) tui.View {
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
				// Dashboard views factory
				func(c *autarch.Client) []tui.View {
					return []tui.View{
						views.NewBigendView(c),
						views.NewSignalsView(c),
						views.NewGurgehView(c),
						views.NewColdwineView(c),
						views.NewPollardView(c),
					}
				},
			)

			// Wire unified sprint view (chat-driven 8-phase flow)
			app.SetSprintViewFactory(func(projectPath string) tui.View {
				v := views.NewSprintView(projectPath)
				v.SetCallbacks(func() tea.Cmd {
					return func() tea.Msg { return tui.NavigateBackMsg{} }
				})
				return v
			})

			return tui.RunUnified(client, app)
		},
	}

	cmd.Flags().IntVar(&port, "port", 7338, "Intermute server port")
	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory (default: ~/.autarch)")
	cmd.Flags().BoolVar(&skipOnboard, "skip-onboard", false, "Skip onboarding and go directly to dashboard")

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
			// Setup logging
			logLevel := slog.LevelInfo
			if tuiMode {
				logLevel = slog.LevelError
			}
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: logLevel,
			}))
			slog.SetDefault(logger)

			if stop, err := intermute.RegisterTool(context.Background(), "bigend"); err != nil {
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
			agg := aggregator.New(scanner, cfg)

			if !tuiMode {
				slog.Info("scanning for projects", "roots", cfg.Discovery.ScanRoots)
			}
			if err := agg.Refresh(context.Background()); err != nil {
				slog.Error("initial scan failed", "error", err)
			}

			if daemonMode {
				return runBigendDaemon(daemonAddr, cfg.Discovery.ScanRoots)
			} else if tuiMode {
				return runBigendTUI(agg)
			}
			return runBigendWeb(cfg, agg)
		},
	}

	cmd.Flags().IntVar(&port, "port", 8099, "HTTP server port")
	cmd.Flags().StringVar(&host, "host", "0.0.0.0", "HTTP server bind address")
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.Start(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func runBigendTUI(agg *aggregator.Aggregator) error {
	m := bigendTui.New(agg, buildInfoString())
	p := tea.NewProgram(m, tea.WithAltScreen())
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		if stop, err := intermute.RegisterTool(context.Background(), "gurgeh"); err != nil {
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
		if stop, err := intermute.RegisterTool(context.Background(), "coldwine"); err != nil {
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
		if stop, err := intermute.RegisterTool(context.Background(), "pollard"); err != nil {
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
