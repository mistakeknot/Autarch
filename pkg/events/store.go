package events

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver
)

// DefaultDBPath returns the default path for the events database
func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".autarch/events.db"
	}
	return filepath.Join(home, ".autarch", "events.db")
}

// Store provides SQLite-backed event storage with WAL mode
type Store struct {
	db   *sql.DB
	path string
}

// OpenStore opens or creates the events database
func OpenStore(path string) (*Store, error) {
	if path == "" {
		path = DefaultDBPath()
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Open with WAL mode for better concurrency
	dsn := path + "?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &Store{db: db, path: path}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return store, nil
}

// migrate creates the schema if needed
func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY,
    event_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    source_tool TEXT NOT NULL,
    payload JSON NOT NULL,
    project_path TEXT,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_entity ON events(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_events_source ON events(source_tool);
CREATE INDEX IF NOT EXISTS idx_events_time ON events(created_at);
CREATE INDEX IF NOT EXISTS idx_events_project ON events(project_path);
`
	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// Path returns the database file path
func (s *Store) Path() string {
	return s.path
}

// Append writes an event to the store
func (s *Store) Append(event *Event) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}

	result, err := s.db.Exec(`
		INSERT INTO events (event_type, entity_type, entity_id, source_tool, payload, project_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, event.EventType, event.EntityType, event.EntityID, event.SourceTool, event.Payload, event.ProjectPath, event.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err == nil {
		event.ID = id
	}
	return nil
}

// Query retrieves events matching the filter
func (s *Store) Query(filter *EventFilter) ([]*Event, error) {
	if filter == nil {
		filter = NewEventFilter()
	}

	query := "SELECT id, event_type, entity_type, entity_id, source_tool, payload, project_path, created_at FROM events WHERE 1=1"
	var args []interface{}

	if len(filter.EventTypes) > 0 {
		placeholders := make([]string, len(filter.EventTypes))
		for i, t := range filter.EventTypes {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += " AND event_type IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(filter.EntityTypes) > 0 {
		placeholders := make([]string, len(filter.EntityTypes))
		for i, t := range filter.EntityTypes {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += " AND entity_type IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(filter.EntityIDs) > 0 {
		placeholders := make([]string, len(filter.EntityIDs))
		for i, id := range filter.EntityIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND entity_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(filter.SourceTools) > 0 {
		placeholders := make([]string, len(filter.SourceTools))
		for i, t := range filter.SourceTools {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += " AND source_tool IN (" + strings.Join(placeholders, ",") + ")"
	}

	if filter.Since != nil {
		query += " AND created_at >= ?"
		args = append(args, filter.Since.Format(time.RFC3339Nano))
	}

	if filter.Until != nil {
		query += " AND created_at <= ?"
		args = append(args, filter.Until.Format(time.RFC3339Nano))
	}

	query += " ORDER BY id ASC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var e Event
		var createdAt string
		var projectPath sql.NullString
		if err := rows.Scan(&e.ID, &e.EventType, &e.EntityType, &e.EntityID, &e.SourceTool, &e.Payload, &projectPath, &createdAt); err != nil {
			return nil, err
		}
		if projectPath.Valid {
			e.ProjectPath = projectPath.String
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		events = append(events, &e)
	}

	return events, rows.Err()
}

// GetByID retrieves a single event by ID
func (s *Store) GetByID(id int64) (*Event, error) {
	row := s.db.QueryRow(`
		SELECT id, event_type, entity_type, entity_id, source_tool, payload, project_path, created_at
		FROM events WHERE id = ?
	`, id)

	var e Event
	var createdAt string
	var projectPath sql.NullString
	if err := row.Scan(&e.ID, &e.EventType, &e.EntityType, &e.EntityID, &e.SourceTool, &e.Payload, &projectPath, &createdAt); err != nil {
		return nil, err
	}
	if projectPath.Valid {
		e.ProjectPath = projectPath.String
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &e, nil
}

// LastID returns the highest event ID (for replay/sync)
func (s *Store) LastID() (int64, error) {
	row := s.db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM events")
	var id int64
	err := row.Scan(&id)
	return id, err
}

// Count returns the total number of events
func (s *Store) Count() (int64, error) {
	row := s.db.QueryRow("SELECT COUNT(*) FROM events")
	var count int64
	err := row.Scan(&count)
	return count, err
}

// Replay replays events since a given ID
func (s *Store) Replay(sinceID int64, filter *EventFilter, handler func(*Event) error) error {
	if filter == nil {
		filter = NewEventFilter()
	}

	query := "SELECT id, event_type, entity_type, entity_id, source_tool, payload, project_path, created_at FROM events WHERE id > ?"
	args := []interface{}{sinceID}

	if len(filter.EventTypes) > 0 {
		placeholders := make([]string, len(filter.EventTypes))
		for i, t := range filter.EventTypes {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += " AND event_type IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(filter.EntityTypes) > 0 {
		placeholders := make([]string, len(filter.EntityTypes))
		for i, t := range filter.EntityTypes {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += " AND entity_type IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(filter.SourceTools) > 0 {
		placeholders := make([]string, len(filter.SourceTools))
		for i, t := range filter.SourceTools {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += " AND source_tool IN (" + strings.Join(placeholders, ",") + ")"
	}

	query += " ORDER BY id ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var e Event
		var createdAt string
		var projectPath sql.NullString
		if err := rows.Scan(&e.ID, &e.EventType, &e.EntityType, &e.EntityID, &e.SourceTool, &e.Payload, &projectPath, &createdAt); err != nil {
			return err
		}
		if projectPath.Valid {
			e.ProjectPath = projectPath.String
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if err := handler(&e); err != nil {
			return err
		}
	}

	return rows.Err()
}

// MarshalPayload converts a struct to JSON payload
func MarshalPayload(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// UnmarshalPayload parses JSON payload into a struct
func UnmarshalPayload(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
