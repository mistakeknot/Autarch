package commands

import (
	"path/filepath"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/explore"
	"github.com/spf13/cobra"
)

func ScanCmd() *cobra.Command {
	var depth int
	cmd := &cobra.Command{
		Use:   "scan [path]",
		Short: "Scan repo for new epics",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			planDir := filepath.Join(root, ".tandemonium", "plan")
			_, err := explore.Run(root, planDir, explore.Options{Depth: depth})
			return err
		},
	}
	cmd.Flags().IntVar(&depth, "depth", 2, "scan depth (1-3)")
	return cmd
}
