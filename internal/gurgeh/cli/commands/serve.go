package commands

import (
	"fmt"
	"os"

	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	"github.com/mistakeknot/autarch/internal/gurgeh/server"
	"github.com/spf13/cobra"
)

func ServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve Gurgeh Spec API (local-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), `deprecated: use "autarch serve" (mounted at /gurgeh/{project}/)`)
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := project.EnsureInitialized(root); err != nil {
				return err
			}
			srv := server.New(root)
			fmt.Fprintf(cmd.OutOrStdout(), "Gurgeh API listening on %s\n", addr)
			return srv.ListenAndServe(addr)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8091", "HTTP bind address")
	cmd.SetOut(os.Stdout)
	return cmd
}
