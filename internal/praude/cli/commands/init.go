package commands

import (
	"os"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
	"github.com/spf13/cobra"
)

func InitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize .praude/ in current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			if err := project.Init(root); err != nil {
				return err
			}
			_, err = specs.CreateTemplate(project.SpecsDir(root), time.Now())
			return err
		},
	}
}
