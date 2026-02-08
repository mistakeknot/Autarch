package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/exploration"
	"github.com/spf13/cobra"
)

func ExploreCmd() *cobra.Command {
	var jsonOut bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "explore [path]",
		Short: "Explore a codebase using Claude Code for PRD evidence",
		Long: `Invokes Claude Code subprocess to iteratively explore a codebase
and gather evidence for PRD generation (Vision, Problem, Users).

If no path is provided, uses the current working directory.

Example:
  gurgeh explore                    # Explore current directory
  gurgeh explore /path/to/project   # Explore specific path
  gurgeh explore --json             # Output as JSON`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			// Resolve to absolute path
			if path == "." {
				var err error
				path, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("failed to get working directory: %w", err)
				}
			}

			// Verify path exists
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return fmt.Errorf("path does not exist: %s", path)
			}

			if !jsonOut {
				fmt.Printf("🔍 Exploring codebase at: %s\n", path)
				fmt.Printf("⏱️  Timeout: %s\n", timeout)
				fmt.Println("⏳ This may take a few minutes...")
				fmt.Println()
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			start := time.Now()
			result, _, err := exploration.Explore(ctx, path)
			elapsed := time.Since(start)

			if err != nil {
				if jsonOut {
					errJSON, _ := json.MarshalIndent(map[string]any{
						"error":   err.Error(),
						"elapsed": elapsed.String(),
					}, "", "  ")
					fmt.Println(string(errJSON))
					return nil
				}
				return fmt.Errorf("exploration failed: %w", err)
			}

			if jsonOut {
				result["_elapsed"] = elapsed.String()
				out, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal result: %w", err)
				}
				fmt.Println(string(out))
				return nil
			}

			// Pretty print the result
			fmt.Printf("✅ Exploration complete in %s\n\n", elapsed.Round(time.Second))

			if vision, ok := result["vision"]; ok {
				fmt.Println("📌 Vision:")
				printValue(vision, "  ")
				fmt.Println()
			}

			if problem, ok := result["problem"]; ok {
				fmt.Println("🎯 Problem:")
				printValue(problem, "  ")
				fmt.Println()
			}

			if users, ok := result["users"]; ok {
				fmt.Println("👥 Users:")
				printValue(users, "  ")
				fmt.Println()
			}

			// Print any other keys
			for k, v := range result {
				if k == "vision" || k == "problem" || k == "users" || k == "_elapsed" {
					continue
				}
				fmt.Printf("📎 %s:\n", k)
				printValue(v, "  ")
				fmt.Println()
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output result as JSON")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Exploration timeout")

	return cmd
}

func printValue(v any, indent string) {
	switch val := v.(type) {
	case string:
		fmt.Printf("%s%s\n", indent, val)
	case map[string]any:
		for k, vv := range val {
			fmt.Printf("%s%s: ", indent, k)
			if s, ok := vv.(string); ok && len(s) < 60 {
				fmt.Println(s)
			} else {
				fmt.Println()
				printValue(vv, indent+"  ")
			}
		}
	case []any:
		for _, item := range val {
			printValue(item, indent+"- ")
		}
	default:
		fmt.Printf("%s%v\n", indent, val)
	}
}
