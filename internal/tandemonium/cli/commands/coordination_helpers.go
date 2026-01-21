package commands

import (
	"database/sql"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func openStateDB() (*sql.DB, func(), error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return nil, nil, err
	}
	db, err := storage.Open(project.StateDBPath(root))
	if err != nil {
		return nil, nil, err
	}
	if err := storage.Migrate(db); err != nil {
		db.Close()
		return nil, nil, err
	}
	return db, func() { _ = db.Close() }, nil
}
