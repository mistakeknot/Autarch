package tui

import (
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

type ApproveAdapter struct{}

func (a *ApproveAdapter) Approve(taskID, branch string) error {
	if err := git.MergeBranch(&git.ExecRunner{}, branch); err != nil {
		return err
	}
	root, err := project.FindRoot(".")
	if err != nil {
		return err
	}
	db, err := storage.OpenShared(project.StateDBPath(root))
	if err != nil {
		return err
	}
	return storage.ApproveTask(db, taskID)
}
