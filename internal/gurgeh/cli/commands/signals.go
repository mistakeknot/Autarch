package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	gsignals "github.com/mistakeknot/autarch/internal/gurgeh/signals"
	"github.com/mistakeknot/autarch/pkg/signals"
	"github.com/spf13/cobra"
)

// SignalsCmd manages vision spec signals.
func SignalsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "signals",
		Short: "Manage vision spec quality signals",
	}

	cmd.AddCommand(signalsListCmd(), signalsDismissCmd())
	return cmd
}

func signalsListCmd() *cobra.Command {
	var specID string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List active signals (JSON output)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := project.EnsureInitialized(cwd); err != nil {
				return err
			}

			store, err := gsignals.NewStore(cwd)
			if err != nil {
				return fmt.Errorf("opening signal store: %w", err)
			}
			defer store.Close()

			var sigs []signals.Signal
			if specID != "" {
				sigs, err = store.Active(specID)
			} else {
				sigs, err = store.ActiveAll()
			}
			if err != nil {
				return fmt.Errorf("querying signals: %w", err)
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(sigs)
		},
	}

	cmd.Flags().StringVar(&specID, "spec-id", "", "Filter signals by spec ID")
	return cmd
}

func signalsDismissCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dismiss <signal-id>",
		Short: "Dismiss a signal by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := project.EnsureInitialized(cwd); err != nil {
				return err
			}

			store, err := gsignals.NewStore(cwd)
			if err != nil {
				return fmt.Errorf("opening signal store: %w", err)
			}
			defer store.Close()

			if err := store.Dismiss(args[0]); err != nil {
				return fmt.Errorf("dismissing signal: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Dismissed signal %s\n", args[0])
			return nil
		},
	}
}
