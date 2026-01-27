package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/autarch/internal/gurgeh/project"
	gsignals "github.com/mistakeknot/autarch/internal/gurgeh/signals"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// VisionReviewCmd manages vision spec review lifecycle.
func VisionReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "vision-review",
		Aliases: []string{"vr"},
		Short:   "Check and trigger vision spec reviews",
	}

	cmd.AddCommand(visionReviewCheckCmd())
	return cmd
}

func visionReviewCheckCmd() *cobra.Command {
	var specID string

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check if vision spec needs review (JSON output)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := project.EnsureInitialized(cwd); err != nil {
				return err
			}

			// Find vision spec
			spec, err := findVisionSpec(cwd, specID)
			if err != nil {
				return err
			}

			// Open signal store
			store, err := gsignals.NewStore(cwd)
			if err != nil {
				return fmt.Errorf("opening signal store: %w", err)
			}
			defer store.Close()

			status := gsignals.CheckReviewNeeded(spec, store)

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(status)
		},
	}

	cmd.Flags().StringVar(&specID, "spec-id", "", "Vision spec ID (auto-detected if omitted)")
	return cmd
}

// findVisionSpec locates a vision spec by ID or auto-detects the first one.
func findVisionSpec(root string, specID string) (*specs.Spec, error) {
	specsDir := filepath.Join(root, ".gurgeh", "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return nil, fmt.Errorf("reading specs dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specsDir, entry.Name()))
		if err != nil {
			continue
		}
		var s specs.Spec
		if err := yaml.Unmarshal(data, &s); err != nil {
			continue
		}
		if s.EffectiveType() != specs.SpecTypeVision {
			continue
		}
		if specID != "" && s.ID != specID {
			continue
		}
		return &s, nil
	}

	if specID != "" {
		return nil, fmt.Errorf("vision spec %q not found", specID)
	}
	return nil, fmt.Errorf("no vision spec found in %s", specsDir)
}
