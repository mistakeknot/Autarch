// Package cli provides the command-line interface for Pollard.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/pollard/config"
	"github.com/mistakeknot/autarch/internal/pollard/proposal"
)

var rootCmd = &cobra.Command{
	Use:   "pollard",
	Short: "Pollard - Continuous research intelligence",
	Long: `Pollard gathers competitive landscape data, user flow patterns,
open source implementations, and industry trends to enrich
Praude and Tandemonium artifacts.`,
	RunE: runStatus,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// RootCmd returns the root command for embedding in other CLIs
func RootCmd() *cobra.Command {
	return rootCmd
}

func init() {
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(initCmd)
	// Agent-native research commands
	rootCmd.AddCommand(researchCmd)
	rootCmd.AddCommand(suggestCmd)
	rootCmd.AddCommand(hunterCmd)
	rootCmd.AddCommand(proposeCmd)
}

// runStatus shows Pollard status and suggests next actions.
func runStatus(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Check if initialized
	pollardDir := filepath.Join(cwd, ".pollard")
	if _, err := os.Stat(pollardDir); os.IsNotExist(err) {
		fmt.Println("Pollard is not initialized in this project.")
		fmt.Println()
		fmt.Println("To get started:")
		fmt.Println("  pollard init      # Initialize Pollard with default config")
		fmt.Println("  pollard propose   # Generate research agendas from your docs")
		return nil
	}

	// Load config
	cfg, err := config.Load(cwd)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check for proposals
	proposals, proposalErr := proposal.LoadResult(cwd)
	hasProposals := proposalErr == nil && len(proposals.Agendas) > 0

	// Display status
	fmt.Println("Pollard Status")
	fmt.Println("==============")
	fmt.Println()

	// Project info
	scanner := proposal.NewContextScanner(cwd)
	ctx, _ := scanner.Scan()
	if ctx != nil && ctx.ProjectName != "" {
		fmt.Printf("Project: %s\n", ctx.ProjectName)
		if ctx.DetectedType != "" && ctx.DetectedType != "unknown" {
			fmt.Printf("Type: %s\n", ctx.DetectedType)
		}
		if len(ctx.Technologies) > 0 {
			fmt.Printf("Technologies: %s\n", strings.Join(ctx.Technologies, ", "))
		}
		fmt.Println()
	}

	// Hunters status
	enabledHunters := cfg.EnabledHunters()
	fmt.Printf("Hunters: %d enabled\n", len(enabledHunters))
	if len(enabledHunters) > 0 {
		for _, name := range enabledHunters {
			hcfg, _ := cfg.GetHunterConfig(name)
			fmt.Printf("  • %s (%d queries)\n", name, len(hcfg.Queries))
		}
	}
	fmt.Println()

	// Proposals status
	if hasProposals {
		fmt.Printf("Research Agendas: %d proposals\n", len(proposals.Agendas))
		for _, agenda := range proposals.Agendas {
			fmt.Printf("  • [%s] %s (%s)\n", agenda.ID, agenda.Title, agenda.Priority)
		}
		fmt.Printf("  Generated: %s\n", proposals.GeneratedAt.Format(time.RFC3339))
		fmt.Println()
	}

	// Suggest next actions
	fmt.Println("Actions:")
	if !hasProposals {
		fmt.Println("  pollard propose    # Generate research agendas from your docs (recommended)")
	} else {
		fmt.Println("  pollard propose    # Regenerate research agendas")
		fmt.Println("  pollard propose --select  # Apply agendas to hunter config")
	}
	fmt.Println("  pollard scan       # Run research hunters")
	fmt.Println("  pollard report     # Generate research report")

	return nil
}
