package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
	"github.com/spf13/cobra"
)

func ValidateCmd() *cobra.Command {
	var mode string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "validate <id>",
		Short: "Validate a PRD spec",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, err := config.LoadFromRoot(root)
			if err != nil {
				return err
			}
			selected := mode
			if selected == "" {
				selected = cfg.ValidationMode
			}
			if err := validateMode(selected); err != nil {
				return err
			}
			id := args[0]
			path, err := resolveSpecPath(project.SpecsDir(root), id)
			if err != nil {
				return err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			res, err := specs.Validate(raw, specs.ValidationOptions{
				Mode: specs.ValidationMode(selected),
				Root: root,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				payload := validationJSON{
					ID:       id,
					Mode:     selected,
					Errors:   res.Errors,
					Warnings: res.Warnings,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			if len(res.Errors) > 0 {
				return fmt.Errorf("validation failed: %s", strings.Join(res.Errors, "; "))
			}
			if len(res.Warnings) > 0 {
				if specs.ValidationMode(selected) == specs.ValidationSoft {
					if err := specs.StoreValidationWarnings(path, res.Warnings); err != nil {
						return err
					}
				}
				for _, warning := range res.Warnings {
					fmt.Fprintln(cmd.OutOrStdout(), "WARN:", warning)
				}
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OK")
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "Validation mode (hard|soft)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print validation results as JSON")
	return cmd
}

type validationJSON struct {
	ID       string   `json:"id"`
	Mode     string   `json:"mode"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func validateMode(mode string) error {
	if mode != "hard" && mode != "soft" {
		return fmt.Errorf("invalid validation mode %q", mode)
	}
	return nil
}

func resolveSpecPath(dir, id string) (string, error) {
	path := filepath.Join(dir, id+".yaml")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	alt := filepath.Join(dir, id+".yml")
	if _, err := os.Stat(alt); err == nil {
		return alt, nil
	}
	return "", fmt.Errorf("spec not found: %s", id)
}
