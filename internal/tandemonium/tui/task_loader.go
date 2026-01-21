package tui

import (
	"database/sql"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

type TaskItem struct {
	ID           string
	Title        string
	Status       string
	SessionState string
}

func LoadTasks(db *sql.DB) ([]TaskItem, error) {
	rows, err := db.Query(`
SELECT
  t.id,
  t.title,
  t.status,
  COALESCE((
    SELECT s.state FROM sessions s
    WHERE s.task_id = t.id
    ORDER BY rowid DESC
    LIMIT 1
  ), '') AS session_state
FROM tasks t
ORDER BY t.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskItem
	for rows.Next() {
		var item TaskItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Status, &item.SessionState); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func LoadTasksFromProject() ([]TaskItem, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return nil, err
	}
	db, err := storage.OpenShared(project.StateDBPath(root))
	if err != nil {
		return nil, err
	}
	if err := storage.Migrate(db); err != nil {
		return nil, err
	}
	return LoadTasks(db)
}
