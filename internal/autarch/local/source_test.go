package local

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/coldwine/storage"
	autarchdb "github.com/mistakeknot/autarch/pkg/db"
)

func TestLocalSource_ListSpecs(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, ".gurgeh", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	prdYAML := `id: MVP
title: Test PRD
version: mvp
status: draft
created_at: "2025-06-15T10:00:00Z"
updated_at: "2025-06-16T12:00:00Z"
features: []
`
	if err := os.WriteFile(filepath.Join(specsDir, "mvp.yaml"), []byte(prdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(dir)
	specs, err := src.ListSpecs("")
	if err != nil {
		t.Fatalf("ListSpecs: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}

	spec := specs[0]
	if spec.ID != "mvp" {
		t.Errorf("ID = %q, want %q", spec.ID, "mvp")
	}
	if spec.Title != "Test PRD" {
		t.Errorf("Title = %q, want %q", spec.Title, "Test PRD")
	}
	if string(spec.Status) != "draft" {
		t.Errorf("Status = %q, want %q", spec.Status, "draft")
	}
	// Verify RFC3339 timestamps are parsed, not time.Now()
	if spec.CreatedAt.Year() != 2025 {
		t.Errorf("CreatedAt year = %d, want 2025", spec.CreatedAt.Year())
	}
	if spec.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestLocalSource_ListSpecs_Legacy(t *testing.T) {
	dir := t.TempDir()
	// Use .praude (legacy) instead of .gurgeh
	specsDir := filepath.Join(dir, ".praude", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	prdYAML := `id: V1
title: Legacy PRD
version: v1
status: approved
created_at: "2025-01-01T00:00:00Z"
features: []
`
	if err := os.WriteFile(filepath.Join(specsDir, "v1.yaml"), []byte(prdYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(dir)
	specs, err := src.ListSpecs("")
	if err != nil {
		t.Fatalf("ListSpecs: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec from legacy path, got %d", len(specs))
	}
	if specs[0].ID != "v1" {
		t.Errorf("ID = %q, want %q", specs[0].ID, "v1")
	}
}

func TestLocalSource_ListSpecs_StatusFilter(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, ".gurgeh", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write two specs with different statuses
	for _, tc := range []struct {
		file, version, status string
	}{
		{"mvp.yaml", "mvp", "draft"},
		{"v1.yaml", "v1", "approved"},
	} {
		yaml := "id: X\ntitle: T\nversion: " + tc.version + "\nstatus: " + tc.status + "\ncreated_at: \"2025-01-01T00:00:00Z\"\nfeatures: []\n"
		if err := os.WriteFile(filepath.Join(specsDir, tc.file), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	src := NewLocalSource(dir)

	// Filter by "draft" → should match "mvp" (draft maps to SpecStatusDraft = "draft")
	specs, err := src.ListSpecs("draft")
	if err != nil {
		t.Fatalf("ListSpecs(draft): %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec with draft status, got %d", len(specs))
	}
	if specs[0].ID != "mvp" {
		t.Errorf("filtered spec ID = %q, want %q", specs[0].ID, "mvp")
	}
}

func TestLocalSource_ListEpics(t *testing.T) {
	dir := t.TempDir()
	db := setupTestDB(t, dir)

	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO epics (id, feature_ref, title, status, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "EPIC-001", "FEAT-001", "Auth System", "open", 1, now, now)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	src := NewLocalSource(dir)
	epics, err := src.ListEpics("")
	if err != nil {
		t.Fatalf("ListEpics: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(epics))
	}
	if epics[0].ID != "EPIC-001" {
		t.Errorf("ID = %q", epics[0].ID)
	}
	if epics[0].SpecID != "FEAT-001" {
		t.Errorf("SpecID = %q, want %q (mapped from FeatureRef)", epics[0].SpecID, "FEAT-001")
	}
	if string(epics[0].Status) != "open" {
		t.Errorf("Status = %q", epics[0].Status)
	}
}

func TestLocalSource_ListEpics_SpecIDFilter(t *testing.T) {
	dir := t.TempDir()
	db := setupTestDB(t, dir)

	now := time.Now().Format(time.RFC3339)
	for _, e := range []struct {
		id, ref, title string
	}{
		{"EPIC-001", "FEAT-001", "Auth"},
		{"EPIC-002", "FEAT-002", "Payments"},
	} {
		_, err := db.Exec(`INSERT INTO epics (id, feature_ref, title, status, priority, created_at, updated_at)
			VALUES (?, ?, ?, 'open', 1, ?, ?)`, e.id, e.ref, e.title, now, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	src := NewLocalSource(dir)
	epics, err := src.ListEpics("FEAT-001")
	if err != nil {
		t.Fatalf("ListEpics: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("expected 1 filtered epic, got %d", len(epics))
	}
	if epics[0].ID != "EPIC-001" {
		t.Errorf("ID = %q", epics[0].ID)
	}
}

func TestLocalSource_ListEpics_NoV2(t *testing.T) {
	dir := t.TempDir()
	dbDir := filepath.Join(dir, ".coldwine")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create DB without MigrateV2 — no epics table
	db, err := autarchdb.Open(filepath.Join(dbDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	src := NewLocalSource(dir)
	epics, err := src.ListEpics("")
	if err != nil {
		t.Fatalf("ListEpics should return nil error for missing table, got: %v", err)
	}
	if len(epics) != 0 {
		t.Errorf("expected empty slice, got %d epics", len(epics))
	}
}

func TestLocalSource_ListStories(t *testing.T) {
	dir := t.TempDir()
	db := setupTestDB(t, dir)

	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO epics (id, feature_ref, title, status, priority, created_at, updated_at)
		VALUES ('EPIC-001', 'FEAT-001', 'Auth', 'open', 1, ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO stories (id, epic_id, title, status, priority, created_at, updated_at)
		VALUES ('STORY-001', 'EPIC-001', 'Login flow', 'open', 1, ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	src := NewLocalSource(dir)
	stories, err := src.ListStories("EPIC-001")
	if err != nil {
		t.Fatalf("ListStories: %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("expected 1 story, got %d", len(stories))
	}
	if stories[0].EpicID != "EPIC-001" {
		t.Errorf("EpicID = %q", stories[0].EpicID)
	}
}

func TestLocalSource_ListTasks(t *testing.T) {
	dir := t.TempDir()
	db := setupTestDB(t, dir)

	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO epics (id, feature_ref, title, status, priority, created_at, updated_at)
		VALUES ('EPIC-001', 'FEAT-001', 'Auth', 'open', 1, ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO stories (id, epic_id, title, status, priority, created_at, updated_at)
		VALUES ('STORY-001', 'EPIC-001', 'Login', 'open', 1, ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO work_tasks (id, story_id, title, status, priority, assignee, session_ref, created_at, updated_at)
		VALUES ('TASK-001', 'STORY-001', 'Implement login', 'in_progress', 1, 'claude', 'sess-1', ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	src := NewLocalSource(dir)

	// No filter
	tasks, err := src.ListTasks("", "")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Agent != "claude" {
		t.Errorf("Agent = %q, want %q (mapped from assignee)", tasks[0].Agent, "claude")
	}
	if tasks[0].SessionID != "sess-1" {
		t.Errorf("SessionID = %q", tasks[0].SessionID)
	}

	// Filter by status
	tasks, err = src.ListTasks("in_progress", "")
	if err != nil {
		t.Fatalf("ListTasks(status): %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task for status filter, got %d", len(tasks))
	}

	// Filter by agent
	tasks, err = src.ListTasks("", "claude")
	if err != nil {
		t.Fatalf("ListTasks(agent): %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task for agent filter, got %d", len(tasks))
	}

	// Filter by non-matching agent
	tasks, err = src.ListTasks("", "codex")
	if err != nil {
		t.Fatalf("ListTasks(no match): %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestLocalSource_ListInsights(t *testing.T) {
	dir := t.TempDir()
	insightsDir := filepath.Join(dir, ".pollard", "insights")
	if err := os.MkdirAll(insightsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	insightYAML := `id: INS-001
title: Competitor Analysis
category: competitive
collected_at: 2025-06-15T10:00:00Z
sources:
  - url: https://example.com
    type: product
  - url: https://other.com
    type: article
findings:
  - title: Feature gap
    relevance: high
    description: Missing auth
`
	if err := os.WriteFile(filepath.Join(insightsDir, "INS-001.yaml"), []byte(insightYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(dir)
	insights, err := src.ListInsights("", "")
	if err != nil {
		t.Fatalf("ListInsights: %v", err)
	}
	if len(insights) != 1 {
		t.Fatalf("expected 1 insight, got %d", len(insights))
	}

	i := insights[0]
	if i.ID != "INS-001" {
		t.Errorf("ID = %q", i.ID)
	}
	if i.Category != "competitive" {
		t.Errorf("Category = %q", i.Category)
	}
	// Lossy mapping: first source only
	if i.Source != "product" {
		t.Errorf("Source = %q, want %q (first source type)", i.Source, "product")
	}
	if i.URL != "https://example.com" {
		t.Errorf("URL = %q", i.URL)
	}
	// Body unavailable in local insights
	if i.Body != "" {
		t.Errorf("Body should be empty for local insight, got %q", i.Body)
	}
}

func TestLocalSource_ListInsights_CategoryFilter(t *testing.T) {
	dir := t.TempDir()
	insightsDir := filepath.Join(dir, ".pollard", "insights")
	if err := os.MkdirAll(insightsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		file, id, category string
	}{
		{"comp.yaml", "INS-001", "competitive"},
		{"trend.yaml", "INS-002", "trends"},
	} {
		yaml := "id: " + tc.id + "\ntitle: T\ncategory: " + tc.category + "\ncollected_at: 2025-01-01T00:00:00Z\nsources: []\nfindings: []\n"
		if err := os.WriteFile(filepath.Join(insightsDir, tc.file), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	src := NewLocalSource(dir)
	insights, err := src.ListInsights("", "competitive")
	if err != nil {
		t.Fatalf("ListInsights: %v", err)
	}
	if len(insights) != 1 {
		t.Fatalf("expected 1 insight, got %d", len(insights))
	}
	if insights[0].ID != "INS-001" {
		t.Errorf("ID = %q", insights[0].ID)
	}
}

func TestLocalSource_ListInsights_NoSources(t *testing.T) {
	dir := t.TempDir()
	insightsDir := filepath.Join(dir, ".pollard", "insights")
	if err := os.MkdirAll(insightsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yaml := "id: INS-001\ntitle: T\ncategory: trends\ncollected_at: 2025-01-01T00:00:00Z\nsources: []\nfindings: []\n"
	if err := os.WriteFile(filepath.Join(insightsDir, "test.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	src := NewLocalSource(dir)
	insights, err := src.ListInsights("", "")
	if err != nil {
		t.Fatalf("ListInsights: %v", err)
	}
	if len(insights) != 1 {
		t.Fatalf("expected 1 insight, got %d", len(insights))
	}
	// Guard: empty sources → "local"
	if insights[0].Source != "local" {
		t.Errorf("Source = %q, want %q for empty sources", insights[0].Source, "local")
	}
}

func TestLocalSource_MissingDir(t *testing.T) {
	dir := t.TempDir() // No dot-directories created

	src := NewLocalSource(dir)

	// All should return (empty, nil)
	specs, err := src.ListSpecs("")
	if err != nil {
		t.Errorf("ListSpecs: %v", err)
	}
	if len(specs) != 0 {
		t.Errorf("ListSpecs: expected empty, got %d", len(specs))
	}

	epics, err := src.ListEpics("")
	if err != nil {
		t.Errorf("ListEpics: %v", err)
	}
	if len(epics) != 0 {
		t.Errorf("ListEpics: expected empty, got %d", len(epics))
	}

	stories, err := src.ListStories("")
	if err != nil {
		t.Errorf("ListStories: %v", err)
	}
	if len(stories) != 0 {
		t.Errorf("ListStories: expected empty, got %d", len(stories))
	}

	tasks, err := src.ListTasks("", "")
	if err != nil {
		t.Errorf("ListTasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("ListTasks: expected empty, got %d", len(tasks))
	}

	insights, err := src.ListInsights("", "")
	if err != nil {
		t.Errorf("ListInsights: %v", err)
	}
	if len(insights) != 0 {
		t.Errorf("ListInsights: expected empty, got %d", len(insights))
	}
}

// setupTestDB creates a .coldwine/state.db with V2 schema.
func setupTestDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	dbDir := filepath.Join(dir, ".coldwine")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}

	db, err := autarchdb.Open(filepath.Join(dbDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}

	// Apply V2 schema (creates epics, stories, work_tasks tables)
	if err := storage.MigrateV2(db); err != nil {
		t.Fatal(err)
	}

	return db
}
