package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/internal/bigend/daemon"
	"github.com/mistakeknot/autarch/internal/bigend/discovery"
	bigendIntermute "github.com/mistakeknot/autarch/internal/bigend/intermute"
	bigendTui "github.com/mistakeknot/autarch/internal/bigend/tui"
	"github.com/mistakeknot/autarch/internal/bigend/web"
	coldwineCli "github.com/mistakeknot/autarch/internal/coldwine/cli"
	coldwineIntermute "github.com/mistakeknot/autarch/internal/coldwine/intermute"
	gurgehCli "github.com/mistakeknot/autarch/internal/gurgeh/cli"
	gurgehIntermute "github.com/mistakeknot/autarch/internal/gurgeh/intermute"
	pollardCli "github.com/mistakeknot/autarch/internal/pollard/cli"
	pollardIntermute "github.com/mistakeknot/autarch/internal/pollard/intermute"
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

	root.AddCommand(bigendCmd())
	root.AddCommand(gurgehCmd())
	root.AddCommand(coldwineCmd())
	root.AddCommand(pollardCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
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

			if stop, err := bigendIntermute.Start(context.Background()); err != nil {
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
		if stop, err := gurgehIntermute.Start(context.Background()); err != nil {
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
		if stop, err := coldwineIntermute.Start(context.Background()); err != nil {
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
		if stop, err := pollardIntermute.Start(context.Background()); err != nil {
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
