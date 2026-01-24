package storage

import (
	"database/sql"
	"time"
)

// InsertWorktree inserts a new worktree record
func InsertWorktree(db *sql.DB, w Worktree) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO worktrees (id, task_id, path, branch, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.TaskID, w.Path, w.Branch, w.Status, now, now)
	return err
}

// GetWorktree retrieves a worktree by ID
func GetWorktree(db *sql.DB, id string) (Worktree, error) {
	row := db.QueryRow(`
		SELECT id, task_id, path, branch, status, created_at, updated_at
		FROM worktrees WHERE id = ?`, id)

	var w Worktree
	var createdAt, updatedAt string
	if err := row.Scan(&w.ID, &w.TaskID, &w.Path, &w.Branch, &w.Status, &createdAt, &updatedAt); err != nil {
		return Worktree{}, err
	}
	w.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	w.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return w, nil
}

// GetWorktreeByPath retrieves a worktree by its filesystem path
func GetWorktreeByPath(db *sql.DB, path string) (Worktree, error) {
	row := db.QueryRow(`
		SELECT id, task_id, path, branch, status, created_at, updated_at
		FROM worktrees WHERE path = ?`, path)

	var w Worktree
	var createdAt, updatedAt string
	if err := row.Scan(&w.ID, &w.TaskID, &w.Path, &w.Branch, &w.Status, &createdAt, &updatedAt); err != nil {
		return Worktree{}, err
	}
	w.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	w.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return w, nil
}

// GetWorktreeByTask retrieves the worktree for a task
func GetWorktreeByTask(db *sql.DB, taskID string) (Worktree, error) {
	row := db.QueryRow(`
		SELECT id, task_id, path, branch, status, created_at, updated_at
		FROM worktrees WHERE task_id = ? AND status = 'active'`, taskID)

	var w Worktree
	var createdAt, updatedAt string
	if err := row.Scan(&w.ID, &w.TaskID, &w.Path, &w.Branch, &w.Status, &createdAt, &updatedAt); err != nil {
		return Worktree{}, err
	}
	w.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	w.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return w, nil
}

// ListActiveWorktrees returns all active worktrees
func ListActiveWorktrees(db *sql.DB) ([]Worktree, error) {
	rows, err := db.Query(`
		SELECT id, task_id, path, branch, status, created_at, updated_at
		FROM worktrees WHERE status = 'active' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var worktrees []Worktree
	for rows.Next() {
		var w Worktree
		var createdAt, updatedAt string
		if err := rows.Scan(&w.ID, &w.TaskID, &w.Path, &w.Branch, &w.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		w.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		worktrees = append(worktrees, w)
	}
	return worktrees, nil
}

// UpdateWorktreeStatus updates a worktree's status
func UpdateWorktreeStatus(db *sql.DB, id, status string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE worktrees SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
	return err
}

// DeleteWorktree deletes a worktree record
func DeleteWorktree(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM worktrees WHERE id = ?`, id)
	return err
}
