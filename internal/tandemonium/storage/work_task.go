package storage

import (
	"database/sql"
	"time"
)

// InsertWorkTask inserts a new work task
func InsertWorkTask(db *sql.DB, t WorkTask) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO work_tasks (id, story_id, title, description, status, priority, assignee, worktree_ref, session_ref, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.StoryID, t.Title, t.Description, t.Status, t.Priority, t.Assignee, t.WorktreeRef, t.SessionRef, now, now)
	return err
}

// GetWorkTask retrieves a work task by ID
func GetWorkTask(db *sql.DB, id string) (WorkTask, error) {
	row := db.QueryRow(`
		SELECT id, story_id, title, description, status, priority, assignee, worktree_ref, session_ref, created_at, updated_at
		FROM work_tasks WHERE id = ?`, id)

	var t WorkTask
	var desc, assignee, worktreeRef, sessionRef sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&t.ID, &t.StoryID, &t.Title, &desc, &t.Status, &t.Priority, &assignee, &worktreeRef, &sessionRef, &createdAt, &updatedAt); err != nil {
		return WorkTask{}, err
	}
	t.Description = desc.String
	t.Assignee = assignee.String
	t.WorktreeRef = worktreeRef.String
	t.SessionRef = sessionRef.String
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return t, nil
}

// ListWorkTasksByStory returns tasks for a story
func ListWorkTasksByStory(db *sql.DB, storyID string) ([]WorkTask, error) {
	rows, err := db.Query(`
		SELECT id, story_id, title, description, status, priority, assignee, worktree_ref, session_ref, created_at, updated_at
		FROM work_tasks WHERE story_id = ? ORDER BY priority ASC, created_at DESC`, storyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []WorkTask
	for rows.Next() {
		var t WorkTask
		var desc, assignee, worktreeRef, sessionRef sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&t.ID, &t.StoryID, &t.Title, &desc, &t.Status, &t.Priority, &assignee, &worktreeRef, &sessionRef, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		t.Description = desc.String
		t.Assignee = assignee.String
		t.WorktreeRef = worktreeRef.String
		t.SessionRef = sessionRef.String
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListWorkTasksByAssignee returns tasks assigned to an agent/user
func ListWorkTasksByAssignee(db *sql.DB, assignee string) ([]WorkTask, error) {
	rows, err := db.Query(`
		SELECT id, story_id, title, description, status, priority, assignee, worktree_ref, session_ref, created_at, updated_at
		FROM work_tasks WHERE assignee = ? ORDER BY priority ASC`, assignee)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []WorkTask
	for rows.Next() {
		var t WorkTask
		var desc, ass, worktreeRef, sessionRef sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&t.ID, &t.StoryID, &t.Title, &desc, &t.Status, &t.Priority, &ass, &worktreeRef, &sessionRef, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		t.Description = desc.String
		t.Assignee = ass.String
		t.WorktreeRef = worktreeRef.String
		t.SessionRef = sessionRef.String
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListWorkTasksByStatus returns tasks with a given status
func ListWorkTasksByStatus(db *sql.DB, status TaskStatus) ([]WorkTask, error) {
	rows, err := db.Query(`
		SELECT id, story_id, title, description, status, priority, assignee, worktree_ref, session_ref, created_at, updated_at
		FROM work_tasks WHERE status = ? ORDER BY priority ASC`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []WorkTask
	for rows.Next() {
		var t WorkTask
		var desc, assignee, worktreeRef, sessionRef sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&t.ID, &t.StoryID, &t.Title, &desc, &t.Status, &t.Priority, &assignee, &worktreeRef, &sessionRef, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		t.Description = desc.String
		t.Assignee = assignee.String
		t.WorktreeRef = worktreeRef.String
		t.SessionRef = sessionRef.String
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// UpdateWorkTaskStatus updates a task's status
func UpdateWorkTaskStatus(db *sql.DB, id string, status TaskStatus) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE work_tasks SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
	return err
}

// AssignWorkTask assigns a task to an agent/user
func AssignWorkTask(db *sql.DB, id, assignee string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE work_tasks SET assignee = ?, updated_at = ? WHERE id = ?`, assignee, now, id)
	return err
}

// LinkWorkTaskToSession links a task to an agent session
func LinkWorkTaskToSession(db *sql.DB, taskID, sessionRef string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE work_tasks SET session_ref = ?, updated_at = ? WHERE id = ?`, sessionRef, now, taskID)
	return err
}

// LinkWorkTaskToWorktree links a task to a git worktree
func LinkWorkTaskToWorktree(db *sql.DB, taskID, worktreeRef string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE work_tasks SET worktree_ref = ?, updated_at = ? WHERE id = ?`, worktreeRef, now, taskID)
	return err
}

// DeleteWorkTask deletes a task
func DeleteWorkTask(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM work_tasks WHERE id = ?`, id)
	return err
}
