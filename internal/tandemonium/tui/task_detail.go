package tui

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/specs"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

type TaskDetail struct {
	ID           string
	Title        string
	Status       string
	SessionState string
	Summary      string
	LastLine     string
}

func LoadTaskDetail(taskID string) (TaskDetail, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return TaskDetail{}, err
	}
	db, err := storage.OpenShared(project.StateDBPath(root))
	if err != nil {
		db = nil
	}
	return LoadTaskDetailWithDB(db, taskID)
}

func LoadTaskDetailWithDB(db *sql.DB, taskID string) (TaskDetail, error) {
	root, err := project.FindRoot(".")
	if err != nil {
		return TaskDetail{}, err
	}
	detail := TaskDetail{ID: taskID}
	if specPath, err := project.TaskSpecPath(root, taskID); err == nil {
		if spec, err := specs.LoadDetail(specPath); err == nil {
			if spec.Title != "" {
				detail.Title = spec.Title
			}
			if spec.Summary != "" {
				detail.Summary = spec.Summary
			}
		}
	}
	if db != nil {
		_ = storage.Migrate(db)
		if task, err := storage.GetTask(db, taskID); err == nil {
			if detail.Title == "" {
				detail.Title = task.Title
			}
			detail.Status = task.Status
		}
		if session, err := storage.FindSessionByTask(db, taskID); err == nil {
			detail.SessionState = session.State
			logPath := filepath.Join(project.SessionsDir(root), session.ID+".log")
			detail.LastLine = tailLastLine(logPath)
		}
	}
	if detail.Title == "" {
		detail.Title = taskID
	}
	return detail, nil
}

func tailLastLine(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimRight(string(raw), "\n")
	if text == "" {
		return ""
	}
	parts := strings.Split(text, "\n")
	return parts[len(parts)-1]
}
