package storage

import (
	"database/sql"
	"time"
)

// EpicStatus represents the status of an epic
type EpicStatus string

const (
	EpicStatusDraft      EpicStatus = "draft"
	EpicStatusOpen       EpicStatus = "open"
	EpicStatusInProgress EpicStatus = "in_progress"
	EpicStatusDone       EpicStatus = "done"
	EpicStatusClosed     EpicStatus = "closed"
)

// StoryStatus represents the status of a story
type StoryStatus string

const (
	StoryStatusDraft      StoryStatus = "draft"
	StoryStatusOpen       StoryStatus = "open"
	StoryStatusInProgress StoryStatus = "in_progress"
	StoryStatusDone       StoryStatus = "done"
	StoryStatusClosed     StoryStatus = "closed"
)

// TaskStatus represents the status of a task
type TaskStatus string

const (
	TaskStatusTodo       TaskStatus = "todo"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusDone       TaskStatus = "done"
)

// Epic represents a large body of work broken into stories
type Epic struct {
	ID         string     `json:"id"`          // "EPIC-001"
	FeatureRef string     `json:"feature_ref"` // "FEAT-001" links to Praude
	Title      string     `json:"title"`
	Status     EpicStatus `json:"status"`
	Priority   int        `json:"priority"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Story represents a user story within an epic
type Story struct {
	ID          string      `json:"id"`       // "STORY-001"
	EpicID      string      `json:"epic_id"`  // "EPIC-001"
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Status      StoryStatus `json:"status"`
	Priority    int         `json:"priority"`
	Complexity  string      `json:"complexity"` // xs, s, m, l, xl
	Assignee    string      `json:"assignee,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// WorkTask represents an implementable unit of work
type WorkTask struct {
	ID          string     `json:"id"`       // "TASK-001"
	StoryID     string     `json:"story_id"` // "STORY-001"
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	Priority    int        `json:"priority"`
	Assignee    string     `json:"assignee,omitempty"`
	WorktreeRef string     `json:"worktree_ref,omitempty"` // Git worktree path
	SessionRef  string     `json:"session_ref,omitempty"`  // Agent session ID
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// AgentSession represents an agent working on a task
type AgentSession struct {
	ID            string    `json:"id"`
	TaskID        string    `json:"task_id"`
	AgentName     string    `json:"agent_name"`
	AgentProgram  string    `json:"agent_program"` // claude, codex, aider
	State         string    `json:"state"`         // working, waiting, blocked, done
	WorktreePath  string    `json:"worktree_path,omitempty"`
	LastActiveAt  time.Time `json:"last_active_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// Worktree represents a git worktree for isolated work
type Worktree struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Path      string    `json:"path"`
	Branch    string    `json:"branch"`
	Status    string    `json:"status"` // active, merged, abandoned
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MigrateV2 adds the epic/story/task schema
func MigrateV2(db *sql.DB) error {
	_, err := db.Exec(`
-- Epics table
CREATE TABLE IF NOT EXISTS epics (
  id TEXT PRIMARY KEY,
  feature_ref TEXT,
  title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft',
  priority INTEGER NOT NULL DEFAULT 2,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- Stories table
CREATE TABLE IF NOT EXISTS stories (
  id TEXT PRIMARY KEY,
  epic_id TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT,
  status TEXT NOT NULL DEFAULT 'draft',
  priority INTEGER NOT NULL DEFAULT 2,
  complexity TEXT DEFAULT 'm',
  assignee TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (epic_id) REFERENCES epics(id) ON DELETE CASCADE
);

-- Work tasks table (renamed from tasks to avoid conflict)
CREATE TABLE IF NOT EXISTS work_tasks (
  id TEXT PRIMARY KEY,
  story_id TEXT NOT NULL,
  title TEXT NOT NULL,
  description TEXT,
  status TEXT NOT NULL DEFAULT 'todo',
  priority INTEGER NOT NULL DEFAULT 2,
  assignee TEXT,
  worktree_ref TEXT,
  session_ref TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (story_id) REFERENCES stories(id) ON DELETE CASCADE
);

-- Agent sessions table (for tracking agent work)
CREATE TABLE IF NOT EXISTS agent_sessions (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  agent_name TEXT NOT NULL,
  agent_program TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'working',
  worktree_path TEXT,
  last_active_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY (task_id) REFERENCES work_tasks(id) ON DELETE CASCADE
);

-- Worktrees table
CREATE TABLE IF NOT EXISTS worktrees (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  path TEXT NOT NULL UNIQUE,
  branch TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (task_id) REFERENCES work_tasks(id) ON DELETE CASCADE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_epics_feature_ref ON epics(feature_ref);
CREATE INDEX IF NOT EXISTS idx_epics_status ON epics(status);
CREATE INDEX IF NOT EXISTS idx_stories_epic_id ON stories(epic_id);
CREATE INDEX IF NOT EXISTS idx_stories_status ON stories(status);
CREATE INDEX IF NOT EXISTS idx_stories_assignee ON stories(assignee);
CREATE INDEX IF NOT EXISTS idx_work_tasks_story_id ON work_tasks(story_id);
CREATE INDEX IF NOT EXISTS idx_work_tasks_status ON work_tasks(status);
CREATE INDEX IF NOT EXISTS idx_work_tasks_assignee ON work_tasks(assignee);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_task_id ON agent_sessions(task_id);
CREATE INDEX IF NOT EXISTS idx_agent_sessions_agent_name ON agent_sessions(agent_name);
CREATE INDEX IF NOT EXISTS idx_worktrees_task_id ON worktrees(task_id);
`)
	return err
}
