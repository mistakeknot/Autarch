package commands

import (
	"fmt"
	"path/filepath"

	"github.com/mistakeknot/autarch/internal/coldwine/drift"
	"github.com/mistakeknot/autarch/internal/coldwine/explore"
	"github.com/mistakeknot/autarch/pkg/events"
	"github.com/spf13/cobra"
)

func ScanCmd() *cobra.Command {
	var depth int
	var actionItems bool
	var minConfidence int
	cmd := &cobra.Command{
		Use:   "scan [path]",
		Short: "Scan repo for new epics",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			if minConfidence < 0 || minConfidence > 100 {
				return fmt.Errorf("--min-confidence must be between 0 and 100")
			}
			if actionItems {
				return runActionItemScan(cmd, root, minConfidence)
			}
			planDir := filepath.Join(root, ".coldwine", "plan")
			_, err := explore.Run(root, planDir, explore.Options{Depth: depth})
			return err
		},
	}
	cmd.Flags().IntVar(&depth, "depth", 2, "scan depth (1-3)")
	cmd.Flags().BoolVar(&actionItems, "action-items", false, "scan docs for untracked action items and emit events")
	cmd.Flags().IntVar(&minConfidence, "min-confidence", 75, "minimum confidence threshold (0-100) for --action-items")
	return cmd
}

func runActionItemScan(cmd *cobra.Command, root string, minConfidence int) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	items, err := drift.ScanDocActionItems(absRoot)
	if err != nil {
		return err
	}
	reconciled, err := drift.ReconcileActionItems(absRoot, items)
	if err != nil {
		return err
	}

	store, err := events.OpenStore("")
	if err != nil {
		return err
	}
	defer store.Close()

	writer := events.NewWriter(store, events.SourceColdwine)
	writer.SetProjectPath(absRoot)
	writer.SetContext(cmd.Context())

	untracked := selectUntrackedByConfidence(reconciled, minConfidence)
	for _, item := range untracked {
		payload := events.UntrackedItemDetectedPayload{
			ID:         item.ID,
			SourcePath: item.SourcePath,
			Kind:       item.Kind,
			Text:       item.Text,
			Confidence: item.Confidence,
			Line:       item.Line,
			Matched:    item.Matched,
		}
		if err := writer.EmitUntrackedItemDetected(payload); err != nil {
			return err
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Found %d untracked action items >= %d%% confidence\n", len(untracked), minConfidence)
	for _, item := range untracked {
		fmt.Fprintf(cmd.OutOrStdout(), "- [%d%%] %s:%d %s\n", item.Confidence, item.SourcePath, item.Line, item.Text)
	}
	return nil
}

func selectUntrackedByConfidence(items []drift.ReconciledActionItem, minConfidence int) []drift.ReconciledActionItem {
	untracked := make([]drift.ReconciledActionItem, 0)
	for _, item := range items {
		if item.Tracked || item.Confidence < minConfidence {
			continue
		}
		untracked = append(untracked, item)
	}
	return untracked
}
