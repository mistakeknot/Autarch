package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/pkg/mcp"
)

func mcpCmd() *cobra.Command {
	var project string

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the Autarch MCP server over stdin/stdout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("get current directory: %w", err)
				}
				project = cwd
			}
			if _, err := os.Stat(project); err != nil {
				return fmt.Errorf("project directory: %w", err)
			}

			parent := cmd.Context()
			if parent == nil {
				parent = context.Background()
			}
			ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			srv := mcp.NewServer(project).WithIO(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err := srv.Run(ctx); err != nil && err != context.Canceled {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project directory (default: current directory)")
	return cmd
}
