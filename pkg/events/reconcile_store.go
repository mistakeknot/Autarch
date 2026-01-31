package events

import (
	"database/sql"
	"encoding/json"
	"time"
)

// ReconcileCursor tracks the last reconciled fingerprint for an entity.
type ReconcileCursor struct {
	ProjectPath string
	EntityType  EntityType
	EntityID    string
	Fingerprint string
	Status      string
	Version     int
	UpdatedAt   time.Time
}

// ReconcileConflict records a reconciliation conflict.
type ReconcileConflict struct {
	ID          int64
	ProjectPath string
	EntityType  EntityType
	EntityID    string
	Reason      string
	Details     map[string]interface{}
	CreatedAt   time.Time
}

// GetCursor retrieves a reconciliation cursor for an entity.
func (s *Store) GetCursor(projectPath string, entityType EntityType, entityID string) (*ReconcileCursor, error) {
	row := s.db.QueryRow(`
		SELECT project_path, entity_type, entity_id, fingerprint, status, version, updated_at
		FROM reconcile_cursors
		WHERE project_path = ? AND entity_type = ? AND entity_id = ?
	`, projectPath, string(entityType), entityID)

	var cursor ReconcileCursor
	var updatedAt string
	var status sql.NullString
	var version sql.NullInt64
	if err := row.Scan(&cursor.ProjectPath, &cursor.EntityType, &cursor.EntityID, &cursor.Fingerprint, &status, &version, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if status.Valid {
		cursor.Status = status.String
	}
	if version.Valid {
		cursor.Version = int(version.Int64)
	}
	cursor.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &cursor, nil
}

// UpsertCursor inserts or updates a reconciliation cursor.
func (s *Store) UpsertCursor(cursor *ReconcileCursor) error {
	if cursor.UpdatedAt.IsZero() {
		cursor.UpdatedAt = time.Now()
	}
	_, err := s.db.Exec(`
		INSERT INTO reconcile_cursors (project_path, entity_type, entity_id, fingerprint, status, version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_path, entity_type, entity_id)
		DO UPDATE SET fingerprint = excluded.fingerprint,
			status = excluded.status,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, cursor.ProjectPath, string(cursor.EntityType), cursor.EntityID, cursor.Fingerprint, cursor.Status, cursor.Version, cursor.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

// LogConflict records a reconciliation conflict.
func (s *Store) LogConflict(conflict *ReconcileConflict) error {
	if conflict.CreatedAt.IsZero() {
		conflict.CreatedAt = time.Now()
	}
	var detailsJSON []byte
	if conflict.Details != nil {
		data, err := json.Marshal(conflict.Details)
		if err != nil {
			return err
		}
		detailsJSON = data
	}

	_, err := s.db.Exec(`
		INSERT INTO reconcile_conflicts (project_path, entity_type, entity_id, reason, details, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, conflict.ProjectPath, string(conflict.EntityType), conflict.EntityID, conflict.Reason, detailsJSON, conflict.CreatedAt.Format(time.RFC3339Nano))
	return err
}

// ListConflicts returns recent reconciliation conflicts.
func (s *Store) ListConflicts(projectPath string, limit int) ([]ReconcileConflict, error) {
	query := `SELECT id, project_path, entity_type, entity_id, reason, details, created_at FROM reconcile_conflicts`
	var args []interface{}
	if projectPath != "" {
		query += " WHERE project_path = ?"
		args = append(args, projectPath)
	}
	query += " ORDER BY id DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conflicts []ReconcileConflict
	for rows.Next() {
		var c ReconcileConflict
		var details sql.NullString
		var createdAt string
		if err := rows.Scan(&c.ID, &c.ProjectPath, &c.EntityType, &c.EntityID, &c.Reason, &details, &createdAt); err != nil {
			return nil, err
		}
		if details.Valid {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(details.String), &parsed); err == nil {
				c.Details = parsed
			}
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		conflicts = append(conflicts, c)
	}

	return conflicts, rows.Err()
}
