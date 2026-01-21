# Tandemonium Go Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a TUI for orchestrating multiple AI coding agents using Go + Bubble Tea, with PM Agent refinement as the key differentiator.

**Architecture:** Bubble Tea Model-Update-View pattern, tmux for agent session isolation, Claude API for PM Agent, SQLite for persistence.

**Tech Stack:** Go 1.22+, Bubble Tea, Lip Gloss, Bubbles, SQLite, tmux

---

## Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/tandemonium/main.go`
- Create: `internal/app/app.go`

**Step 1: Initialize Go module**

```bash
cd /Users/sma/Tandemonium
go mod init github.com/sma/tandemonium
```

**Step 2: Add dependencies**

```bash
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbles/...
go get github.com/mattn/go-sqlite3
go get github.com/BurntSushi/toml
go get github.com/anthropics/anthropic-sdk-go
```

**Step 3: Create entry point**

Create `cmd/tandemonium/main.go`:

```go
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sma/tandemonium/internal/app"
)

func main() {
	p := tea.NewProgram(app.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 4: Create app skeleton**

Create `internal/app/app.go`:

```go
package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type View int

const (
	ViewFleet View = iota
	ViewFocus
	ViewRefine
	ViewSpecReview
	ViewCodeReview
	ViewQueue
)

type Model struct {
	currentView View
	width       int
	height      int
}

func New() Model {
	return Model{
		currentView: ViewFleet,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m Model) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	return titleStyle.Render("TANDEMONIUM") + "\n\nPress 'q' to quit"
}
```

**Step 5: Verify it runs**

```bash
go run cmd/tandemonium/main.go
```

Expected: TUI shows "TANDEMONIUM" title, pressing 'q' exits.

**Step 6: Commit**

```bash
git add go.mod go.sum cmd/ internal/
git commit -m "feat: initialize Go project with Bubble Tea skeleton"
```

---

## Task 2: Data Models

**Files:**
- Create: `internal/models/task.go`
- Create: `internal/models/agent.go`
- Create: `internal/models/log.go`
- Create: `internal/models/task_test.go`

**Step 1: Write failing test for Task model**

Create `internal/models/task_test.go`:

```go
package models

import (
	"testing"
	"time"
)

func TestTaskCreation(t *testing.T) {
	task := NewTask("Add user authentication")

	if task.ID == "" {
		t.Error("Task ID should not be empty")
	}
	if task.RawInput != "Add user authentication" {
		t.Errorf("RawInput = %q, want %q", task.RawInput, "Add user authentication")
	}
	if task.Status != StatusDraft {
		t.Errorf("Status = %q, want %q", task.Status, StatusDraft)
	}
	if task.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestTaskStatusTransition(t *testing.T) {
	task := NewTask("Test task")

	// Valid transition: Draft -> Refining
	err := task.SetStatus(StatusRefining)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if task.Status != StatusRefining {
		t.Errorf("Status = %q, want %q", task.Status, StatusRefining)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/models/... -v
```

Expected: FAIL - package doesn't exist yet

**Step 3: Implement Task model**

Create `internal/models/task.go`:

```go
package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type TaskStatus string

const (
	StatusDraft         TaskStatus = "draft"
	StatusRefining      TaskStatus = "refining"
	StatusPendingReview TaskStatus = "pending_review"
	StatusQueued        TaskStatus = "queued"
	StatusAssigned      TaskStatus = "assigned"
	StatusInProgress    TaskStatus = "in_progress"
	StatusBlocked       TaskStatus = "blocked"
	StatusReview        TaskStatus = "review"
	StatusDone          TaskStatus = "done"
	StatusRejected      TaskStatus = "rejected"
)

// ValidTransitions defines allowed status transitions
var ValidTransitions = map[TaskStatus][]TaskStatus{
	StatusDraft:         {StatusRefining, StatusQueued}, // Can skip refinement
	StatusRefining:      {StatusPendingReview},
	StatusPendingReview: {StatusQueued, StatusRejected},
	StatusQueued:        {StatusAssigned},
	StatusAssigned:      {StatusInProgress},
	StatusInProgress:    {StatusBlocked, StatusReview},
	StatusBlocked:       {StatusInProgress},
	StatusReview:        {StatusDone, StatusRejected},
	StatusRejected:      {StatusRefining, StatusQueued},
}

type QAPair struct {
	Question   string    `json:"question"`
	Answer     string    `json:"answer"`
	AnsweredAt time.Time `json:"answered_at"`
}

type FilePlan struct {
	Path        string `json:"path"`
	Action      string `json:"action"` // "create", "modify", "delete"
	Description string `json:"description"`
}

type ContextItem struct {
	Source  string `json:"source"` // "file", "search", "git"
	Content string `json:"content"`
}

type TestResults struct {
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Skipped int    `json:"skipped"`
	Output  string `json:"output"`
}

type RefinedSpec struct {
	Title               string        `json:"title"`
	Summary             string        `json:"summary"`
	Context             []ContextItem `json:"context"`
	Requirements        []string      `json:"requirements"`
	AcceptanceCriteria  []string      `json:"acceptance_criteria"`
	FilesToModify       []FilePlan    `json:"files_to_modify"`
	ImplementationNotes string        `json:"implementation_notes"`
	EstimatedComplexity string        `json:"estimated_complexity"`
	EstimatedMinutes    int           `json:"estimated_minutes"`
	QuestionsAsked      int           `json:"questions_asked"`
	FilesExamined       int           `json:"files_examined"`
}

type Task struct {
	ID        string     `json:"id"`
	RawInput  string     `json:"raw_input"`
	Status    TaskStatus `json:"status"`
	Priority  int        `json:"priority"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Refinement
	RefinedSpec         *RefinedSpec `json:"refined_spec,omitempty"`
	Clarifications      []QAPair     `json:"clarifications,omitempty"`
	RefinementCostCents int          `json:"refinement_cost_cents"`
	RefinementSkipped   bool         `json:"refinement_skipped"`

	// Execution
	AssignedAgent      string `json:"assigned_agent,omitempty"`
	ExecutionCostCents int    `json:"execution_cost_cents"`
	BranchName         string `json:"branch_name,omitempty"`
	TmuxSession        string `json:"tmux_session,omitempty"`

	// Completion
	FilesChanged  []string     `json:"files_changed,omitempty"`
	TestResults   *TestResults `json:"test_results,omitempty"`
	HumanFeedback string       `json:"human_feedback,omitempty"`
}

func NewTask(rawInput string) *Task {
	now := time.Now()
	return &Task{
		ID:        generateTaskID(),
		RawInput:  rawInput,
		Status:    StatusDraft,
		Priority:  2, // Medium by default
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func generateTaskID() string {
	// Use first 8 chars of UUID for shorter IDs
	return "TAND-" + uuid.New().String()[:8]
}

func (t *Task) SetStatus(newStatus TaskStatus) error {
	allowed, ok := ValidTransitions[t.Status]
	if !ok {
		return fmt.Errorf("no transitions defined for status %q", t.Status)
	}

	for _, s := range allowed {
		if s == newStatus {
			t.Status = newStatus
			t.UpdatedAt = time.Now()
			return nil
		}
	}

	return fmt.Errorf("invalid transition from %q to %q", t.Status, newStatus)
}

func (t *Task) TotalCostCents() int {
	return t.RefinementCostCents + t.ExecutionCostCents
}
```

**Step 4: Add uuid dependency and run tests**

```bash
go get github.com/google/uuid
go test ./internal/models/... -v
```

Expected: PASS

**Step 5: Create Agent model**

Create `internal/models/agent.go`:

```go
package models

import "time"

type AgentType string

const (
	AgentTypePM    AgentType = "pm"
	AgentTypeCoder AgentType = "coder"
)

type AgentStatus string

const (
	AgentStatusIdle           AgentStatus = "idle"
	AgentStatusRefining       AgentStatus = "refining"
	AgentStatusWorking        AgentStatus = "working"
	AgentStatusBlocked        AgentStatus = "blocked"
	AgentStatusAwaitingReview AgentStatus = "awaiting_review"
)

type BlockedState struct {
	Question  string    `json:"question"`
	Context   string    `json:"context"`
	Options   []string  `json:"options,omitempty"`
	BlockedAt time.Time `json:"blocked_at"`
}

type Agent struct {
	ID              string        `json:"id"`
	Type            AgentType     `json:"type"`
	Status          AgentStatus   `json:"status"`
	CurrentTaskID   string        `json:"current_task_id,omitempty"`
	TmuxSession     string        `json:"tmux_session,omitempty"`
	CostCents       int           `json:"cost_cents"`
	StartedAt       *time.Time    `json:"started_at,omitempty"`
	WorkingDir      string        `json:"working_dir"`
	BranchName      string        `json:"branch_name,omitempty"`
	BlockedState    *BlockedState `json:"blocked_state,omitempty"`
	LastContentHash string        `json:"last_content_hash,omitempty"`
}

func NewCoderAgent(id string) *Agent {
	return &Agent{
		ID:     id,
		Type:   AgentTypeCoder,
		Status: AgentStatusIdle,
	}
}

func NewPMAgent(id string) *Agent {
	return &Agent{
		ID:     id,
		Type:   AgentTypePM,
		Status: AgentStatusIdle,
	}
}
```

**Step 6: Create Log model**

Create `internal/models/log.go`:

```go
package models

import "time"

type LogEntryType string

const (
	LogTypeOutput   LogEntryType = "output"
	LogTypeInput    LogEntryType = "input"
	LogTypeStatus   LogEntryType = "status"
	LogTypeError    LogEntryType = "error"
	LogTypeCost     LogEntryType = "cost"
	LogTypeQuestion LogEntryType = "question"
)

type LogEntry struct {
	Timestamp time.Time    `json:"timestamp"`
	Type      LogEntryType `json:"type"`
	AgentID   string       `json:"agent_id"`
	TaskID    string       `json:"task_id,omitempty"`
	Content   string       `json:"content"`
}

func NewLogEntry(entryType LogEntryType, agentID, content string) LogEntry {
	return LogEntry{
		Timestamp: time.Now(),
		Type:      entryType,
		AgentID:   agentID,
		Content:   content,
	}
}
```

**Step 7: Run tests again**

```bash
go test ./internal/models/... -v
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/models/
git commit -m "feat: add Task, Agent, and Log data models"
```

---

## Task 3: Tmux Session Wrapper

**Files:**
- Create: `internal/agents/tmux/session.go`
- Create: `internal/agents/tmux/session_test.go`

**Step 1: Write failing test**

Create `internal/agents/tmux/session_test.go`:

```go
package tmux

import (
	"os/exec"
	"testing"
)

func TestSessionPrefix(t *testing.T) {
	if SessionPrefix != "tandemonium_" {
		t.Errorf("SessionPrefix = %q, want %q", SessionPrefix, "tandemonium_")
	}
}

func TestSessionName(t *testing.T) {
	s := &Session{Name: "tandemonium_test"}
	if s.Name != "tandemonium_test" {
		t.Errorf("Name = %q, want %q", s.Name, "tandemonium_test")
	}
}

// Integration test - requires tmux
func TestSessionCreateAndKill(t *testing.T) {
	// Skip if tmux not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	session, err := Create("test-agent", "/tmp", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer session.Kill()

	if !session.Exists() {
		t.Error("Session should exist after creation")
	}

	session.Kill()
	if session.Exists() {
		t.Error("Session should not exist after kill")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/agents/tmux/... -v
```

Expected: FAIL - package doesn't exist

**Step 3: Implement tmux session wrapper**

Create `internal/agents/tmux/session.go`:

```go
package tmux

import (
	"crypto/sha256"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const SessionPrefix = "tandemonium_"

type Session struct {
	Name            string
	WorkDir         string
	LastContentHash string
}

// Create creates a new detached tmux session
func Create(agentID, workDir, command string) (*Session, error) {
	name := SessionPrefix + agentID

	// Create detached session
	args := []string{"new-session", "-d", "-s", name, "-c", workDir}
	cmd := exec.Command("tmux", args...)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Wait for session to exist
	if err := waitForSession(name, 5*time.Second); err != nil {
		return nil, err
	}

	// Configure history limit
	exec.Command("tmux", "set-option", "-t", name, "history-limit", "50000").Run()

	// Send the command if provided
	if command != "" {
		exec.Command("tmux", "send-keys", "-t", name, command, "Enter").Run()
	}

	return &Session{Name: name, WorkDir: workDir}, nil
}

func waitForSession(name string, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		cmd := exec.Command("tmux", "has-session", "-t", name)
		if cmd.Run() == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("session %s did not start within %v", name, timeout)
}

// CapturePane captures the current pane content
func (s *Session) CapturePane() (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", s.Name, "-p", "-e")
	out, err := cmd.Output()
	return string(out), err
}

// CapturePaneHistory captures pane with scrollback history
func (s *Session) CapturePaneHistory(lines int) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", s.Name, "-p", "-S", fmt.Sprintf("-%d", lines))
	out, err := cmd.Output()
	return string(out), err
}

// HasUpdated checks if content changed since last check (hash-based)
func (s *Session) HasUpdated() bool {
	content, err := s.CapturePane()
	if err != nil {
		return false
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	updated := s.LastContentHash != hash
	s.LastContentHash = hash
	return updated
}

// SendKeys sends keystrokes to the session
func (s *Session) SendKeys(keys string) error {
	return exec.Command("tmux", "send-keys", "-t", s.Name, keys).Run()
}

// SendText sends text followed by Enter
func (s *Session) SendText(text string) error {
	if err := s.SendKeys(text); err != nil {
		return err
	}
	return s.SendKeys("Enter")
}

// TapEnter sends Enter keystroke (for auto-accept)
func (s *Session) TapEnter() error {
	return s.SendKeys("Enter")
}

// SendCtrlC sends Ctrl+C to interrupt
func (s *Session) SendCtrlC() error {
	return exec.Command("tmux", "send-keys", "-t", s.Name, "C-c").Run()
}

// Kill kills the session
func (s *Session) Kill() error {
	return exec.Command("tmux", "kill-session", "-t", s.Name).Run()
}

// Exists checks if session still exists
func (s *Session) Exists() bool {
	return exec.Command("tmux", "has-session", "-t", s.Name).Run() == nil
}

// CleanupAll kills all tandemonium sessions
func CleanupAll() {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, SessionPrefix) {
			exec.Command("tmux", "kill-session", "-t", line).Run()
		}
	}
}

// ListAll lists all tandemonium sessions
func ListAll() []string {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return nil
	}

	var sessions []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, SessionPrefix) {
			sessions = append(sessions, line)
		}
	}
	return sessions
}
```

**Step 4: Run tests**

```bash
go test ./internal/agents/tmux/... -v
```

Expected: PASS (or skip if tmux not installed)

**Step 5: Commit**

```bash
git add internal/agents/tmux/
git commit -m "feat: add tmux session wrapper"
```

---

## Task 4: Prompt Detector

**Files:**
- Create: `internal/agents/detector/detector.go`
- Create: `internal/agents/detector/detector_test.go`

**Step 1: Write failing test**

Create `internal/agents/detector/detector_test.go`:

```go
package detector

import "testing"

func TestDetectTrustPrompt(t *testing.T) {
	tests := []struct {
		content string
		want    State
	}{
		{"Do you trust this file?", StateTrustPrompt},
		{"Allow once or always?", StateTrustPrompt},
		{"(y)es or (n)o?", StateTrustPrompt},
		{"Press Enter to continue", StateTrustPrompt},
	}

	for _, tt := range tests {
		got := Detect(tt.content)
		if got != tt.want {
			t.Errorf("Detect(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}

func TestDetectBlocked(t *testing.T) {
	tests := []struct {
		content string
		want    State
	}{
		{"Should I use Redis or Memcached?", StateBlocked},
		{"Which approach do you prefer?", StateBlocked},
		{"Do you want me to continue?", StateBlocked},
	}

	for _, tt := range tests {
		got := Detect(tt.content)
		if got != tt.want {
			t.Errorf("Detect(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}

func TestDetectComplete(t *testing.T) {
	tests := []struct {
		content string
		want    State
	}{
		{"Task complete!", StateComplete},
		{"All tests pass", StateComplete},
		{"Successfully implemented the feature", StateComplete},
	}

	for _, tt := range tests {
		got := Detect(tt.content)
		if got != tt.want {
			t.Errorf("Detect(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}

func TestDetectWorking(t *testing.T) {
	content := "Running npm install...\nInstalling dependencies..."
	got := Detect(content)
	if got != StateWorking {
		t.Errorf("Detect(%q) = %q, want %q", content, got, StateWorking)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/agents/detector/... -v
```

Expected: FAIL - package doesn't exist

**Step 3: Implement prompt detector**

Create `internal/agents/detector/detector.go`:

```go
package detector

import (
	"regexp"
	"strings"
)

type State string

const (
	StateWorking     State = "working"
	StateTrustPrompt State = "trust_prompt"
	StateBlocked     State = "blocked"
	StateComplete    State = "complete"
)

var (
	questionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)should i`),
		regexp.MustCompile(`(?i)which (one|option|approach)`),
		regexp.MustCompile(`(?i)do you want`),
		regexp.MustCompile(`(?i)would you like`),
		regexp.MustCompile(`(?i)can you clarify`),
		regexp.MustCompile(`(?i)i need (to know|clarification|more info)`),
		regexp.MustCompile(`(?i)what (should|would|do)`),
	}

	trustPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)trust (this|these) files?`),
		regexp.MustCompile(`(?i)allow (once|always)`),
		regexp.MustCompile(`(?i)\(y\)es.*\(n\)o`),
		regexp.MustCompile(`(?i)yes.?no`),
		regexp.MustCompile(`(?i)press enter to continue`),
		regexp.MustCompile(`(?i)do you trust`),
	}

	completionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)task (complete|finished|done)`),
		regexp.MustCompile(`(?i)all tests pass`),
		regexp.MustCompile(`(?i)successfully (created|updated|implemented)`),
		regexp.MustCompile(`(?i)implementation complete`),
		regexp.MustCompile(`(?i)changes committed`),
	}
)

// Detect analyzes pane content and returns the detected state
func Detect(content string) State {
	// Check last 10 lines for prompts
	lines := strings.Split(content, "\n")
	start := len(lines) - 10
	if start < 0 {
		start = 0
	}
	tail := strings.Join(lines[start:], "\n")

	// Trust prompts take priority (need immediate action)
	for _, p := range trustPatterns {
		if p.MatchString(tail) {
			return StateTrustPrompt
		}
	}

	// Then check for completion
	for _, p := range completionPatterns {
		if p.MatchString(tail) {
			return StateComplete
		}
	}

	// Then check for blocked (questions)
	for _, p := range questionPatterns {
		if p.MatchString(tail) {
			return StateBlocked
		}
	}

	return StateWorking
}

// ExtractQuestion attempts to extract the question from blocked content
func ExtractQuestion(content string) string {
	lines := strings.Split(content, "\n")

	// Look for question marks in last 10 lines
	start := len(lines) - 10
	if start < 0 {
		start = 0
	}

	for i := len(lines) - 1; i >= start; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.Contains(line, "?") {
			return line
		}
	}

	// If no question mark, return last non-empty line
	for i := len(lines) - 1; i >= start; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}

	return ""
}
```

**Step 4: Run tests**

```bash
go test ./internal/agents/detector/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/agents/detector/
git commit -m "feat: add prompt detector for agent state detection"
```

---

## Task 5: SQLite Store

**Files:**
- Create: `internal/services/store/store.go`
- Create: `internal/services/store/store_test.go`

**Step 1: Write failing test**

Create `internal/services/store/store_test.go`:

```go
package store

import (
	"os"
	"testing"

	"github.com/sma/tandemonium/internal/models"
)

func TestStoreTaskCRUD(t *testing.T) {
	// Use temp file for test
	f, err := os.CreateTemp("", "tandemonium-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	s, err := New(f.Name())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer s.Close()

	// Create
	task := models.NewTask("Test task")
	if err := s.SaveTask(task); err != nil {
		t.Fatalf("SaveTask() error: %v", err)
	}

	// Read
	loaded, err := s.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask() error: %v", err)
	}
	if loaded.RawInput != task.RawInput {
		t.Errorf("RawInput = %q, want %q", loaded.RawInput, task.RawInput)
	}

	// Update
	task.Status = models.StatusRefining
	if err := s.SaveTask(task); err != nil {
		t.Fatalf("SaveTask() update error: %v", err)
	}

	loaded, _ = s.GetTask(task.ID)
	if loaded.Status != models.StatusRefining {
		t.Errorf("Status = %q, want %q", loaded.Status, models.StatusRefining)
	}

	// List
	tasks, err := s.ListTasks()
	if err != nil {
		t.Fatalf("ListTasks() error: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("len(tasks) = %d, want 1", len(tasks))
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/services/store/... -v
```

Expected: FAIL - package doesn't exist

**Step 3: Implement SQLite store**

Create `internal/services/store/store.go`:

```go
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/sma/tandemonium/internal/models"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		raw_input TEXT NOT NULL,
		status TEXT NOT NULL,
		priority INTEGER NOT NULL DEFAULT 2,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		refined_spec_json TEXT,
		clarifications_json TEXT,
		refinement_cost_cents INTEGER DEFAULT 0,
		refinement_skipped INTEGER DEFAULT 0,
		assigned_agent TEXT,
		execution_cost_cents INTEGER DEFAULT 0,
		branch_name TEXT,
		tmux_session TEXT,
		files_changed_json TEXT,
		test_results_json TEXT,
		human_feedback TEXT
	);

	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		status TEXT NOT NULL,
		current_task_id TEXT,
		tmux_session TEXT,
		cost_cents INTEGER DEFAULT 0,
		started_at TEXT,
		working_dir TEXT,
		branch_name TEXT,
		blocked_state_json TEXT,
		last_content_hash TEXT
	);

	CREATE TABLE IF NOT EXISTS log_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT NOT NULL,
		type TEXT NOT NULL,
		agent_id TEXT,
		task_id TEXT,
		content TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_log_entries_timestamp ON log_entries(timestamp);
	`

	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Task operations

func (s *Store) SaveTask(t *models.Task) error {
	refinedSpecJSON, _ := json.Marshal(t.RefinedSpec)
	clarificationsJSON, _ := json.Marshal(t.Clarifications)
	filesChangedJSON, _ := json.Marshal(t.FilesChanged)
	testResultsJSON, _ := json.Marshal(t.TestResults)

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO tasks (
			id, raw_input, status, priority, created_at, updated_at,
			refined_spec_json, clarifications_json, refinement_cost_cents, refinement_skipped,
			assigned_agent, execution_cost_cents, branch_name, tmux_session,
			files_changed_json, test_results_json, human_feedback
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		t.ID, t.RawInput, t.Status, t.Priority,
		t.CreatedAt.Format(time.RFC3339), t.UpdatedAt.Format(time.RFC3339),
		string(refinedSpecJSON), string(clarificationsJSON),
		t.RefinementCostCents, boolToInt(t.RefinementSkipped),
		t.AssignedAgent, t.ExecutionCostCents, t.BranchName, t.TmuxSession,
		string(filesChangedJSON), string(testResultsJSON), t.HumanFeedback,
	)
	return err
}

func (s *Store) GetTask(id string) (*models.Task, error) {
	row := s.db.QueryRow(`SELECT * FROM tasks WHERE id = ?`, id)
	return s.scanTask(row)
}

func (s *Store) ListTasks() ([]*models.Task, error) {
	rows, err := s.db.Query(`SELECT * FROM tasks ORDER BY priority, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*models.Task
	for rows.Next() {
		t, err := s.scanTaskFromRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *Store) ListTasksByStatus(status models.TaskStatus) ([]*models.Task, error) {
	rows, err := s.db.Query(`SELECT * FROM tasks WHERE status = ? ORDER BY priority, created_at DESC`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*models.Task
	for rows.Next() {
		t, err := s.scanTaskFromRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *Store) DeleteTask(id string) error {
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	return err
}

// Helper functions

type scanner interface {
	Scan(dest ...interface{}) error
}

func (s *Store) scanTask(row *sql.Row) (*models.Task, error) {
	t := &models.Task{}
	var createdAt, updatedAt string
	var refinedSpecJSON, clarificationsJSON, filesChangedJSON, testResultsJSON sql.NullString
	var refinementSkipped int

	err := row.Scan(
		&t.ID, &t.RawInput, &t.Status, &t.Priority, &createdAt, &updatedAt,
		&refinedSpecJSON, &clarificationsJSON, &t.RefinementCostCents, &refinementSkipped,
		&t.AssignedAgent, &t.ExecutionCostCents, &t.BranchName, &t.TmuxSession,
		&filesChangedJSON, &testResultsJSON, &t.HumanFeedback,
	)
	if err != nil {
		return nil, err
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	t.RefinementSkipped = refinementSkipped != 0

	if refinedSpecJSON.Valid {
		json.Unmarshal([]byte(refinedSpecJSON.String), &t.RefinedSpec)
	}
	if clarificationsJSON.Valid {
		json.Unmarshal([]byte(clarificationsJSON.String), &t.Clarifications)
	}
	if filesChangedJSON.Valid {
		json.Unmarshal([]byte(filesChangedJSON.String), &t.FilesChanged)
	}
	if testResultsJSON.Valid {
		json.Unmarshal([]byte(testResultsJSON.String), &t.TestResults)
	}

	return t, nil
}

func (s *Store) scanTaskFromRows(rows *sql.Rows) (*models.Task, error) {
	t := &models.Task{}
	var createdAt, updatedAt string
	var refinedSpecJSON, clarificationsJSON, filesChangedJSON, testResultsJSON sql.NullString
	var refinementSkipped int

	err := rows.Scan(
		&t.ID, &t.RawInput, &t.Status, &t.Priority, &createdAt, &updatedAt,
		&refinedSpecJSON, &clarificationsJSON, &t.RefinementCostCents, &refinementSkipped,
		&t.AssignedAgent, &t.ExecutionCostCents, &t.BranchName, &t.TmuxSession,
		&filesChangedJSON, &testResultsJSON, &t.HumanFeedback,
	)
	if err != nil {
		return nil, err
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	t.RefinementSkipped = refinementSkipped != 0

	if refinedSpecJSON.Valid {
		json.Unmarshal([]byte(refinedSpecJSON.String), &t.RefinedSpec)
	}
	if clarificationsJSON.Valid {
		json.Unmarshal([]byte(clarificationsJSON.String), &t.Clarifications)
	}
	if filesChangedJSON.Valid {
		json.Unmarshal([]byte(filesChangedJSON.String), &t.FilesChanged)
	}
	if testResultsJSON.Valid {
		json.Unmarshal([]byte(testResultsJSON.String), &t.TestResults)
	}

	return t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Log operations

func (s *Store) AppendLog(entry models.LogEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO log_entries (timestamp, type, agent_id, task_id, content)
		VALUES (?, ?, ?, ?, ?)
	`, entry.Timestamp.Format(time.RFC3339), entry.Type, entry.AgentID, entry.TaskID, entry.Content)
	return err
}

func (s *Store) GetRecentLogs(limit int) ([]models.LogEntry, error) {
	rows, err := s.db.Query(`
		SELECT timestamp, type, agent_id, task_id, content
		FROM log_entries ORDER BY timestamp DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.LogEntry
	for rows.Next() {
		var e models.LogEntry
		var ts, taskID sql.NullString
		if err := rows.Scan(&ts, &e.Type, &e.AgentID, &taskID, &e.Content); err != nil {
			return nil, err
		}
		e.Timestamp, _ = time.Parse(time.RFC3339, ts.String)
		if taskID.Valid {
			e.TaskID = taskID.String
		}
		entries = append(entries, e)
	}
	return entries, nil
}
```

**Step 4: Run tests**

```bash
go test ./internal/services/store/... -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/services/store/
git commit -m "feat: add SQLite store for task persistence"
```

---

## Task 6: Fleet View (Basic UI)

**Files:**
- Create: `internal/views/fleet/fleet.go`
- Modify: `internal/app/app.go`

**Step 1: Create Fleet View component**

Create `internal/views/fleet/fleet.go`:

```go
package fleet

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sma/tandemonium/internal/models"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("241"))

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("255"))

	statusStyles = map[models.AgentStatus]lipgloss.Style{
		models.AgentStatusIdle:           lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		models.AgentStatusRefining:       lipgloss.NewStyle().Foreground(lipgloss.Color("205")),
		models.AgentStatusWorking:        lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		models.AgentStatusBlocked:        lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		models.AgentStatusAwaitingReview: lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
	}

	statusIcons = map[models.AgentStatus]string{
		models.AgentStatusIdle:           "○",
		models.AgentStatusRefining:       "◈",
		models.AgentStatusWorking:        "●",
		models.AgentStatusBlocked:        "◐",
		models.AgentStatusAwaitingReview: "◉",
	}

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)
)

type Model struct {
	Agents       []*models.Agent
	Tasks        []*models.Task
	PendingSpecs []*models.Task // Tasks in pending_review status
	QueuedTasks  []*models.Task // Tasks ready to assign
	SelectedIdx  int
	Width        int
	Height       int
}

func New() Model {
	return Model{
		Agents:      make([]*models.Agent, 0),
		Tasks:       make([]*models.Task, 0),
		SelectedIdx: 0,
	}
}

func (m Model) View() string {
	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render("─ TANDEMONIUM ─"))
	b.WriteString("\n")

	// Stats line
	stats := fmt.Sprintf("%d agents    %d tasks", len(m.Agents), len(m.Tasks))
	b.WriteString(headerStyle.Render(stats))
	b.WriteString("\n\n")

	// Agents section
	b.WriteString(headerStyle.Render("AGENTS"))
	b.WriteString("\n")

	if len(m.Agents) == 0 {
		b.WriteString("  No agents running\n")
	} else {
		for i, agent := range m.Agents {
			line := m.renderAgentLine(agent)
			if i == m.SelectedIdx {
				line = selectedStyle.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Pending specs section
	b.WriteString(headerStyle.Render("PENDING SPECS"))
	b.WriteString("\n")

	if len(m.PendingSpecs) == 0 {
		b.WriteString("  No specs awaiting review\n")
	} else {
		for _, task := range m.PendingSpecs {
			b.WriteString(fmt.Sprintf("  › %s  %s\n", task.ID, truncate(task.RawInput, 40)))
		}
	}

	b.WriteString("\n")

	// Task queue section
	b.WriteString(headerStyle.Render("TASK QUEUE"))
	b.WriteString("\n")

	if len(m.QueuedTasks) == 0 {
		b.WriteString("  No tasks queued\n")
	} else {
		for _, task := range m.QueuedTasks {
			b.WriteString(fmt.Sprintf("    %s  %s  P%d\n", task.ID, truncate(task.RawInput, 30), task.Priority))
		}
	}

	// Help
	help := "[n]ew task  [a]ssign  [1-4] focus  [r]eview  [q]uit"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func (m Model) renderAgentLine(agent *models.Agent) string {
	icon := statusIcons[agent.Status]
	style := statusStyles[agent.Status]

	status := string(agent.Status)
	if len(status) > 10 {
		status = status[:10]
	}

	taskInfo := "—"
	if agent.CurrentTaskID != "" {
		taskInfo = agent.CurrentTaskID
	}

	line := fmt.Sprintf("  %s %-10s %-10s %s",
		style.Render(icon),
		agent.ID,
		strings.ToUpper(status),
		taskInfo,
	)

	return line
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func (m *Model) MoveUp() {
	if m.SelectedIdx > 0 {
		m.SelectedIdx--
	}
}

func (m *Model) MoveDown() {
	if m.SelectedIdx < len(m.Agents)-1 {
		m.SelectedIdx++
	}
}

func (m *Model) SelectedAgent() *models.Agent {
	if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Agents) {
		return m.Agents[m.SelectedIdx]
	}
	return nil
}
```

**Step 2: Update app.go to use Fleet View**

Modify `internal/app/app.go`:

```go
package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sma/tandemonium/internal/models"
	"github.com/sma/tandemonium/internal/views/fleet"
)

type View int

const (
	ViewFleet View = iota
	ViewFocus
	ViewRefine
	ViewSpecReview
	ViewCodeReview
	ViewQueue
)

type Model struct {
	currentView View
	width       int
	height      int
	fleetView   fleet.Model
}

func New() Model {
	f := fleet.New()

	// Add some sample agents for testing
	f.Agents = []*models.Agent{
		{ID: "pm-1", Type: models.AgentTypePM, Status: models.AgentStatusRefining, CurrentTaskID: "TAND-001"},
		{ID: "claude-1", Type: models.AgentTypeCoder, Status: models.AgentStatusWorking, CurrentTaskID: "TAND-002"},
		{ID: "claude-2", Type: models.AgentTypeCoder, Status: models.AgentStatusBlocked, CurrentTaskID: "TAND-003"},
		{ID: "claude-3", Type: models.AgentTypeCoder, Status: models.AgentStatusIdle},
	}

	return Model{
		currentView: ViewFleet,
		fleetView:   f,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			m.fleetView.MoveDown()
		case "k", "up":
			m.fleetView.MoveUp()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.fleetView.Width = msg.Width
		m.fleetView.Height = msg.Height
	}
	return m, nil
}

func (m Model) View() string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	content := m.fleetView.View()

	if m.width > 0 && m.height > 0 {
		borderStyle = borderStyle.Width(m.width - 4).Height(m.height - 4)
	}

	return borderStyle.Render(content)
}
```

**Step 3: Test the TUI**

```bash
go run cmd/tandemonium/main.go
```

Expected: TUI shows Fleet View with sample agents, j/k navigation works, q quits.

**Step 4: Commit**

```bash
git add internal/views/fleet/ internal/app/
git commit -m "feat: add Fleet View with agent list"
```

---

## Task 7: Agent Manager (Spawn and Monitor)

**Files:**
- Create: `internal/agents/manager/manager.go`
- Create: `internal/agents/manager/manager_test.go`

**Step 1: Write failing test**

Create `internal/agents/manager/manager_test.go`:

```go
package manager

import (
	"os/exec"
	"testing"
	"time"
)

func TestManagerCreateAgent(t *testing.T) {
	// Skip if tmux not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	m := New("/tmp")
	defer m.Cleanup()

	agent, err := m.SpawnCoder("test-1", "echo hello")
	if err != nil {
		t.Fatalf("SpawnCoder failed: %v", err)
	}

	if agent.ID != "test-1" {
		t.Errorf("ID = %q, want %q", agent.ID, "test-1")
	}

	if len(m.Agents()) != 1 {
		t.Errorf("len(Agents) = %d, want 1", len(m.Agents()))
	}

	// Give it time to run
	time.Sleep(500 * time.Millisecond)

	// Verify session exists
	if agent.TmuxSession == "" {
		t.Error("TmuxSession should be set")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/agents/manager/... -v
```

Expected: FAIL - package doesn't exist

**Step 3: Implement Agent Manager**

Create `internal/agents/manager/manager.go`:

```go
package manager

import (
	"context"
	"sync"
	"time"

	"github.com/sma/tandemonium/internal/agents/detector"
	"github.com/sma/tandemonium/internal/agents/tmux"
	"github.com/sma/tandemonium/internal/models"
)

type StateChangeHandler func(agent *models.Agent, oldState, newState detector.State)

type Manager struct {
	mu       sync.RWMutex
	agents   map[string]*ManagedAgent
	workDir  string
	onChange StateChangeHandler
}

type ManagedAgent struct {
	Agent   *models.Agent
	Session *tmux.Session
	cancel  context.CancelFunc
}

func New(workDir string) *Manager {
	return &Manager{
		agents:  make(map[string]*ManagedAgent),
		workDir: workDir,
	}
}

func (m *Manager) SetOnChange(handler StateChangeHandler) {
	m.onChange = handler
}

func (m *Manager) SpawnCoder(id, command string) (*models.Agent, error) {
	session, err := tmux.Create(id, m.workDir, command)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	agent := &models.Agent{
		ID:          id,
		Type:        models.AgentTypeCoder,
		Status:      models.AgentStatusWorking,
		TmuxSession: session.Name,
		WorkingDir:  m.workDir,
		StartedAt:   &now,
	}

	ctx, cancel := context.WithCancel(context.Background())
	managed := &ManagedAgent{
		Agent:   agent,
		Session: session,
		cancel:  cancel,
	}

	m.mu.Lock()
	m.agents[id] = managed
	m.mu.Unlock()

	// Start monitoring goroutine
	go m.monitor(ctx, managed)

	return agent, nil
}

func (m *Manager) monitor(ctx context.Context, managed *ManagedAgent) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !managed.Session.Exists() {
				m.updateAgentStatus(managed.Agent, models.AgentStatusIdle)
				return
			}

			if managed.Session.HasUpdated() {
				content, err := managed.Session.CapturePane()
				if err != nil {
					continue
				}

				state := detector.Detect(content)
				m.handleStateChange(managed, state, content)
			}
		}
	}
}

func (m *Manager) handleStateChange(managed *ManagedAgent, state detector.State, content string) {
	agent := managed.Agent
	oldStatus := agent.Status

	switch state {
	case detector.StateTrustPrompt:
		// Auto-accept trust prompts
		managed.Session.TapEnter()

	case detector.StateBlocked:
		if agent.Status != models.AgentStatusBlocked {
			question := detector.ExtractQuestion(content)
			agent.Status = models.AgentStatusBlocked
			agent.BlockedState = &models.BlockedState{
				Question:  question,
				BlockedAt: time.Now(),
			}
		}

	case detector.StateComplete:
		agent.Status = models.AgentStatusAwaitingReview

	case detector.StateWorking:
		if agent.Status == models.AgentStatusBlocked {
			// Unblocked
			agent.Status = models.AgentStatusWorking
			agent.BlockedState = nil
		}
	}

	if agent.Status != oldStatus && m.onChange != nil {
		m.onChange(agent, detector.State(oldStatus), state)
	}
}

func (m *Manager) updateAgentStatus(agent *models.Agent, status models.AgentStatus) {
	m.mu.Lock()
	agent.Status = status
	m.mu.Unlock()
}

func (m *Manager) GetAgent(id string) *models.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if managed, ok := m.agents[id]; ok {
		return managed.Agent
	}
	return nil
}

func (m *Manager) Agents() []*models.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*models.Agent, 0, len(m.agents))
	for _, managed := range m.agents {
		agents = append(agents, managed.Agent)
	}
	return agents
}

func (m *Manager) SendInput(id, text string) error {
	m.mu.RLock()
	managed, ok := m.agents[id]
	m.mu.RUnlock()

	if !ok {
		return nil
	}

	return managed.Session.SendText(text)
}

func (m *Manager) KillAgent(id string) error {
	m.mu.Lock()
	managed, ok := m.agents[id]
	if !ok {
		m.mu.Unlock()
		return nil
	}

	managed.cancel()
	delete(m.agents, id)
	m.mu.Unlock()

	return managed.Session.Kill()
}

func (m *Manager) Cleanup() {
	m.mu.Lock()
	for _, managed := range m.agents {
		managed.cancel()
		managed.Session.Kill()
	}
	m.agents = make(map[string]*ManagedAgent)
	m.mu.Unlock()

	tmux.CleanupAll()
}

func (m *Manager) CaptureOutput(id string) (string, error) {
	m.mu.RLock()
	managed, ok := m.agents[id]
	m.mu.RUnlock()

	if !ok {
		return "", nil
	}

	return managed.Session.CapturePaneHistory(1000)
}
```

**Step 4: Run tests**

```bash
go test ./internal/agents/manager/... -v
```

Expected: PASS (or skip if tmux not installed)

**Step 5: Commit**

```bash
git add internal/agents/manager/
git commit -m "feat: add Agent Manager for spawning and monitoring coding agents"
```

---

## Summary

This plan covers the foundation tasks (1-7) for Phase 1 of the Go implementation:

| Task | Component | Purpose |
|------|-----------|---------|
| 1 | Project Scaffolding | Go module, Bubble Tea skeleton |
| 2 | Data Models | Task, Agent, Log structs |
| 3 | Tmux Session | tmux wrapper for session isolation |
| 4 | Prompt Detector | Detect trust prompts, blockers, completion |
| 5 | SQLite Store | Task persistence |
| 6 | Fleet View | Basic TUI with agent list |
| 7 | Agent Manager | Spawn and monitor coding agents |

**Next tasks to add (Phase 1 continued):**
- Task 8: Focus View (single agent detail view)
- Task 9: New Task Modal
- Task 10: Wire up store to app
- Task 11: Integration test (spawn agent, detect completion)

**Phase 2 tasks (PM Agent):**
- Task 12: PM Agent with Claude API
- Task 13: Codebase search tools
- Task 14: Refine View
- Task 15: Spec Review View

---

Plan complete and saved to `docs/plans/2025-01-10-go-implementation-plan.md`.

**Two execution options:**

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**
