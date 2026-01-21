package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type AgentProfile struct {
	Name            string
	Program         string
	Model           string
	TaskDescription string
	CreatedAt       string
	UpdatedAt       string
	LastActiveAt    string
}

func UpsertAgent(db *sql.DB, profile AgentProfile) (AgentProfile, error) {
	if strings.TrimSpace(profile.Name) == "" {
		return AgentProfile{}, errors.New("agent name required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := db.Begin()
	if err != nil {
		return AgentProfile{}, err
	}

	var existing AgentProfile
	row := tx.QueryRow(`SELECT name, program, model, task_description, created_ts, updated_ts, last_active_ts
FROM agents
WHERE name = ?`, profile.Name)
	switch err := row.Scan(&existing.Name, &existing.Program, &existing.Model, &existing.TaskDescription, &existing.CreatedAt, &existing.UpdatedAt, &existing.LastActiveAt); err {
	case nil:
		if _, err := tx.Exec(`UPDATE agents SET program = ?, model = ?, task_description = ?, updated_ts = ?, last_active_ts = ? WHERE name = ?`,
			profile.Program, profile.Model, profile.TaskDescription, now, now, profile.Name); err != nil {
			_ = tx.Rollback()
			return AgentProfile{}, err
		}
		existing.Program = profile.Program
		existing.Model = profile.Model
		existing.TaskDescription = profile.TaskDescription
		existing.UpdatedAt = now
		existing.LastActiveAt = now
		if err := tx.Commit(); err != nil {
			return AgentProfile{}, err
		}
		return existing, nil
	case sql.ErrNoRows:
		if _, err := tx.Exec(`INSERT INTO agents (name, program, model, task_description, created_ts, updated_ts, last_active_ts)
VALUES (?, ?, ?, ?, ?, ?, ?)`, profile.Name, profile.Program, profile.Model, profile.TaskDescription, now, now, now); err != nil {
			_ = tx.Rollback()
			return AgentProfile{}, err
		}
		if err := tx.Commit(); err != nil {
			return AgentProfile{}, err
		}
		profile.CreatedAt = now
		profile.UpdatedAt = now
		profile.LastActiveAt = now
		return profile, nil
	default:
		_ = tx.Rollback()
		return AgentProfile{}, err
	}
}

func GetAgent(db *sql.DB, name string) (AgentProfile, error) {
	if strings.TrimSpace(name) == "" {
		return AgentProfile{}, errors.New("agent name required")
	}
	row := db.QueryRow(`SELECT name, program, model, task_description, created_ts, updated_ts, last_active_ts
FROM agents
WHERE name = ?`, name)
	var agent AgentProfile
	if err := row.Scan(&agent.Name, &agent.Program, &agent.Model, &agent.TaskDescription, &agent.CreatedAt, &agent.UpdatedAt, &agent.LastActiveAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentProfile{}, fmt.Errorf("agent not found")
		}
		return AgentProfile{}, err
	}
	return agent, nil
}
