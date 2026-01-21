# Task-First Flow Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make Fleet view task‑centric with start/stop/review actions so the TUI feels active like Clark/Claude Squad.

**Architecture:** Query tasks from SQLite as the primary list, add key handlers for start/stop/review, and wire to existing git/tmux/storage helpers. Keep review view unchanged. Use TDD for each behavior change and commit after each small step.

**Tech Stack:** Go 1.22+, Bubble Tea, Cobra, SQLite, tmux, YAML specs.

---

### Task 1: Task list loader (SQLite → TUI)

**Files:**
- Create: `internal/tui/task_loader.go`
- Create: `internal/tui/task_loader_test.go`
- Modify: `internal/storage/task.go`

**Step 1: Write the failing test**

```go
func TestLoadTasksReturnsRows(t *testing.T) {
	db, _ := storage.OpenTemp()
	_ = storage.Migrate(db)
	_ = storage.InsertTask(db, storage.Task{ID: "T1", Title: "One", Status: "todo"})
	list, err := LoadTasks(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "T1" {
		t.Fatalf("expected task list with T1")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`  
Expected: FAIL with "LoadTasks undefined"

**Step 3: Write minimal implementation**

```go
type TaskItem struct {
	ID     string
	Title  string
	Status string
}

func LoadTasks(db *sql.DB) ([]TaskItem, error) {
	rows, err := db.Query(`SELECT id, title, status FROM tasks ORDER BY id ASC`)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []TaskItem
	for rows.Next() {
		var t TaskItem
		if err := rows.Scan(&t.ID, &t.Title, &t.Status); err != nil { return nil, err }
		out = append(out, t)
	}
	return out, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/task_loader.go internal/tui/task_loader_test.go internal/storage/task.go
git commit -m "feat: add task list loader"
```

---

### Task 2: Task list in Fleet view

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/view_test.go`

**Step 1: Write the failing test**

```go
func TestFleetViewShowsTaskList(t *testing.T) {
	m := NewModel()
	m.TaskList = []TaskItem{{ID: "T1", Title: "One", Status: "todo"}}
	out := m.View()
	if !strings.Contains(out, "T1") || !strings.Contains(out, "One") {
		t.Fatalf("expected task list in view")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`  
Expected: FAIL with missing task list in view

**Step 3: Write minimal implementation**

```go
// in Model
TaskList []TaskItem
SelectedTask int

// in View() fleet section
out += "TASKS\n"
for i, t := range m.TaskList {
	prefix := "- "
	if i == m.SelectedTask { prefix = "> " }
	out += prefix + t.ID + " " + t.Title + " [" + t.Status + "]\n"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/view_test.go
git commit -m "feat: show tasks in fleet view"
```

---

### Task 3: Start/Stop actions for selected task

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/task_actions_test.go`

**Step 1: Write the failing test**

```go
func TestStartTaskCallsStarter(t *testing.T) {
	m := NewModel()
	m.TaskList = []TaskItem{{ID: "T1", Title: "One", Status: "todo"}}
	called := false
	m.TaskStarter = func(id string) error { called = true; return nil }
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated.(Model)
	if !called {
		t.Fatalf("expected starter to be called")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`  
Expected: FAIL with TaskStarter undefined or not called

**Step 3: Write minimal implementation**

```go
// in Model
TaskStarter func(id string) error
TaskStopper func(id string) error

// in Update() switch
case "s": m.handleTaskStart()
case "x": m.handleTaskStop()

// implement handlers with defaults using git/tmux/storage
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/task_actions_test.go
git commit -m "feat: start/stop task actions"
```

---

### Task 4: Review action enters Review view for review tasks

**Files:**
- Modify: `internal/tui/model.go`
- Create: `internal/tui/task_review_test.go`

**Step 1: Write the failing test**

```go
func TestReviewActionEntersReviewView(t *testing.T) {
	m := NewModel()
	m.TaskList = []TaskItem{{ID: "T1", Title: "One", Status: "review"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updated.(Model)
	if m.ViewMode != ViewReview {
		t.Fatalf("expected review view")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`  
Expected: FAIL with view mode unchanged

**Step 3: Write minimal implementation**

```go
case "r":
	if m.ViewMode == ViewFleet && m.selectedTaskStatus() == "review" {
		m.ViewMode = ViewReview
		m.ensureReviewDetail()
	}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/task_review_test.go
git commit -m "feat: enter review view from task list"
```

---

### Task 5: Wire task list loading on startup

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

**Step 1: Write the failing test**

```go
func TestRefreshTasksLoadsFromProject(t *testing.T) {
	m := NewModel()
	m.TaskLoader = func() ([]TaskItem, error) { return []TaskItem{{ID: "T1"}}, nil }
	m.RefreshTasks()
	if len(m.TaskList) != 1 {
		t.Fatalf("expected tasks loaded")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -v`  
Expected: FAIL with RefreshTasks undefined

**Step 3: Write minimal implementation**

```go
TaskLoader func() ([]TaskItem, error)
func (m *Model) RefreshTasks() { ... }
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat: load task list on startup"
```

---

## Verification

Run full TUI tests:
```bash
go test ./internal/tui -v
```

Manual smoke:
```bash
./dev
# Press n → create task
# Press s → start (stubbed initially if needed)
# Press r → review for review tasks
```
