package storage

import (
	"database/sql"
	"time"
)

// InsertEpic inserts a new epic
func InsertEpic(db *sql.DB, e Epic) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO epics (id, feature_ref, title, status, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.FeatureRef, e.Title, e.Status, e.Priority, now, now)
	return err
}

// GetEpic retrieves an epic by ID
func GetEpic(db *sql.DB, id string) (Epic, error) {
	row := db.QueryRow(`
		SELECT id, feature_ref, title, status, priority, created_at, updated_at
		FROM epics WHERE id = ?`, id)

	var e Epic
	var createdAt, updatedAt string
	if err := row.Scan(&e.ID, &e.FeatureRef, &e.Title, &e.Status, &e.Priority, &createdAt, &updatedAt); err != nil {
		return Epic{}, err
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return e, nil
}

// ListEpics returns all epics
func ListEpics(db *sql.DB) ([]Epic, error) {
	rows, err := db.Query(`
		SELECT id, feature_ref, title, status, priority, created_at, updated_at
		FROM epics ORDER BY priority ASC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var epics []Epic
	for rows.Next() {
		var e Epic
		var createdAt, updatedAt string
		if err := rows.Scan(&e.ID, &e.FeatureRef, &e.Title, &e.Status, &e.Priority, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		epics = append(epics, e)
	}
	return epics, nil
}

// ListEpicsByFeature returns epics linked to a feature
func ListEpicsByFeature(db *sql.DB, featureRef string) ([]Epic, error) {
	rows, err := db.Query(`
		SELECT id, feature_ref, title, status, priority, created_at, updated_at
		FROM epics WHERE feature_ref = ? ORDER BY priority ASC`, featureRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var epics []Epic
	for rows.Next() {
		var e Epic
		var createdAt, updatedAt string
		if err := rows.Scan(&e.ID, &e.FeatureRef, &e.Title, &e.Status, &e.Priority, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		epics = append(epics, e)
	}
	return epics, nil
}

// UpdateEpicStatus updates an epic's status
func UpdateEpicStatus(db *sql.DB, id string, status EpicStatus) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE epics SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
	return err
}

// DeleteEpic deletes an epic and all its stories/tasks (via cascade)
func DeleteEpic(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM epics WHERE id = ?`, id)
	return err
}
