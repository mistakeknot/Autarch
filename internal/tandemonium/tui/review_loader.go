package tui

import (
	"database/sql"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func LoadReviewQueue(db *sql.DB) ([]string, error) {
	return storage.ListReviewQueue(db)
}

func LoadReviewQueueFromProject() ([]string, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return nil, err
	}
	db, err := storage.OpenShared(project.StateDBPath(root))
	if err != nil {
		return nil, err
	}
	return storage.ListReviewQueue(db)
}
