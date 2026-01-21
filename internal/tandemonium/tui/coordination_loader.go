package tui

import (
	"database/sql"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func LoadCoordInbox(db *sql.DB, recipient string, limit int, urgentOnly bool) ([]storage.MessageDelivery, error) {
	return storage.FetchInboxWithFilters(db, recipient, limit, "", urgentOnly)
}

func LoadCoordInboxFromProject(recipient string, limit int, urgentOnly bool) ([]storage.MessageDelivery, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return nil, err
	}
	db, err := storage.OpenShared(project.StateDBPath(root))
	if err != nil {
		return nil, err
	}
	if err := storage.Migrate(db); err != nil {
		return nil, err
	}
	return LoadCoordInbox(db, recipient, limit, urgentOnly)
}

func LoadCoordLocks(db *sql.DB, limit int) ([]storage.Reservation, error) {
	return storage.ListActiveReservations(db, limit)
}

func LoadCoordLocksFromProject(limit int) ([]storage.Reservation, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return nil, err
	}
	db, err := storage.OpenShared(project.StateDBPath(root))
	if err != nil {
		return nil, err
	}
	if err := storage.Migrate(db); err != nil {
		return nil, err
	}
	return LoadCoordLocks(db, limit)
}
