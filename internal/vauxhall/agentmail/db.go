package agentmail

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB provides read-only access to an MCP Agent Mail database
type DB struct {
	path string
	db   *sql.DB
}

// Open opens a read-only connection to the agent mail database
func Open(path string) (*DB, error) {
	// Open in read-only mode
	db, err := sql.Open("sqlite3", path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{path: path, db: db}, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// GetProjects returns all projects
func (d *DB) GetProjects() ([]Project, error) {
	rows, err := d.db.Query(`
		SELECT id, slug, human_key, created_at
		FROM projects
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		var createdAt string
		if err := rows.Scan(&p.ID, &p.Slug, &p.HumanKey, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		projects = append(projects, p)
	}
	return projects, nil
}

// GetAgents returns all agents with computed fields
func (d *DB) GetAgents() ([]Agent, error) {
	rows, err := d.db.Query(`
		SELECT
			a.id, a.project_id, a.name, a.program, a.model,
			a.task_description, a.inception_ts, a.last_active_ts,
			a.attachments_policy, a.contact_policy,
			p.human_key,
			(SELECT COUNT(*) FROM message_recipients mr
			 JOIN messages m ON mr.message_id = m.id
			 WHERE mr.agent_id = a.id) as inbox_count,
			(SELECT COUNT(*) FROM message_recipients mr
			 JOIN messages m ON mr.message_id = m.id
			 WHERE mr.agent_id = a.id AND mr.read_ts IS NULL) as unread_count
		FROM agents a
		JOIN projects p ON a.project_id = p.id
		ORDER BY a.last_active_ts DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		var inceptionTS, lastActiveTS string
		if err := rows.Scan(
			&a.ID, &a.ProjectID, &a.Name, &a.Program, &a.Model,
			&a.TaskDescription, &inceptionTS, &lastActiveTS,
			&a.AttachmentsPolicy, &a.ContactPolicy,
			&a.ProjectPath, &a.InboxCount, &a.UnreadCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}
		a.InceptionTS, _ = time.Parse(time.RFC3339Nano, inceptionTS)
		a.LastActiveTS, _ = time.Parse(time.RFC3339Nano, lastActiveTS)
		agents = append(agents, a)
	}
	return agents, nil
}

// GetAgentsByProject returns agents for a specific project path
func (d *DB) GetAgentsByProject(projectPath string) ([]Agent, error) {
	rows, err := d.db.Query(`
		SELECT
			a.id, a.project_id, a.name, a.program, a.model,
			a.task_description, a.inception_ts, a.last_active_ts,
			a.attachments_policy, a.contact_policy,
			p.human_key,
			(SELECT COUNT(*) FROM message_recipients mr
			 JOIN messages m ON mr.message_id = m.id
			 WHERE mr.agent_id = a.id) as inbox_count,
			(SELECT COUNT(*) FROM message_recipients mr
			 JOIN messages m ON mr.message_id = m.id
			 WHERE mr.agent_id = a.id AND mr.read_ts IS NULL) as unread_count
		FROM agents a
		JOIN projects p ON a.project_id = p.id
		WHERE p.human_key = ?
		ORDER BY a.last_active_ts DESC
	`, projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		var inceptionTS, lastActiveTS string
		if err := rows.Scan(
			&a.ID, &a.ProjectID, &a.Name, &a.Program, &a.Model,
			&a.TaskDescription, &inceptionTS, &lastActiveTS,
			&a.AttachmentsPolicy, &a.ContactPolicy,
			&a.ProjectPath, &a.InboxCount, &a.UnreadCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}
		a.InceptionTS, _ = time.Parse(time.RFC3339Nano, inceptionTS)
		a.LastActiveTS, _ = time.Parse(time.RFC3339Nano, lastActiveTS)
		agents = append(agents, a)
	}
	return agents, nil
}

// GetAgent returns a single agent by name
func (d *DB) GetAgent(name string) (*Agent, error) {
	var a Agent
	var inceptionTS, lastActiveTS string
	err := d.db.QueryRow(`
		SELECT
			a.id, a.project_id, a.name, a.program, a.model,
			a.task_description, a.inception_ts, a.last_active_ts,
			a.attachments_policy, a.contact_policy,
			p.human_key,
			(SELECT COUNT(*) FROM message_recipients mr WHERE mr.agent_id = a.id) as inbox_count,
			(SELECT COUNT(*) FROM message_recipients mr WHERE mr.agent_id = a.id AND mr.read_ts IS NULL) as unread_count
		FROM agents a
		JOIN projects p ON a.project_id = p.id
		WHERE a.name = ?
	`, name).Scan(
		&a.ID, &a.ProjectID, &a.Name, &a.Program, &a.Model,
		&a.TaskDescription, &inceptionTS, &lastActiveTS,
		&a.AttachmentsPolicy, &a.ContactPolicy,
		&a.ProjectPath, &a.InboxCount, &a.UnreadCount,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query agent: %w", err)
	}
	a.InceptionTS, _ = time.Parse(time.RFC3339Nano, inceptionTS)
	a.LastActiveTS, _ = time.Parse(time.RFC3339Nano, lastActiveTS)
	return &a, nil
}

// GetMessagesForAgent returns messages where the agent is a recipient
func (d *DB) GetMessagesForAgent(agentID int, limit int) ([]Message, error) {
	rows, err := d.db.Query(`
		SELECT
			m.id, m.project_id, m.sender_id, m.thread_id, m.subject,
			m.body_md, m.importance, m.ack_required, m.created_ts,
			s.name as sender_name
		FROM messages m
		JOIN message_recipients mr ON m.id = mr.message_id
		JOIN agents s ON m.sender_id = s.id
		WHERE mr.agent_id = ?
		ORDER BY m.created_ts DESC
		LIMIT ?
	`, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var createdTS string
		var threadID sql.NullString
		if err := rows.Scan(
			&m.ID, &m.ProjectID, &m.SenderID, &threadID, &m.Subject,
			&m.BodyMD, &m.Importance, &m.AckRequired, &createdTS,
			&m.SenderName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		m.CreatedTS, _ = time.Parse(time.RFC3339Nano, createdTS)
		if threadID.Valid {
			m.ThreadID = threadID.String
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// GetActiveReservations returns non-expired, non-released file reservations
func (d *DB) GetActiveReservations() ([]FileReservation, error) {
	rows, err := d.db.Query(`
		SELECT
			fr.id, fr.project_id, fr.agent_id, fr.path_pattern,
			fr.exclusive, fr.reason, fr.created_ts, fr.expires_ts,
			a.name as agent_name
		FROM file_reservations fr
		JOIN agents a ON fr.agent_id = a.id
		WHERE fr.released_ts IS NULL
		  AND fr.expires_ts > datetime('now')
		ORDER BY fr.created_ts DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query reservations: %w", err)
	}
	defer rows.Close()

	var reservations []FileReservation
	for rows.Next() {
		var fr FileReservation
		var createdTS, expiresTS string
		if err := rows.Scan(
			&fr.ID, &fr.ProjectID, &fr.AgentID, &fr.PathPattern,
			&fr.Exclusive, &fr.Reason, &createdTS, &expiresTS,
			&fr.AgentName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan reservation: %w", err)
		}
		fr.CreatedTS, _ = time.Parse(time.RFC3339Nano, createdTS)
		fr.ExpiresTS, _ = time.Parse(time.RFC3339Nano, expiresTS)
		fr.IsActive = true
		reservations = append(reservations, fr)
	}
	return reservations, nil
}

// GetReservationsForAgent returns file reservations for a specific agent
func (d *DB) GetReservationsForAgent(agentID int) ([]FileReservation, error) {
	rows, err := d.db.Query(`
		SELECT
			fr.id, fr.project_id, fr.agent_id, fr.path_pattern,
			fr.exclusive, fr.reason, fr.created_ts, fr.expires_ts, fr.released_ts
		FROM file_reservations fr
		WHERE fr.agent_id = ?
		ORDER BY fr.created_ts DESC
		LIMIT 50
	`, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query reservations: %w", err)
	}
	defer rows.Close()

	var reservations []FileReservation
	for rows.Next() {
		var fr FileReservation
		var createdTS, expiresTS string
		var releasedTS sql.NullString
		if err := rows.Scan(
			&fr.ID, &fr.ProjectID, &fr.AgentID, &fr.PathPattern,
			&fr.Exclusive, &fr.Reason, &createdTS, &expiresTS, &releasedTS,
		); err != nil {
			return nil, fmt.Errorf("failed to scan reservation: %w", err)
		}
		fr.CreatedTS, _ = time.Parse(time.RFC3339Nano, createdTS)
		fr.ExpiresTS, _ = time.Parse(time.RFC3339Nano, expiresTS)
		if releasedTS.Valid {
			t, _ := time.Parse(time.RFC3339Nano, releasedTS.String)
			fr.ReleasedTS = &t
		}
		fr.IsActive = fr.ReleasedTS == nil && time.Now().Before(fr.ExpiresTS)
		reservations = append(reservations, fr)
	}
	return reservations, nil
}

// Reader wraps DB with a default database path
type Reader struct {
	dbPath string
}

// DefaultDBPath is the default location for the MCP Agent Mail database
const DefaultDBPath = "/root/mcp_agent_mail/storage.sqlite3"

// NewReader creates a new reader with the default database path
func NewReader() *Reader {
	return &Reader{dbPath: DefaultDBPath}
}

// NewReaderWithPath creates a reader with a custom database path
func NewReaderWithPath(path string) *Reader {
	return &Reader{dbPath: path}
}

// GetAllAgents returns all agents from the database
func (r *Reader) GetAllAgents() ([]Agent, error) {
	db, err := Open(r.dbPath)
	if err != nil {
		slog.Debug("failed to open agent mail db", "error", err)
		return nil, err
	}
	defer db.Close()
	return db.GetAgents()
}

// GetAgent returns a single agent by name
func (r *Reader) GetAgent(name string) (*Agent, error) {
	db, err := Open(r.dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return db.GetAgent(name)
}

// GetAgentMessages returns recent messages for an agent
func (r *Reader) GetAgentMessages(agentID int, limit int) ([]Message, error) {
	db, err := Open(r.dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return db.GetMessagesForAgent(agentID, limit)
}

// GetAgentReservations returns file reservations for an agent
func (r *Reader) GetAgentReservations(agentID int) ([]FileReservation, error) {
	db, err := Open(r.dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return db.GetReservationsForAgent(agentID)
}

// GetActiveReservations returns all active file reservations
func (r *Reader) GetActiveReservations() ([]FileReservation, error) {
	db, err := Open(r.dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return db.GetActiveReservations()
}

// IsAvailable checks if the database exists and is accessible
func (r *Reader) IsAvailable() bool {
	db, err := Open(r.dbPath)
	if err != nil {
		return false
	}
	db.Close()
	return true
}
