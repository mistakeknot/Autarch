package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/discovery"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/tui"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/web"
)

func main() {
	var (
		port     = flag.Int("port", 8099, "HTTP server port")
		host     = flag.String("host", "0.0.0.0", "HTTP server bind address")
		scanRoot = flag.String("scan-root", "", "Root directory to scan for projects")
		cfgPath  = flag.String("config", "", "Path to config file")
		tuiMode  = flag.Bool("tui", false, "Run in TUI mode instead of web server")
	)
	flag.Parse()

	// Setup logging
	logLevel := slog.LevelInfo
	if *tuiMode {
		// Suppress logs in TUI mode to avoid interfering with display
		logLevel = slog.LevelError
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Load config
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Override with flags
	if *port != 8099 {
		cfg.Server.Port = *port
	}
	if *host != "0.0.0.0" {
		cfg.Server.Host = *host
	}
	if *scanRoot != "" {
		cfg.Discovery.ScanRoots = []string{*scanRoot}
	}

	// Create scanner
	scanner := discovery.NewScanner(cfg.Discovery)

	// Create aggregator
	agg := aggregator.New(scanner, cfg)

	// Initial scan
	if !*tuiMode {
		slog.Info("scanning for projects", "roots", cfg.Discovery.ScanRoots)
	}
	if err := agg.Refresh(context.Background()); err != nil {
		slog.Error("initial scan failed", "error", err)
	}

	if *tuiMode {
		runTUI(agg)
	} else {
		runWeb(cfg, agg)
	}
}

func runTUI(agg *aggregator.Aggregator) {
	m := tui.New(agg)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func runWeb(cfg *config.Config, agg *aggregator.Aggregator) {
	// Create web server
	srv := web.NewServer(cfg.Server, agg)

	// Start background refresh
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

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	slog.Info("starting server", "addr", addr)

	go func() {
		if err := srv.ListenAndServe(addr); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}
