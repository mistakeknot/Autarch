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
	cmd := &cobra.Command{
		Use:   "scan [path]",
		Short: "Scan repo for new epics",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			if actionItems {
				return runActionItemScan(cmd, root)
			}
			planDir := filepath.Join(root, ".tandemonium", "plan")
			_, err := explore.Run(root, planDir, explore.Options{Depth: depth})
			return err
		},
	}
	cmd.Flags().IntVar(&depth, "depth", 2, "scan depth (1-3)")
	cmd.Flags().BoolVar(&actionItems, "action-items", false, "scan docs for untracked action items and emit events")
	return cmd
}

func runActionItemScan(cmd *cobra.Command, root string) error {
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

	untracked := make([]drift.ReconciledActionItem, 0)
	for _, item := range reconciled {
		if item.Tracked {
			continue
		}
		untracked = append(untracked, item)
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

	fmt.Fprintf(cmd.OutOrStdout(), "Found %d action items (%d untracked)\n", len(reconciled), len(untracked))
	for _, item := range untracked {
		fmt.Fprintf(cmd.OutOrStdout(), "- [%d%%] %s:%d %s\n", item.Confidence, item.SourcePath, item.Line, item.Text)
	}
	return nil
}
