package storage

import (
	"database/sql"
	"time"
)

// InsertAgentSession inserts a new agent session
func InsertAgentSession(db *sql.DB, s AgentSession) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO agent_sessions (id, task_id, agent_name, agent_program, state, worktree_path, last_active_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.TaskID, s.AgentName, s.AgentProgram, s.State, s.WorktreePath, now, now)
	return err
}

// GetAgentSession retrieves an agent session by ID
func GetAgentSession(db *sql.DB, id string) (AgentSession, error) {
	row := db.QueryRow(`
		SELECT id, task_id, agent_name, agent_program, state, worktree_path, last_active_at, created_at
		FROM agent_sessions WHERE id = ?`, id)

	var s AgentSession
	var worktreePath sql.NullString
	var lastActiveAt, createdAt string
	if err := row.Scan(&s.ID, &s.TaskID, &s.AgentName, &s.AgentProgram, &s.State, &worktreePath, &lastActiveAt, &createdAt); err != nil {
		return AgentSession{}, err
	}
	s.WorktreePath = worktreePath.String
	s.LastActiveAt, _ = time.Parse(time.RFC3339, lastActiveAt)
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return s, nil
}

// GetAgentSessionByTask retrieves the active session for a task
func GetAgentSessionByTask(db *sql.DB, taskID string) (AgentSession, error) {
	row := db.QueryRow(`
		SELECT id, task_id, agent_name, agent_program, state, worktree_path, last_active_at, created_at
		FROM agent_sessions WHERE task_id = ? AND state != 'done' ORDER BY created_at DESC LIMIT 1`, taskID)

	var s AgentSession
	var worktreePath sql.NullString
	var lastActiveAt, createdAt string
	if err := row.Scan(&s.ID, &s.TaskID, &s.AgentName, &s.AgentProgram, &s.State, &worktreePath, &lastActiveAt, &createdAt); err != nil {
		return AgentSession{}, err
	}
	s.WorktreePath = worktreePath.String
	s.LastActiveAt, _ = time.Parse(time.RFC3339, lastActiveAt)
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return s, nil
}

// ListAgentSessionsByAgent returns all sessions for an agent
func ListAgentSessionsByAgent(db *sql.DB, agentName string) ([]AgentSession, error) {
	rows, err := db.Query(`
		SELECT id, task_id, agent_name, agent_program, state, worktree_path, last_active_at, created_at
		FROM agent_sessions WHERE agent_name = ? ORDER BY created_at DESC`, agentName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []AgentSession
	for rows.Next() {
		var s AgentSession
		var worktreePath sql.NullString
		var lastActiveAt, createdAt string
		if err := rows.Scan(&s.ID, &s.TaskID, &s.AgentName, &s.AgentProgram, &s.State, &worktreePath, &lastActiveAt, &createdAt); err != nil {
			return nil, err
		}
		s.WorktreePath = worktreePath.String
		s.LastActiveAt, _ = time.Parse(time.RFC3339, lastActiveAt)
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// ListActiveAgentSessions returns all active (non-done) sessions
func ListActiveAgentSessions(db *sql.DB) ([]AgentSession, error) {
	rows, err := db.Query(`
		SELECT id, task_id, agent_name, agent_program, state, worktree_path, last_active_at, created_at
		FROM agent_sessions WHERE state != 'done' ORDER BY last_active_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []AgentSession
	for rows.Next() {
		var s AgentSession
		var worktreePath sql.NullString
		var lastActiveAt, createdAt string
		if err := rows.Scan(&s.ID, &s.TaskID, &s.AgentName, &s.AgentProgram, &s.State, &worktreePath, &lastActiveAt, &createdAt); err != nil {
			return nil, err
		}
		s.WorktreePath = worktreePath.String
		s.LastActiveAt, _ = time.Parse(time.RFC3339, lastActiveAt)
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// UpdateAgentSessionState updates a session's state
func UpdateAgentSessionState(db *sql.DB, id, state string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE agent_sessions SET state = ?, last_active_at = ? WHERE id = ?`, state, now, id)
	return err
}

// TouchAgentSession updates the last_active_at timestamp
func TouchAgentSession(db *sql.DB, id string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE agent_sessions SET last_active_at = ? WHERE id = ?`, now, id)
	return err
}

// DeleteAgentSession deletes a session
func DeleteAgentSession(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM agent_sessions WHERE id = ?`, id)
	return err
}
