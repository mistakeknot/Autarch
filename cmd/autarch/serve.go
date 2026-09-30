package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/bigend/config"
	"github.com/mistakeknot/autarch/internal/serve"
)

func serveCmd() *cobra.Command {
	var (
		addr         string
		projectDirs  []string
		tokenFile    string
		allowOrigins []string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the consolidated read-only local service (Bigend, Gurgeh, Signals)",
		Long: `Run one loopback HTTP service that mounts Bigend at /bigend/, Signals at
/signals/ and each project's Gurgeh at /gurgeh/{project}/.

Every route except /health needs the bearer token stored in --token-file
(created with mode 0600 on first start). GET /api/projects lists the resolved
project roots. The service holds no decisions and only binds loopback addresses.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dirs := projectDirs
			if len(dirs) == 0 {
				cfg, err := config.Load("")
				if err != nil {
					return fmt.Errorf("load bigend discovery defaults: %w", err)
				}
				dirs = cfg.Discovery.ScanRoots
			}
			tokenPath := tokenFile
			if tokenPath == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				tokenPath = filepath.Join(home, ".autarch", "serve.token")
			}

			srv, err := serve.New(serve.Config{
				Addr:         addr,
				ProjectDirs:  dirs,
				TokenPath:    tokenPath,
				AllowOrigins: allowOrigins,
			})
			if err != nil {
				return err
			}
			ln, err := srv.Listen()
			if err != nil {
				return err
			}

			parent := cmd.Context()
			if parent == nil {
				parent = context.Background()
			}
			ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			// Never print the token, only where it lives.
			fmt.Fprintf(cmd.OutOrStdout(), "autarch serve listening on %s (token file: %s)\n", ln.Addr(), tokenPath)
			return srv.Serve(ctx, ln)
		},
	}

	cmd.Flags().StringVar(&addr, "addr", serve.DefaultAddr, "Listen address (loopback only)")
	cmd.Flags().StringArrayVar(&projectDirs, "project-dir", nil, "Scan root whose child directories are projects (repeatable; default: Bigend discovery roots)")
	cmd.Flags().StringVar(&tokenFile, "token-file", "", "Bearer token file (default ~/.autarch/serve.token)")
	cmd.Flags().StringArrayVar(&allowOrigins, "allow-origin", nil, "Allowed browser Origin (repeatable; default none)")
	return cmd
}
