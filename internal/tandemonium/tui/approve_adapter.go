package tui

import (
	"database/sql"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

type ApproveAdapter struct {
	DB     *sql.DB
	Runner git.Runner
}

func (a *ApproveAdapter) Approve(taskID, branch string) error {
	runner := a.Runner
	if runner == nil {
		runner = &git.ExecRunner{}
	}
	if err := git.MergeBranch(runner, branch); err != nil {
		return err
	}
	db := a.DB
	if db == nil {
		root, err := project.FindRoot(".")
		if err != nil {
			return err
		}
		db, err = storage.OpenShared(project.StateDBPath(root))
		if err != nil {
			return err
		}
	}
	return storage.ApproveTask(db, taskID)
}
