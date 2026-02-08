package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/pollard/inbox"
	"github.com/mistakeknot/autarch/internal/pollard/server"
	"github.com/spf13/cobra"
)

func serveCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve Pollard HTTP API (local-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			srv, err := server.New(root)
			if err != nil {
				return err
			}
			defer srv.Close()
			if strings.TrimSpace(os.Getenv("INTERMUTE_URL")) != "" {
				handler := inbox.NewHandler(srv.Scanner(), 2*time.Second)
				go handler.Run(cmd.Context())
				fmt.Fprintln(cmd.OutOrStdout(), "Intermute inbox polling enabled")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Pollard API listening on %s\n", addr)
			return srv.ListenAndServe(addr)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8090", "HTTP bind address")
	return cmd
}
