package storage

import (
	"database/sql"
	"time"
)

// InsertStory inserts a new story
func InsertStory(db *sql.DB, s Story) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO stories (id, epic_id, title, description, status, priority, complexity, assignee, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.EpicID, s.Title, s.Description, s.Status, s.Priority, s.Complexity, s.Assignee, now, now)
	return err
}

// GetStory retrieves a story by ID
func GetStory(db *sql.DB, id string) (Story, error) {
	row := db.QueryRow(`
		SELECT id, epic_id, title, description, status, priority, complexity, assignee, created_at, updated_at
		FROM stories WHERE id = ?`, id)

	var s Story
	var desc, assignee sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&s.ID, &s.EpicID, &s.Title, &desc, &s.Status, &s.Priority, &s.Complexity, &assignee, &createdAt, &updatedAt); err != nil {
		return Story{}, err
	}
	s.Description = desc.String
	s.Assignee = assignee.String
	s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return s, nil
}

// ListStoriesByEpic returns stories for an epic
func ListStoriesByEpic(db *sql.DB, epicID string) ([]Story, error) {
	rows, err := db.Query(`
		SELECT id, epic_id, title, description, status, priority, complexity, assignee, created_at, updated_at
		FROM stories WHERE epic_id = ? ORDER BY priority ASC, created_at DESC`, epicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stories []Story
	for rows.Next() {
		var s Story
		var desc, assignee sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.EpicID, &s.Title, &desc, &s.Status, &s.Priority, &s.Complexity, &assignee, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		s.Description = desc.String
		s.Assignee = assignee.String
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		stories = append(stories, s)
	}
	return stories, nil
}

// ListStoriesByAssignee returns stories assigned to an agent/user
func ListStoriesByAssignee(db *sql.DB, assignee string) ([]Story, error) {
	rows, err := db.Query(`
		SELECT id, epic_id, title, description, status, priority, complexity, assignee, created_at, updated_at
		FROM stories WHERE assignee = ? ORDER BY priority ASC`, assignee)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stories []Story
	for rows.Next() {
		var s Story
		var desc, ass sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&s.ID, &s.EpicID, &s.Title, &desc, &s.Status, &s.Priority, &s.Complexity, &ass, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		s.Description = desc.String
		s.Assignee = ass.String
		s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		stories = append(stories, s)
	}
	return stories, nil
}

// UpdateStoryStatus updates a story's status
func UpdateStoryStatus(db *sql.DB, id string, status StoryStatus) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE stories SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
	return err
}

// AssignStory assigns a story to an agent/user
func AssignStory(db *sql.DB, id, assignee string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE stories SET assignee = ?, updated_at = ? WHERE id = ?`, assignee, now, id)
	return err
}

// DeleteStory deletes a story and all its tasks (via cascade)
func DeleteStory(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM stories WHERE id = ?`, id)
	return err
}
