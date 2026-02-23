// Package local provides a file-based DataSource that reads from local
// dot-directories (.gurgeh/, .tandemonium/, .pollard/) without requiring
// the Intermute HTTP server. Used as a fallback when Intermute is unreachable.
package local

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
	"github.com/mistakeknot/autarch/internal/pollard/insights"
	"github.com/mistakeknot/autarch/pkg/autarch"
	autarchdb "github.com/mistakeknot/autarch/pkg/db"
)

// allowedTables is the closed set of tables hasTable() may query.
// PRAGMA does not support parameterized arguments, so we validate against this allowlist.
var allowedTables = map[string]bool{
	"epics":      true,
	"stories":    true,
	"work_tasks": true,
}

// LocalSource implements autarch.DataSource by reading local project files.
type LocalSource struct {
	projectPath string
}

// NewLocalSource creates a LocalSource rooted at the given project directory.
func NewLocalSource(projectPath string) *LocalSource {
	return &LocalSource{projectPath: projectPath}
}

// ListSpecs reads PRDs from .gurgeh/specs/*.yaml (handles .praude/ legacy).
func (s *LocalSource) ListSpecs(status string) ([]autarch.Spec, error) {
	prds, err := specs.LoadAllPRDs(s.projectPath)
	if err != nil {
		return nil, err
	}

	var result []autarch.Spec
	for _, prd := range prds {
		spec := mapPRDToSpec(prd)
		if status != "" && string(spec.Status) != status {
			continue
		}
		result = append(result, spec)
	}
	if result == nil {
		result = []autarch.Spec{}
	}
	return result, nil
}

// ListEpics reads epics from .tandemonium/state.db.
// Returns empty slice if DB or epics table doesn't exist (MigrateV2 not applied).
func (s *LocalSource) ListEpics(specID string) ([]autarch.Epic, error) {
	db, err := s.openDB()
	if err != nil {
		if os.IsNotExist(err) {
			return []autarch.Epic{}, nil
		}
		return nil, err
	}
	defer db.Close()

	if !s.hasTable(db, "epics") {
		return []autarch.Epic{}, nil
	}

	query := `SELECT id, feature_ref, title, status, created_at, updated_at FROM epics`
	args := []any{}
	if specID != "" {
		query += ` WHERE feature_ref = ?`
		args = append(args, specID)
	}
	query += ` ORDER BY priority ASC, created_at DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []autarch.Epic
	for rows.Next() {
		var e autarch.Epic
		var featureRef sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&e.ID, &featureRef, &e.Title, &e.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		e.SpecID = featureRef.String // FeatureRef → SpecID (different namespace, documented)
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan epics: %w", err)
	}
	if result == nil {
		result = []autarch.Epic{}
	}
	return result, nil
}

// ListStories reads stories from .tandemonium/state.db.
func (s *LocalSource) ListStories(epicID string) ([]autarch.Story, error) {
	db, err := s.openDB()
	if err != nil {
		if os.IsNotExist(err) {
			return []autarch.Story{}, nil
		}
		return nil, err
	}
	defer db.Close()

	if !s.hasTable(db, "stories") {
		return []autarch.Story{}, nil
	}

	query := `SELECT id, epic_id, title, status, created_at, updated_at FROM stories`
	args := []any{}
	if epicID != "" {
		query += ` WHERE epic_id = ?`
		args = append(args, epicID)
	}
	query += ` ORDER BY priority ASC, created_at DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []autarch.Story
	for rows.Next() {
		var st autarch.Story
		var createdAt, updatedAt string
		if err := rows.Scan(&st.ID, &st.EpicID, &st.Title, &st.Status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		st.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		st.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		result = append(result, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan stories: %w", err)
	}
	if result == nil {
		result = []autarch.Story{}
	}
	return result, nil
}

// ListTasks reads work_tasks from .tandemonium/state.db.
func (s *LocalSource) ListTasks(status, agent string) ([]autarch.Task, error) {
	db, err := s.openDB()
	if err != nil {
		if os.IsNotExist(err) {
			return []autarch.Task{}, nil
		}
		return nil, err
	}
	defer db.Close()

	if !s.hasTable(db, "work_tasks") {
		return []autarch.Task{}, nil
	}

	query := `SELECT id, story_id, title, status, assignee, session_ref, created_at, updated_at FROM work_tasks`
	var conditions []string
	args := []any{}
	if status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, status)
	}
	if agent != "" {
		conditions = append(conditions, "assignee = ?")
		args = append(args, agent)
	}
	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for _, c := range conditions[1:] {
			query += " AND " + c
		}
	}
	query += ` ORDER BY priority ASC, created_at DESC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []autarch.Task
	for rows.Next() {
		var t autarch.Task
		var assignee, sessionRef sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&t.ID, &t.StoryID, &t.Title, &t.Status, &assignee, &sessionRef, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		t.Agent = assignee.String
		t.SessionID = sessionRef.String
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan work_tasks: %w", err)
	}
	if result == nil {
		result = []autarch.Task{}
	}
	return result, nil
}

// ListInsights reads insights from .pollard/insights/*.yaml.
func (s *LocalSource) ListInsights(specID, category string) ([]autarch.Insight, error) {
	localInsights, err := insights.LoadAll(s.projectPath)
	if err != nil {
		return nil, err
	}

	var result []autarch.Insight
	for _, li := range localInsights {
		ai := mapInsightToAutarch(li)
		if specID != "" && ai.SpecID != specID {
			continue
		}
		if category != "" && ai.Category != category {
			continue
		}
		result = append(result, ai)
	}
	if result == nil {
		result = []autarch.Insight{}
	}
	return result, nil
}

// openDB opens the Coldwine state database at .tandemonium/state.db.
// Uses a fresh sql.DB (NOT OpenShared) so we can defer Close().
func (s *LocalSource) openDB() (*sql.DB, error) {
	dbPath := filepath.Join(s.projectPath, ".tandemonium", "state.db")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}
	return autarchdb.Open(dbPath)
}

// hasTable checks whether a table exists via PRAGMA table_info.
// Returns false if MigrateV2 hasn't been applied.
// Table name is validated against allowedTables since PRAGMA doesn't support parameterized args.
func (s *LocalSource) hasTable(db *sql.DB, table string) bool {
	if !allowedTables[table] {
		return false
	}
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next() // true if at least one column exists
}

// mapPRDToSpec converts a local PRD to an autarch.Spec.
// Uses prd.Version as synthetic ID (Intermute would assign a UUID).
// Parses PRD's RFC3339 timestamps instead of using time.Now().
func mapPRDToSpec(prd *specs.PRD) autarch.Spec {
	spec := autarch.Spec{
		ID:    prd.Version, // Synthetic ID from version slug (e.g. "mvp", "v1")
		Title: prd.Title,
		Status: mapPRDStatusToSpecStatus(prd.Status),
	}

	if prd.CreatedAt != "" {
		spec.CreatedAt, _ = time.Parse(time.RFC3339, prd.CreatedAt)
	}
	if prd.UpdatedAt != "" {
		spec.UpdatedAt, _ = time.Parse(time.RFC3339, prd.UpdatedAt)
	}

	return spec
}

// mapPRDStatusToSpecStatus maps Gurgeh PRD status to autarch Spec status.
// Mirrors the mapping in internal/gurgeh/intermute/sync.go.
func mapPRDStatusToSpecStatus(status specs.PRDStatus) autarch.SpecStatus {
	switch status {
	case specs.PRDStatusDraft:
		return autarch.SpecStatusDraft
	case specs.PRDStatusApproved:
		return autarch.SpecStatusResearch
	case specs.PRDStatusInProgress:
		return autarch.SpecStatusValidated
	case specs.PRDStatusDone:
		return autarch.SpecStatusArchived
	default:
		return autarch.SpecStatusDraft
	}
}

// mapInsightToAutarch converts a local Pollard insight to an autarch.Insight.
// Lossy: Sources[] → single Source+URL (takes first), Body unavailable locally.
func mapInsightToAutarch(li *insights.Insight) autarch.Insight {
	ai := autarch.Insight{
		ID:        li.ID,
		Category:  string(li.Category),
		Title:     li.Title,
		CreatedAt: li.CollectedAt,
	}

	// Map first source (lossy — local insights can have multiple sources)
	if len(li.Sources) > 0 {
		ai.Source = li.Sources[0].Type
		ai.URL = li.Sources[0].URL
	} else {
		ai.Source = "local"
	}

	// Map linked features to SpecID (take first if present)
	if len(li.LinkedFeatures) > 0 {
		ai.SpecID = li.LinkedFeatures[0]
	}

	// Body is not available in local insight files — leave empty
	return ai
}
