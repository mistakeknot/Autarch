package prd

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/mistakeknot/autarch/internal/coldwine/project"
	"github.com/mistakeknot/autarch/internal/coldwine/storage"
	"gopkg.in/yaml.v3"
)

// PersistBriefTasksResult describes where imported tasks were persisted.
type PersistBriefTasksResult struct {
	TaskCount   int
	StateDBPath string
	SpecsDir    string
}

// PersistBriefTasks persists imported brief tasks to both state.db and task specs.
func PersistBriefTasks(root string, tasks []storage.WorkTask) (*PersistBriefTasksResult, error) {
	if len(tasks) == 0 {
		return nil, fmt.Errorf("no tasks to persist")
	}

	if err := project.Init(root); err != nil {
		return nil, fmt.Errorf("initialize coldwine project: %w", err)
	}

	dbPath := project.StateDBPath(root)
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open state db: %w", err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate state db: %w", err)
	}

	specsDir := project.SpecsDir(root)
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create specs dir: %w", err)
	}

	for _, t := range tasks {
		if err := upsertTask(db, t); err != nil {
			return nil, fmt.Errorf("persist task %s: %w", t.ID, err)
		}
		if err := writeTaskSpec(specsDir, t); err != nil {
			return nil, fmt.Errorf("write task spec %s: %w", t.ID, err)
		}
	}

	return &PersistBriefTasksResult{
		TaskCount:   len(tasks),
		StateDBPath: dbPath,
		SpecsDir:    specsDir,
	}, nil
}

func upsertTask(db *sql.DB, t storage.WorkTask) error {
	_, err := db.Exec(
		`INSERT INTO tasks (id, title, status)
		 VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET title = excluded.title, status = excluded.status`,
		t.ID, t.Title, string(t.Status),
	)
	return err
}

func writeTaskSpec(specsDir string, t storage.WorkTask) error {
	payload := map[string]any{
		"id":          t.ID,
		"title":       t.Title,
		"description": t.Description,
		"status":      string(t.Status),
		"story_id":    t.StoryID,
		"priority":    t.Priority,
	}
	if t.Assignee != "" {
		payload["assignee"] = t.Assignee
	}
	data, err := yaml.Marshal(payload)
	if err != nil {
		return err
	}
	path, err := project.SafePath(specsDir, t.ID+".yaml")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
