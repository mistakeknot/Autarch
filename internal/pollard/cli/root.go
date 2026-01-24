// Package cli provides the command-line interface for Pollard.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "pollard",
	Short: "Pollard - Continuous research intelligence",
	Long: `Pollard gathers competitive landscape data, user flow patterns,
open source implementations, and industry trends to enrich
Praude and Tandemonium artifacts.`,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(initCmd)
}
