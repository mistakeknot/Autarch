package events

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/contract"
	"gopkg.in/yaml.v3"
)

// ReconcileSummary reports reconciliation activity for a project.
type ReconcileSummary struct {
	SpecsSeen        int
	SpecsEmitted     int
	TasksSeen        int
	TaskEventsEmitted int
	Conflicts        int
}

// ReconcileProject scans file-first sources and emits derived events.
func ReconcileProject(root string, store *Store) (*ReconcileSummary, error) {
	summary := &ReconcileSummary{}

	specWriter := NewWriter(store, SourceGurgeh)
	specWriter.SetProjectPath(root)
	taskWriter := NewWriter(store, SourceColdwine)
	taskWriter.SetProjectPath(root)

	if err := reconcileSpecs(root, store, specWriter, summary); err != nil {
		return summary, err
	}
	if err := reconcileTasks(root, store, taskWriter, summary); err != nil {
		return summary, err
	}

	return summary, nil
}

type specDoc struct {
	ID        string `yaml:"id"`
	Title     string `yaml:"title"`
	Type      string `yaml:"type,omitempty"`
	Status    string `yaml:"status"`
	Version   int    `yaml:"version,omitempty"`
	CreatedAt string `yaml:"created_at,omitempty"`
}

func reconcileSpecs(root string, store *Store, writer *Writer, summary *ReconcileSummary) error {
	specDir := filepath.Join(root, ".gurgeh", "specs")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(specDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var doc specDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			continue
		}

		specID := doc.ID
		if specID == "" {
			specID = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}
		updatedAt := info.ModTime()

		fingerprint := hashBytes(data)
		summary.SpecsSeen++

		cursor, err := store.GetCursor(root, EntitySpec, specID)
		if err != nil {
			return err
		}

		if cursor != nil {
			if doc.Version > 0 && cursor.Version > 0 {
				if doc.Version < cursor.Version {
					if err := store.LogConflict(&ReconcileConflict{
						ProjectPath: root,
						EntityType:  EntitySpec,
						EntityID:    specID,
						Reason:      "spec_version_regression",
						Details: map[string]interface{}{
							"path":            path,
							"file_version":    doc.Version,
							"cursor_version":  cursor.Version,
							"file_fingerprint": fingerprint,
						},
					}); err != nil {
						return err
					}
					summary.Conflicts++
					continue
				}
				if doc.Version == cursor.Version && fingerprint != cursor.Fingerprint {
					if err := store.LogConflict(&ReconcileConflict{
						ProjectPath: root,
						EntityType:  EntitySpec,
						EntityID:    specID,
						Reason:      "spec_version_mismatch",
						Details: map[string]interface{}{
							"path":             path,
							"version":          doc.Version,
							"file_fingerprint": fingerprint,
							"cursor_fingerprint": cursor.Fingerprint,
						},
					}); err != nil {
						return err
					}
					summary.Conflicts++
					continue
				}
			}

			if doc.Version == 0 && cursor.Version == 0 && !cursor.UpdatedAt.IsZero() && updatedAt.Before(cursor.UpdatedAt) && fingerprint != cursor.Fingerprint {
				if err := store.LogConflict(&ReconcileConflict{
					ProjectPath: root,
					EntityType:  EntitySpec,
					EntityID:    specID,
					Reason:      "spec_mtime_regression",
					Details: map[string]interface{}{
						"path":             path,
						"file_mtime":       updatedAt.Format(time.RFC3339Nano),
						"cursor_mtime":     cursor.UpdatedAt.Format(time.RFC3339Nano),
						"file_fingerprint": fingerprint,
					},
				}); err != nil {
					return err
				}
				summary.Conflicts++
				continue
			}

			if fingerprint == cursor.Fingerprint {
				continue
			}
		}

		snapshot := SpecSnapshot{
			ID:        specID,
			Title:     doc.Title,
			Status:    doc.Status,
			Type:      doc.Type,
			Version:   doc.Version,
			Path:      path,
			UpdatedAt: updatedAt,
		}
		if err := writer.EmitSpecRevised(snapshot); err != nil {
			return err
		}
		summary.SpecsEmitted++

		if err := store.UpsertCursor(&ReconcileCursor{
			ProjectPath: root,
			EntityType:  EntitySpec,
			EntityID:    specID,
			Fingerprint: fingerprint,
			Status:      doc.Status,
			Version:     doc.Version,
			UpdatedAt:   updatedAt,
		}); err != nil {
			return err
		}
	}

	return nil
}

type taskDoc struct {
	ID          string `yaml:"id"`
	StoryID     string `yaml:"story_id,omitempty"`
	Title       string `yaml:"title"`
	Description string `yaml:"description,omitempty"`
	Status      string `yaml:"status"`
	Priority    int    `yaml:"priority,omitempty"`
	Assignee    string `yaml:"assignee,omitempty"`
	WorktreeRef string `yaml:"worktree_ref,omitempty"`
	SessionRef  string `yaml:"session_ref,omitempty"`
	BlockReason string `yaml:"block_reason,omitempty"`
	CreatedAt   string `yaml:"created_at,omitempty"`
	UpdatedAt   string `yaml:"updated_at,omitempty"`
}

func reconcileTasks(root string, store *Store, writer *Writer, summary *ReconcileSummary) error {
	tasksDir := filepath.Join(root, ".coldwine", "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(tasksDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var doc taskDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			continue
		}

		taskID := doc.ID
		if taskID == "" {
			taskID = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}
		updatedAt := info.ModTime()

		fingerprint := hashBytes(data)
		summary.TasksSeen++

		cursor, err := store.GetCursor(root, EntityTask, taskID)
		if err != nil {
			return err
		}

		status := strings.ToLower(doc.Status)
		if status == "" {
			status = "todo"
		}

		if cursor != nil {
			prevStatus := strings.ToLower(cursor.Status)
			if prevStatus == "done" || prevStatus == "completed" {
				if status != "done" && status != "completed" {
					if err := store.LogConflict(&ReconcileConflict{
						ProjectPath: root,
						EntityType:  EntityTask,
						EntityID:    taskID,
						Reason:      "task_status_regression",
						Details: map[string]interface{}{
							"path":          path,
							"prev_status":   cursor.Status,
							"next_status":   status,
							"fingerprint":   fingerprint,
						},
					}); err != nil {
						return err
					}
					summary.Conflicts++
					continue
				}
			}

			if fingerprint == cursor.Fingerprint {
				continue
			}
		}

		task := contract.Task{
			ID:          taskID,
			StoryID:     doc.StoryID,
			Title:       doc.Title,
			Description: doc.Description,
			Status:      mapTaskStatus(status),
			Priority:    doc.Priority,
			Assignee:    doc.Assignee,
			WorktreeRef: doc.WorktreeRef,
			SessionRef:  doc.SessionRef,
			SourceTool:  SourceColdwine,
			CreatedAt:   parseTimeOr(updatedAt, doc.CreatedAt),
			UpdatedAt:   parseTimeOr(updatedAt, doc.UpdatedAt),
		}

		if cursor == nil {
			if err := writer.EmitTaskCreated(&task); err != nil {
				return err
			}
			summary.TaskEventsEmitted++
			if status == "in_progress" {
				if err := writer.EmitTaskStarted(taskID); err != nil {
					return err
				}
				summary.TaskEventsEmitted++
			} else if status == "blocked" {
				if err := writer.EmitTaskBlocked(taskID, doc.BlockReason); err != nil {
					return err
				}
				summary.TaskEventsEmitted++
			} else if status == "done" || status == "completed" {
				if err := writer.EmitTaskCompleted(taskID); err != nil {
					return err
				}
				summary.TaskEventsEmitted++
			}
		} else {
			if cursor.Status != status {
				switch status {
				case "in_progress":
					if err := writer.EmitTaskStarted(taskID); err != nil {
						return err
					}
					summary.TaskEventsEmitted++
				case "blocked":
					if err := writer.EmitTaskBlocked(taskID, doc.BlockReason); err != nil {
						return err
					}
					summary.TaskEventsEmitted++
				case "done", "completed":
					if err := writer.EmitTaskCompleted(taskID); err != nil {
						return err
					}
					summary.TaskEventsEmitted++
				}
			}
		}

		if err := store.UpsertCursor(&ReconcileCursor{
			ProjectPath: root,
			EntityType:  EntityTask,
			EntityID:    taskID,
			Fingerprint: fingerprint,
			Status:      status,
			Version:     0,
			UpdatedAt:   updatedAt,
		}); err != nil {
			return err
		}
	}

	return nil
}

func parseTimeOr(fallback time.Time, value string) time.Time {
	if value == "" {
		return fallback
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return fallback
	}
	return t
}

func mapTaskStatus(status string) contract.TaskStatus {
	switch status {
	case "in_progress", "working":
		return contract.TaskStatusInProgress
	case "blocked":
		return contract.TaskStatusBlocked
	case "done", "completed":
		return contract.TaskStatusDone
	case "pending", "todo":
		return contract.TaskStatusTodo
	default:
		return contract.TaskStatusTodo
	}
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
