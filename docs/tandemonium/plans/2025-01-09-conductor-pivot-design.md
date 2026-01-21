# Tandemonium: TUI for Orchestrating AI Coding Agents

**Date:** 2025-01-09 (Updated: 2025-01-10)
**Status:** Proposed
**Author:** Claude (brainstorming skill)
**Version:** 3.2 - Go + Bubble Tea + Architectural Decisions

---

## Changelog

### v3.2 (2025-01-10)
Added Architectural Decisions section addressing Oracle review gaps:
- **TUI Role Decision**: Mission control, not embedded terminal
- **Persistence Invariants**: Events canonical, tasks derived, SQLite WAL mode
- **Git Safety Rails**: Preflight checks, path safety, file-scope locking
- **tmux I/O Strategy**: pipe-pane streaming, not capture-pane polling
- **Daemon Architecture**: Single-writer pattern with Unix socket IPC
- **MVP Scope**: Narrowed Phase 1-4 to core loop, deferred advanced features

### v3.1 (2025-01-10)
Added comprehensive patterns bootstrapped from 15+ AI coding agent projects:
- **Daemon mode** from claude-squad - background agent execution
- **Diff viewing UI** from claude-squad - review changes before commit
- **Push automation** from claude-squad - single-key commit + push + PR
- **Hierarchical config** from clark - user/project/CLI config layers
- **Session persistence** from clark - recover from crashes
- **Rich status icons** from clark - visual agent state
- **Hook system** from claude-code - lifecycle automation
- **MCP integration** from claude-code - extensible tool protocol
- **Cost tracking** from Aider - per-task token/cost visibility
- **RepoMap** from Aider - PageRank-based context selection
- **Per-task memory** from Windsurf - persist learnings
- **Checkpoint/resume** from LangGraph - crash recovery
- **Event sourcing** from OpenHands - audit trail
- **Agent roles** from Cursor 2.0 - specialized agents

### v3.0 (2025-01-10)
- Initial Go + Bubble Tea pivot from Python + Textual

---

## Executive Summary

Tandemonium is a terminal user interface for managing multiple AI coding agents from a single interface. Think "mission control for your agent fleet" crossed with "AI tech lead that preps your work."

**Key Innovation:** A two-phase workflow where a PM Agent refines vague requests into detailed specs before coding agents touch anything.

**Tech Stack Decision:** Go + Bubble Tea (proven stack used by claude-squad, vibemux, claude-pilot).

**Canonical Spec:** See [prd/tandemonium-spec.md](../../prd/tandemonium-spec.md) for the concise product specification. This document contains detailed implementation design.

---

## Prior Art Analysis

Four existing projects informed this design:

### claude-squad (Go + Bubble Tea + tmux) - PRIMARY REFERENCE
- tmux for session isolation - sessions persist beyond TUI
- Git worktrees for filesystem isolation per task
- Status enum: `Running`, `Ready`, `Loading`, `Paused`
- Hash-based change detection (SHA256 of pane content)
- Prompt detection for "trust files", "allow once", "yes/no"
- Pause/Resume: detach tmux, remove worktree, preserve branch

### vibemux (Go + Bubble Tea)
- Grid layout (2×2, 2×3, 3×3) for multi-agent view
- Profile system with per-session environment variables
- Dual-mode input: Control mode vs Terminal mode (F12 toggle)

### clark (Rust + Ratatui + tokio)
- Up to 4 concurrent instances
- SQLite for state persistence
- Invokes tmux as subprocess

### claude-pilot (Go + Bubble Tea + Cobra)
- Modular packages: core (API), shared (types), tui, cli
- Session attachment model - new Claude instances attach as panes
- JSON persistence for session metadata

### Patterns Adopted

| Pattern | Source | Application |
|---------|--------|-------------|
| tmux session isolation | claude-squad | Each agent gets own tmux session |
| Hash-based change detection | claude-squad | Detect when agent output updates |
| Git worktree per task | claude-squad, clark | Filesystem isolation |
| Dual-mode input | vibemux | Control mode vs Terminal passthrough |
| Prompt detection | claude-squad | Auto-accept trust prompts, detect blockers |
| Bubble Tea architecture | all three Go projects | Model-Update-View pattern |

---

## Core Differentiators

1. **PM Agent Refinement Layer**: Before coding agents touch anything, a PM agent transforms vague requests into detailed specs with clarifying questions, codebase research, and acceptance criteria.

2. **Task Queue as First-Class Primitive**: Not just session multiplexing—full task lifecycle with assign → refine → approve → execute → review → done.

3. **Structured Review Flow**: Spec review before coding, code review after—with diff views, approval workflows, and rejection feedback loops.

4. **Blocker Detection + Unblock UX**: When agents get stuck, surface their questions and let humans answer inline.

---

## The Two-Phase Workflow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           TANDEMONIUM                                   │
│                                                                         │
│   ┌─────────┐      ┌─────────────┐      ┌─────────┐      ┌─────────┐   │
│   │  ROUGH  │ ──▶  │  PM AGENT   │ ──▶  │ REFINED │ ──▶  │ CODING  │   │
│   │  TASK   │      │  (refine)   │      │  SPEC   │      │  AGENT  │   │
│   └─────────┘      └─────────────┘      └─────────┘      └─────────┘   │
│                           │                   │                 │       │
│                           ▼                   ▼                 ▼       │
│                    Human approves       Human approves    Human reviews │
│                    refined spec         before coding     completed PR  │
└─────────────────────────────────────────────────────────────────────────┘
```

**Phase 1 - Refinement**: PM Agent takes vague input, researches codebase, asks clarifying questions, produces structured spec.

**Phase 2 - Execution**: Coding Agent executes against the refined spec, with human review at completion.

---

## Technical Stack

- **Language**: Go 1.22+
- **TUI Framework**: Bubble Tea + Lip Gloss + Bubbles
- **Process Management**: tmux for session isolation (proven by claude-squad)
- **PM Agent**: Claude API (direct, with tool use)
- **State Persistence**: SQLite for tasks, sessions, and logs
- **Config**: TOML for project settings
- **Git Integration**: go-git for branch/worktree management

### Why Go + Bubble Tea

| Factor | Go Advantage |
|--------|--------------|
| Concurrency | Goroutines map perfectly to managing multiple agent streams |
| Proven | claude-squad, vibemux, claude-pilot all use this stack |
| Distribution | Single binary, no runtime dependencies |
| Ecosystem | Bubble Tea + Lip Gloss is mature and well-documented |
| Iteration speed | Fast compilation, good tooling |

### What Happens to Existing Rust Code

The existing ~9,500 lines of Rust code in `crates/` served the previous Tauri GUI + TUI hybrid vision. For this pivot:

- **Archive**: Move to `archive/rust-v1/` for reference
- **Port concepts**: Stream parsing heuristics, session lifecycle patterns
- **Don't port directly**: The code structure is too tied to Tauri/Ratatui

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Tandemonium TUI (Bubble Tea)                    │
├─────────────────────────────────────────────────────────────────────────┤
│  Fleet    │  Focus   │  Refine  │  Spec    │  Code    │  Queue         │
│  View     │  View    │  View    │  Review  │  Review  │  View          │
└───────────┴──────────┴──────────┴──────────┴──────────┴────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         Agent Manager                                   │
│  - Spawn/kill agent processes (Claude Code, Codex, Aider)              │
│  - Route messages to/from agents                                        │
│  - Track agent state (refining/working/blocked/review/idle)            │
│  - Monitor stdout/stderr streams via tmux capture-pane                  │
│  - Cost tracking per agent and per task                                 │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    ▼                               ▼
┌───────────────────────────────┐   ┌───────────────────────────────────┐
│        PM Agent               │   │        Coding Agents              │
│  - Claude API (direct)        │   │  - Claude Code CLI in tmux        │
│  - Codebase search tools      │   │  - capture-pane for output        │
│  - Clarification dialogs      │   │  - send-keys for input            │
│  - Spec generation            │   │  - Git worktree isolation         │
└───────────────────────────────┘   └───────────────────────────────────┘
```

---

## Data Models

### Task Status Flow

```
DRAFT → REFINING → PENDING_REVIEW → QUEUED → ASSIGNED → IN_PROGRESS → REVIEW → DONE
           ↓              ↓                        ↓          ↓
       (skippable)    REJECTED               BLOCKED    REJECTED
```

### Core Models

```go
type TaskStatus string

const (
    StatusDraft         TaskStatus = "draft"          // Just created, not yet refined
    StatusRefining      TaskStatus = "refining"       // PM agent working on it
    StatusPendingReview TaskStatus = "pending_review" // Spec ready for human approval
    StatusQueued        TaskStatus = "queued"         // Approved, waiting for coding agent
    StatusAssigned      TaskStatus = "assigned"       // Coding agent claimed it
    StatusInProgress    TaskStatus = "in_progress"    // Coding agent working
    StatusBlocked       TaskStatus = "blocked"        // Agent has a question
    StatusReview        TaskStatus = "review"         // Code complete, awaiting human review
    StatusDone          TaskStatus = "done"           // Approved and merged
    StatusRejected      TaskStatus = "rejected"       // Human rejected, needs rework
)

type Task struct {
    ID        string     `json:"id"`         // e.g., "TAND-42"
    RawInput  string     `json:"raw_input"`  // Original human input
    Status    TaskStatus `json:"status"`
    Priority  int        `json:"priority"`   // 1 = highest
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`

    // Refinement phase
    RefinedSpec        *RefinedSpec   `json:"refined_spec,omitempty"`
    Clarifications     []QAPair       `json:"clarifications,omitempty"`
    RefinementCostCents int           `json:"refinement_cost_cents"`
    RefinementSkipped  bool           `json:"refinement_skipped"`

    // Execution phase
    AssignedAgent      string         `json:"assigned_agent,omitempty"`
    ExecutionCostCents int            `json:"execution_cost_cents"`
    BranchName         string         `json:"branch_name,omitempty"`
    TmuxSession        string         `json:"tmux_session,omitempty"`

    // Completion
    FilesChanged  []string     `json:"files_changed,omitempty"`
    TestResults   *TestResults `json:"test_results,omitempty"`
    HumanFeedback string       `json:"human_feedback,omitempty"`
}

type RefinedSpec struct {
    Title               string        `json:"title"`
    Summary             string        `json:"summary"`              // 2-3 sentence overview
    Context             []ContextItem `json:"context"`              // Codebase research results
    Requirements        []string      `json:"requirements"`         // Functional requirements
    AcceptanceCriteria  []string      `json:"acceptance_criteria"`  // Testable conditions
    FilesToModify       []FilePlan    `json:"files_to_modify"`      // Planned file changes
    ImplementationNotes string        `json:"implementation_notes"` // Guidance for coding agent
    EstimatedComplexity string        `json:"estimated_complexity"` // "trivial", "low", "medium", "high"
    EstimatedMinutes    int           `json:"estimated_minutes"`

    // For audit/learning
    QuestionsAsked        int `json:"questions_asked"`
    CodebaseFilesExamined int `json:"codebase_files_examined"`
    ExternalSearches      int `json:"external_searches"`
}

type AgentType string

const (
    AgentTypePM    AgentType = "pm"    // Refinement agent (Claude API)
    AgentTypeCoder AgentType = "coder" // Execution agent (Claude Code CLI)
)

type AgentStatus string

const (
    AgentStatusIdle           AgentStatus = "idle"
    AgentStatusRefining       AgentStatus = "refining"        // PM agent mode
    AgentStatusWorking        AgentStatus = "working"
    AgentStatusBlocked        AgentStatus = "blocked"
    AgentStatusAwaitingReview AgentStatus = "awaiting_review"
)

type Agent struct {
    ID              string       `json:"id"`         // e.g., "claude-1"
    Type            AgentType    `json:"type"`
    Status          AgentStatus  `json:"status"`
    CurrentTask     *Task        `json:"current_task,omitempty"`
    TmuxSession     string       `json:"tmux_session,omitempty"`
    SessionLog      []LogEntry   `json:"session_log,omitempty"`
    CostCents       int          `json:"cost_cents"`
    StartedAt       *time.Time   `json:"started_at,omitempty"`
    WorkingDir      string       `json:"working_dir"`
    BranchName      string       `json:"branch_name,omitempty"`
    BlockedState    *BlockedState `json:"blocked_state,omitempty"`
    LastContentHash string       `json:"last_content_hash,omitempty"` // For change detection
}
```

---

## Tmux Session Management (from claude-squad)

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
    cmd := exec.Command("tmux", "new-session",
        "-d",           // Detached
        "-s", name,     // Session name
        "-c", workDir,  // Working directory
    )
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("failed to create session: %w", err)
    }

    // Wait for session to exist
    if err := waitForSession(name, 5*time.Second); err != nil {
        return nil, err
    }

    // Configure history limit
    exec.Command("tmux", "set-option", "-t", name, "history-limit", "50000").Run()

    // Send the command
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
    cmd := exec.Command("tmux", "capture-pane",
        "-t", s.Name,
        "-p",  // Print to stdout
        "-e",  // Include escape sequences
    )
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
```

---

## Prompt Detection (from claude-squad patterns)

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
        regexp.MustCompile(`\?\s*$`),
    }

    trustPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)trust (this|these) files?`),
        regexp.MustCompile(`(?i)allow (once|always)`),
        regexp.MustCompile(`(?i)\(y\)es.*\(n\)o`),
        regexp.MustCompile(`(?i)yes/no`),
        regexp.MustCompile(`(?i)press enter to continue`),
    }

    completionPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)task (complete|finished|done)`),
        regexp.MustCompile(`(?i)all tests pass`),
        regexp.MustCompile(`(?i)successfully (created|updated|implemented)`),
    }
)

// Detect analyzes pane content and returns the detected state
func Detect(content string) State {
    // Check last 10 lines
    lines := strings.Split(content, "\n")
    start := len(lines) - 10
    if start < 0 {
        start = 0
    }
    tail := strings.Join(lines[start:], "\n")

    for _, p := range trustPatterns {
        if p.MatchString(tail) {
            return StateTrustPrompt
        }
    }

    for _, p := range completionPatterns {
        if p.MatchString(tail) {
            return StateComplete
        }
    }

    for _, p := range questionPatterns {
        if p.MatchString(tail) {
            return StateBlocked
        }
    }

    return StateWorking
}
```

---

## PM Agent System

The PM Agent is the key differentiator. It's a Claude instance (via API) with specialized tools for task refinement.

```go
package pm

import (
    "context"
    "github.com/anthropics/anthropic-sdk-go"
)

var pmTools = []anthropic.Tool{
    {
        Name:        "search_codebase",
        Description: "Search the codebase for files, patterns, or text",
        InputSchema: anthropic.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "query":        map[string]string{"type": "string", "description": "search term or regex"},
                "file_pattern": map[string]string{"type": "string", "description": "glob pattern like '*.go'"},
                "max_results":  map[string]string{"type": "integer", "description": "default 10"},
            },
            Required: []string{"query"},
        },
    },
    {
        Name:        "read_file",
        Description: "Read the contents of a specific file",
        InputSchema: anthropic.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "path":       map[string]string{"type": "string", "description": "relative path to file"},
                "start_line": map[string]string{"type": "integer"},
                "end_line":   map[string]string{"type": "integer"},
            },
            Required: []string{"path"},
        },
    },
    {
        Name:        "list_directory",
        Description: "List files and directories at a path",
        InputSchema: anthropic.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "path":      map[string]string{"type": "string", "description": "relative path"},
                "recursive": map[string]string{"type": "boolean", "description": "default false"},
            },
            Required: []string{"path"},
        },
    },
    {
        Name:        "ask_clarification",
        Description: "Ask the human a clarifying question",
        InputSchema: anthropic.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "question": map[string]string{"type": "string", "description": "the question to ask"},
                "options":  map[string]string{"type": "array", "description": "multiple choice options"},
                "context":  map[string]string{"type": "string", "description": "why you're asking"},
            },
            Required: []string{"question"},
        },
    },
    {
        Name:        "submit_spec",
        Description: "Submit the refined specification for human review",
        InputSchema: anthropic.ToolInputSchema{
            Type: "object",
            Properties: map[string]interface{}{
                "title":               map[string]string{"type": "string"},
                "summary":             map[string]string{"type": "string"},
                "requirements":        map[string]string{"type": "array"},
                "acceptance_criteria": map[string]string{"type": "array"},
                "files_to_modify":     map[string]string{"type": "array"},
                "implementation_notes": map[string]string{"type": "string"},
                "estimated_complexity": map[string]string{"type": "string"},
                "estimated_minutes":   map[string]string{"type": "integer"},
            },
            Required: []string{"title", "summary", "requirements", "acceptance_criteria"},
        },
    },
}

const pmSystemPrompt = `You are a PM Agent that refines vague task requests into detailed specifications.

Your job:
1. Research the codebase to understand context
2. Ask clarifying questions when needed (max 3)
3. Produce a detailed spec with acceptance criteria

Be thorough but efficient. Don't over-research.`

type PMAgent struct {
    client     *anthropic.Client
    projectDir string
    onQuestion func(question string, options []string) string // callback for human answers
}

func (p *PMAgent) Refine(ctx context.Context, rawInput string) (*RefinedSpec, error) {
    messages := []anthropic.MessageParam{
        anthropic.NewUserMessage(anthropic.NewTextBlock("Refine this task:\n\n" + rawInput)),
    }

    for {
        resp, err := p.client.Messages.Create(ctx, anthropic.MessageCreateParams{
            Model:     anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
            MaxTokens: anthropic.Int(4096),
            System:    anthropic.F([]anthropic.TextBlockParam{anthropic.NewTextBlock(pmSystemPrompt)}),
            Tools:     anthropic.F(pmTools),
            Messages:  anthropic.F(messages),
        })
        if err != nil {
            return nil, err
        }

        // Process tool calls
        for _, block := range resp.Content {
            if block.Type == anthropic.ContentBlockTypeToolUse {
                toolResult := p.executeTool(block.Name, block.Input)

                messages = append(messages,
                    anthropic.NewAssistantMessage(resp.Content...),
                    anthropic.NewUserMessage(anthropic.NewToolResultBlock(block.ID, toolResult, false)),
                )

                if block.Name == "submit_spec" {
                    return parseSpec(block.Input), nil
                }
            }
        }

        if resp.StopReason == anthropic.MessageStopReasonEndTurn {
            break
        }
    }

    return nil, fmt.Errorf("PM agent did not produce a spec")
}

func (p *PMAgent) executeTool(name string, input map[string]interface{}) string {
    switch name {
    case "search_codebase":
        return p.searchCodebase(input)
    case "read_file":
        return p.readFile(input)
    case "list_directory":
        return p.listDirectory(input)
    case "ask_clarification":
        question := input["question"].(string)
        var options []string
        if opts, ok := input["options"].([]interface{}); ok {
            for _, o := range opts {
                options = append(options, o.(string))
            }
        }
        return p.onQuestion(question, options)
    default:
        return "unknown tool"
    }
}
```

---

## Core Views

### Fleet View (Default)

```
┌─ TANDEMONIUM ───────────────────────────────────────────────────────────┐
│ myproject    4 agents    $14.82 today    1 refining  2 awaiting review  │
├─────────────────────────────────────────────────────────────────────────┤
│ AGENTS                                                                  │
│                                                                         │
│  ◈ pm-1     REFINING  Add rate limiting...             3m   $0.24      │
│    └─ Researching existing middleware patterns                          │
│                                                                         │
│  ● claude-1 WORKING   Parse YAML frontmatter          12m   $0.84  ██▓░│
│    └─ "Adding error handling for nested structures"                     │
│                                                                         │
│  ◐ claude-2 BLOCKED   Implement caching layer          8m   $0.62      │
│    └─ "Redis or in-memory for dev environment?"                         │
│                                                                         │
│  ◉ claude-3 REVIEW    Add retry logic                  —    $1.20      │
│    └─ 3 files, +94 -12, tests passing                                   │
│                                                                         │
│  ○ claude-4 IDLE      —                                —    —          │
│                                                                         │
├─────────────────────────────────────────────────────────────────────────┤
│ PENDING SPECS (need your approval)                                      │
│  › TAND-51  Refactor config loading        P2   [Enter] to review      │
│                                                                         │
│ TASK QUEUE (approved, ready to assign)                                  │
│    TAND-47  Write integration tests        ~20m   P1                   │
│                                                                         │
├─────────────────────────────────────────────────────────────────────────┤
│ [1-4] focus  [n]ew task  [p]ending specs  [a]ssign  [r]eview  [q]uit   │
└─────────────────────────────────────────────────────────────────────────┘
```

**Status indicators:**
- `◈` Refining (purple) - PM agent working
- `●` Working (green) - Coding agent executing
- `◐` Blocked (yellow) - Needs human input
- `◉` Awaiting review (blue) - Code complete
- `○` Idle (dim)

### Additional Views

| View | Purpose |
|------|---------|
| **New Task Modal** | Quick task entry with refinement toggle |
| **Refine View** | Watch PM agent research + answer questions |
| **Spec Review** | Review refined spec before coding |
| **Focus View** | Single coding agent activity log |
| **Code Review** | Review diffs, approve/reject work |
| **Unblock Modal** | Answer blocked agent questions |
| **Task Queue** | Full task management with filters |

---

## Git Integration

```go
package git

import (
    "fmt"
    "os/exec"
    "path/filepath"
    "strings"
)

type Manager struct {
    RepoDir      string
    WorktreeBase string
}

func NewManager(repoDir string) *Manager {
    base := filepath.Join(repoDir, ".tandemonium", "worktrees")
    return &Manager{RepoDir: repoDir, WorktreeBase: base}
}

// CreateWorktree creates an isolated worktree for a task
func (m *Manager) CreateWorktree(taskID string) (worktreePath, branchName string, err error) {
    branchName = fmt.Sprintf("tand/%s", strings.ToLower(taskID))
    worktreePath = filepath.Join(m.WorktreeBase, strings.ToLower(taskID))

    cmd := exec.Command("git", "worktree", "add", "-b", branchName, worktreePath)
    cmd.Dir = m.RepoDir
    if err := cmd.Run(); err != nil {
        return "", "", fmt.Errorf("failed to create worktree: %w", err)
    }

    return worktreePath, branchName, nil
}

// CleanupWorktree removes worktree after task completion
func (m *Manager) CleanupWorktree(taskID string) error {
    worktreePath := filepath.Join(m.WorktreeBase, strings.ToLower(taskID))
    cmd := exec.Command("git", "worktree", "remove", worktreePath)
    cmd.Dir = m.RepoDir
    return cmd.Run()
}

// GetDiff gets diff between branch and main
func (m *Manager) GetDiff(branchName string) (string, error) {
    cmd := exec.Command("git", "diff", "main", branchName)
    cmd.Dir = m.RepoDir
    out, err := cmd.Output()
    return string(out), err
}

// MergeBranch merges branch into main
func (m *Manager) MergeBranch(branchName string) error {
    cmds := [][]string{
        {"git", "checkout", "main"},
        {"git", "merge", "--no-ff", branchName, "-m", fmt.Sprintf("Merge %s", branchName)},
        {"git", "branch", "-d", branchName},
    }

    for _, args := range cmds {
        cmd := exec.Command(args[0], args[1:]...)
        cmd.Dir = m.RepoDir
        if err := cmd.Run(); err != nil {
            return err
        }
    }
    return nil
}
```

---

## Configuration

`tandemonium.toml`:

```toml
[project]
name = "myproject"
working_directory = "."
task_prefix = "TAND"

[agents]
max_concurrent = 4
default_coder = "claude"           # "claude", "codex", "aider"
coder_model = "claude-sonnet-4-20250514"
pm_model = "claude-sonnet-4-20250514"

[costs]
daily_limit_cents = 2000           # $20/day
per_task_limit_cents = 500         # $5/task
per_refinement_limit_cents = 100   # $1/refinement
alert_threshold = 0.8

[refinement]
enabled = true
auto_skip_trivial = true

[git]
enabled = true
use_worktrees = true
branch_prefix = "tand"
auto_merge_on_approve = false

[ui]
refresh_rate_ms = 250
theme = "dark"
```

---

## Implementation Phases

### Phase 1: Foundation (Week 1-2)
- [ ] Go module setup with Bubble Tea
- [ ] Basic TUI with Fleet View skeleton
- [ ] TmuxSession wrapper for coding agents
- [ ] Single coding agent spawn and stream capture
- [ ] Task model with SQLite persistence
- [ ] Simple task creation (no refinement yet)
- [ ] Focus View with log display

### Phase 2: PM Agent (Week 3-4)
- [ ] PM Agent with Claude API integration
- [ ] Codebase search tools (ripgrep wrapper)
- [ ] Clarification dialog UI (Refine View)
- [ ] RefinedSpec model
- [ ] Spec Review View
- [ ] Refinement → Coding handoff

### Phase 3: Multi-Agent (Week 5-6)
- [ ] Support 2-4 concurrent coding agents
- [ ] Prompt detection (trust dialogs, blockers)
- [ ] Auto-accept for trust prompts
- [ ] Unblock Modal
- [ ] Git worktree integration
- [ ] Cost tracking and limits

### Phase 4: Review Flow (Week 7-8)
- [ ] Code Review View with diff display
- [ ] Approval/rejection workflows
- [ ] Rejection feedback loop to agent
- [ ] Task Queue View with filters
- [ ] Config file support
- [ ] Session recovery on restart

### Phase 5: Polish (Week 9-10)
- [ ] Desktop notifications
- [ ] Keyboard shortcut refinement
- [ ] Documentation
- [ ] Error handling and edge cases
- [ ] Performance optimization

---

## File Structure

```
tandemonium/
├── go.mod
├── go.sum
├── main.go
├── tandemonium.toml.example
│
├── cmd/
│   └── tandemonium/
│       └── main.go              # Entry point
│
├── internal/
│   ├── app/
│   │   ├── app.go               # Main Bubble Tea model
│   │   ├── messages.go          # Custom messages
│   │   └── keys.go              # Key bindings
│   │
│   ├── models/
│   │   ├── task.go              # Task, RefinedSpec
│   │   ├── agent.go             # Agent, AgentStatus
│   │   └── log.go               # LogEntry, BlockedState
│   │
│   ├── views/
│   │   ├── fleet.go             # Fleet View
│   │   ├── focus.go             # Focus View
│   │   ├── refine.go            # Refine View (PM agent)
│   │   ├── spec_review.go       # Spec Review
│   │   ├── code_review.go       # Code Review
│   │   ├── queue.go             # Task Queue
│   │   └── modals.go            # New task, unblock
│   │
│   ├── agents/
│   │   ├── manager.go           # Agent lifecycle
│   │   ├── pm.go                # PM Agent (Claude API)
│   │   ├── tmux.go              # tmux wrapper
│   │   └── detector.go          # Prompt detection
│   │
│   ├── services/
│   │   ├── store.go             # SQLite persistence
│   │   ├── git.go               # Git/worktree ops
│   │   └── costs.go             # Cost tracking
│   │
│   └── config/
│       └── config.go            # TOML config
│
└── tests/
    └── ...
```

---

## Risk Mitigation

| Risk | Severity | Mitigation |
|------|----------|------------|
| tmux not installed | High | Detect at startup, show install instructions |
| PM Agent over-researches | Medium | Time/cost limits on refinement phase |
| Claude Code output format changes | Medium | Fallback to raw text, tune heuristics |
| Blocked detection false positives | Medium | User can manually set status |
| Session orphaning on crash | Low | tmux sessions persist; implement recovery |

---

## Success Criteria

1. **Refinement quality**: PM-refined tasks have 50%+ fewer back-and-forth cycles
2. **Multi-agent efficiency**: Run 4 agents simultaneously without UI lag
3. **Blocker handling**: Surface blocked questions within 5 seconds
4. **Review workflow**: Review and approve completed task in <2 minutes
5. **Cost visibility**: Always know spending, alerts before limits hit
6. **Persistence**: Survive restarts, resume sessions, never lose task state

---

## Architectural Decisions (v3.2)

This section addresses gaps identified in Oracle review, incorporating lessons from the existing Rust implementation.

### Decision 1: TUI Role — Mission Control, Not Terminal Emulator

**Decision:** Bubble Tea renders summaries and status, NOT embedded terminal output.

**Rationale:** The Rust implementation built a full PTY + terminal emulator stack (`portable-pty` + `wezterm-term`) and paid significant complexity costs. tmux already IS a terminal multiplexer.

**Implications:**
- TUI shows: agent status, last N lines (plain text), detected prompts, task queue
- TUI does NOT show: full ANSI rendering, cursor state, interactive terminal
- User presses `Enter` or `f` to `tmux attach -t session` for full interaction
- No `-e` flag on capture-pane; strip ANSI or use plain text summaries

```go
// What we render in Fleet View
type AgentSummary struct {
    ID          string
    Status      AgentStatus
    TaskTitle   string
    LastLines   []string    // Last 5-10 lines, plain text
    DetectedState PromptState // working, blocked, trust_prompt, complete
    Duration    time.Duration
    CostCents   int
}

// NOT trying to render full terminal state
```

### Decision 2: Persistence Invariants

**Decision:** Events are canonical; task state is derived. SQLite with WAL mode.

**Rationale:** Event sourcing provides audit trail, replay capability, and debugging. Derived state can be rebuilt. Single source of truth prevents divergence.

**Specifics:**

```go
// Schema version tracking
CREATE TABLE schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

// Events are append-only, canonical
CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL,
    agent_id TEXT,
    event_type TEXT NOT NULL,
    payload JSON NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_events_task ON events(task_id);
CREATE INDEX idx_events_type ON events(event_type);

// Tasks table is derived/cached, rebuilt from events on startup
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    raw_input TEXT,
    refined_spec JSON,
    assigned_agent TEXT,
    branch_name TEXT,
    cost_cents INTEGER DEFAULT 0,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    rev INTEGER DEFAULT 0  -- optimistic concurrency
);

// Sessions for recovery
CREATE TABLE sessions (
    agent_id TEXT PRIMARY KEY,
    tmux_session TEXT NOT NULL,
    task_id TEXT,
    status TEXT NOT NULL,
    working_dir TEXT,
    started_at TIMESTAMP,
    last_heartbeat TIMESTAMP,
    terminated_at TIMESTAMP  -- NULL if still active
);
```

**SQLite Configuration:**
```go
func OpenDB(path string) (*sql.DB, error) {
    db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
    if err != nil {
        return nil, err
    }
    // Single connection for writes (daemon is single writer)
    db.SetMaxOpenConns(1)
    return db, nil
}
```

**Migration Strategy:**
```go
var migrations = []Migration{
    {Version: 1, SQL: schemaV1},
    {Version: 2, SQL: "ALTER TABLE tasks ADD COLUMN priority INTEGER DEFAULT 0"},
    // etc.
}

func Migrate(db *sql.DB) error {
    current := getCurrentVersion(db)
    for _, m := range migrations {
        if m.Version > current {
            if _, err := db.Exec(m.SQL); err != nil {
                return fmt.Errorf("migration %d failed: %w", m.Version, err)
            }
            setVersion(db, m.Version)
        }
    }
    return nil
}
```

**Retention Policy:**
- Events: Keep 30 days, then archive to compressed files
- Session logs: Keep 7 days after task completion
- Rebuild tasks table from events on startup (fast for <10k events)

### Decision 3: Git Safety Rails

**Decision:** Port Rust's preflight checks, path safety, and file-scope locking.

**Rationale:** Multi-agent parallel development WILL cause conflicts without guardrails. The Rust code already solved this.

```go
package git

import (
    "errors"
    "os/exec"
    "path/filepath"
    "strings"
)

var (
    ErrDirtyRepo     = errors.New("repository has uncommitted changes")
    ErrPathEscape    = errors.New("path escapes repository root")
    ErrWorktreeExists = errors.New("worktree already exists for this task")
    ErrBranchExists  = errors.New("branch already exists")
    ErrLowDiskSpace  = errors.New("insufficient disk space")
)

// Preflight runs all safety checks before worktree creation
func (m *Manager) Preflight(taskID string) error {
    // 1. Check repo is clean (or allow with flag)
    if dirty, err := m.isDirty(); err != nil {
        return err
    } else if dirty {
        return ErrDirtyRepo
    }

    // 2. Check disk space (need at least 500MB)
    if free, _ := m.freeDiskSpace(); free < 500*1024*1024 {
        return ErrLowDiskSpace
    }

    // 3. Check branch doesn't exist
    branchName := m.branchName(taskID)
    if m.branchExists(branchName) {
        return ErrBranchExists
    }

    // 4. Check worktree doesn't exist
    worktreePath := m.worktreePath(taskID)
    if m.worktreeExists(worktreePath) {
        return ErrWorktreeExists
    }

    return nil
}

// DefaultBranch detects main/master/trunk
func (m *Manager) DefaultBranch() string {
    // Try remote HEAD first
    out, err := exec.Command("git", "-C", m.RepoDir, "symbolic-ref", "refs/remotes/origin/HEAD").Output()
    if err == nil {
        // refs/remotes/origin/main -> main
        parts := strings.Split(strings.TrimSpace(string(out)), "/")
        return parts[len(parts)-1]
    }
    // Fallback: check for main, then master
    for _, branch := range []string{"main", "master", "trunk"} {
        if m.branchExists(branch) {
            return branch
        }
    }
    return "main" // default assumption
}

// SafePath validates path doesn't escape repo
func (m *Manager) SafePath(path string) error {
    abs, err := filepath.Abs(filepath.Join(m.RepoDir, path))
    if err != nil {
        return err
    }
    repoAbs, _ := filepath.Abs(m.RepoDir)
    if !strings.HasPrefix(abs, repoAbs) {
        return ErrPathEscape
    }
    return nil
}

// PathLock prevents concurrent agents from modifying same files
type PathLockManager struct {
    mu    sync.RWMutex
    locks map[string]string // path pattern -> agent_id
}

func (p *PathLockManager) Acquire(agentID string, patterns []string) error {
    p.mu.Lock()
    defer p.mu.Unlock()

    // Check for conflicts
    for _, pattern := range patterns {
        for existing, holder := range p.locks {
            if p.overlaps(pattern, existing) && holder != agentID {
                return fmt.Errorf("path %s conflicts with %s held by %s", pattern, existing, holder)
            }
        }
    }

    // Acquire locks
    for _, pattern := range patterns {
        p.locks[pattern] = agentID
    }
    return nil
}

func (p *PathLockManager) Release(agentID string) {
    p.mu.Lock()
    defer p.mu.Unlock()
    for pattern, holder := range p.locks {
        if holder == agentID {
            delete(p.locks, pattern)
        }
    }
}
```

### Decision 4: tmux I/O Strategy — Streaming, Not Polling

**Decision:** Use `tmux pipe-pane` for streaming output, not `capture-pane` polling.

**Rationale:** Polling 4 agents at 250ms = 16 subprocess spawns/second. Streaming is more efficient and lower latency.

```go
package tmux

import (
    "bufio"
    "os"
    "os/exec"
)

// Session with streaming output
type Session struct {
    Name       string
    WorkDir    string
    OutputPipe string // Named pipe path
    outputChan chan string
    done       chan struct{}
}

// Create creates session with output streaming
func Create(agentID, workDir, command string) (*Session, error) {
    name := SessionPrefix + agentID
    pipePath := filepath.Join(os.TempDir(), "tandemonium", name+".pipe")

    // Create named pipe
    os.MkdirAll(filepath.Dir(pipePath), 0755)
    syscall.Mkfifo(pipePath, 0644)

    // Create detached session
    cmd := exec.Command("tmux", "new-session",
        "-d", "-s", name, "-c", workDir,
    )
    if err := cmd.Run(); err != nil {
        return nil, err
    }

    // Pipe pane output to named pipe
    exec.Command("tmux", "pipe-pane", "-t", name, "-o", "cat >> "+pipePath).Run()

    // Send the command
    if command != "" {
        exec.Command("tmux", "send-keys", "-t", name, command, "Enter").Run()
    }

    s := &Session{
        Name:       name,
        WorkDir:    workDir,
        OutputPipe: pipePath,
        outputChan: make(chan string, 1000),
        done:       make(chan struct{}),
    }

    // Start streaming goroutine
    go s.streamOutput()

    return s, nil
}

func (s *Session) streamOutput() {
    file, err := os.Open(s.OutputPipe)
    if err != nil {
        return
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        select {
        case s.outputChan <- scanner.Text():
        case <-s.done:
            return
        }
    }
}

// Output returns channel for streaming output
func (s *Session) Output() <-chan string {
    return s.outputChan
}

// LastLines returns recent output for display (with ring buffer)
func (s *Session) LastLines(n int) []string {
    // Implementation uses internal ring buffer updated by streamOutput
}
```

**Fallback:** If pipe-pane causes issues, fall back to capture-pane at 1s intervals (not 250ms) for status checks only.

### Decision 5: Daemon Architecture — Single Writer

**Decision:** Daemon is the single writer to SQLite. TUI is read-mostly client.

**Rationale:** Eliminates need for optimistic locking, retry logic, and conflict resolution at every write point.

```go
package daemon

import (
    "encoding/json"
    "net"
    "os"
)

const SocketPath = "/tmp/tandemonium.sock"

// Command types from TUI to daemon
type Command struct {
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload"`
}

// Commands:
// - create_task {raw_input, skip_refinement}
// - assign_task {task_id, agent_id}
// - answer_blocker {task_id, response}
// - approve_spec {task_id}
// - reject_spec {task_id, feedback}
// - approve_code {task_id}
// - reject_code {task_id, feedback}
// - kill_agent {agent_id}

// Event types from daemon to TUI
type Event struct {
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload"`
}

// Events:
// - task_updated {task}
// - agent_updated {agent}
// - output_line {agent_id, line}
// - prompt_detected {agent_id, prompt_type}
// - cost_warning {message, current, limit}

// Server handles TUI connections
type Server struct {
    listener net.Listener
    store    *Store
    agents   *AgentManager
    clients  map[net.Conn]struct{}
}

func (s *Server) handleConnection(conn net.Conn) {
    defer conn.Close()

    // Send initial snapshot
    snapshot := s.store.Snapshot()
    json.NewEncoder(conn).Encode(Event{Type: "snapshot", Payload: marshal(snapshot)})

    // Subscribe to event stream
    events := s.store.Subscribe()
    defer s.store.Unsubscribe(events)

    // Handle commands and forward events
    go s.forwardEvents(conn, events)
    s.handleCommands(conn)
}

// Client connects TUI to daemon
type Client struct {
    conn   net.Conn
    events chan Event
}

func Connect() (*Client, error) {
    conn, err := net.Dial("unix", SocketPath)
    if err != nil {
        return nil, err
    }
    c := &Client{conn: conn, events: make(chan Event, 100)}
    go c.readEvents()
    return c, nil
}

func (c *Client) SendCommand(cmd Command) error {
    return json.NewEncoder(c.conn).Encode(cmd)
}

func (c *Client) Events() <-chan Event {
    return c.events
}
```

**No Daemon Mode (v1 Option):** If daemon adds too much complexity for MVP, the TUI can be the single writer directly. But then background execution is not possible.

### Decision 6: Prompt Detection Safety

**Decision:** Tiered detection with human confirmation for dangerous actions.

**Rationale:** False positive auto-actions can be catastrophic (auto-approving wrong things).

```go
package detector

type PromptTier int

const (
    TierSafe   PromptTier = iota // Auto-action OK (e.g., "press enter to continue")
    TierNotify                    // Notify user, no auto-action
    TierDanger                    // Require explicit confirmation
)

type DetectedPrompt struct {
    Type       PromptType
    Tier       PromptTier
    Confidence float64   // 0.0-1.0
    RawText    string
}

var promptRules = []PromptRule{
    // Safe tier - can auto-respond
    {Pattern: `(?i)press enter to continue`, Type: PromptContinue, Tier: TierSafe},
    {Pattern: `(?i)trust.*files?\s*\?`, Type: PromptTrust, Tier: TierSafe},

    // Notify tier - surface to user
    {Pattern: `(?i)(which|what|how)\s+.*\?`, Type: PromptQuestion, Tier: TierNotify},
    {Pattern: `(?i)should i`, Type: PromptQuestion, Tier: TierNotify},

    // Danger tier - never auto-respond
    {Pattern: `(?i)delete|remove|destroy`, Type: PromptDestructive, Tier: TierDanger},
    {Pattern: `(?i)overwrite|replace`, Type: PromptDestructive, Tier: TierDanger},
    {Pattern: `(?i)yes.*no.*\?`, Type: PromptYesNo, Tier: TierDanger},
}

func (d *Detector) Detect(text string) *DetectedPrompt {
    // Only check last 20 lines
    lines := lastN(strings.Split(text, "\n"), 20)
    tail := strings.Join(lines, "\n")

    for _, rule := range promptRules {
        if rule.Pattern.MatchString(tail) {
            return &DetectedPrompt{
                Type:       rule.Type,
                Tier:       rule.Tier,
                Confidence: 0.8, // Adjust based on match quality
                RawText:    tail,
            }
        }
    }
    return nil
}
```

### Decision 7: Config Discovery from Worktrees

**Decision:** Walk upward to find project root, supporting commands run from within worktrees.

```go
package config

import (
    "os"
    "path/filepath"
)

const ConfigDir = ".tandemonium"

// FindProjectRoot walks upward to find .tandemonium directory
func FindProjectRoot(startDir string) (string, error) {
    dir := startDir
    for {
        configPath := filepath.Join(dir, ConfigDir)
        if info, err := os.Stat(configPath); err == nil && info.IsDir() {
            return dir, nil
        }

        parent := filepath.Dir(dir)
        if parent == dir {
            return "", fmt.Errorf("no %s directory found", ConfigDir)
        }
        dir = parent
    }
}

// Load finds project root and loads hierarchical config
func Load() (*Config, error) {
    cwd, _ := os.Getwd()
    projectRoot, err := FindProjectRoot(cwd)
    if err != nil {
        return nil, err
    }

    cfg := DefaultConfig()

    // Layer 1: User config
    if home, _ := os.UserHomeDir(); home != "" {
        loadTOML(filepath.Join(home, ".config", "tandemonium", "config.toml"), cfg)
    }

    // Layer 2: Project config
    loadTOML(filepath.Join(projectRoot, ConfigDir, "config.toml"), cfg)

    // Layer 3: Environment variables
    loadEnv(cfg)

    cfg.ProjectRoot = projectRoot
    return cfg, nil
}
```

### Decision 8: Migration from Existing Data

**Decision:** One-way import from `.tandemonium/tasks.yml` to SQLite on first run.

```go
func MigrateFromYAML(projectRoot string) error {
    yamlPath := filepath.Join(projectRoot, ".tandemonium", "tasks.yml")
    if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
        return nil // No YAML to migrate
    }

    // Read existing YAML
    data, err := os.ReadFile(yamlPath)
    if err != nil {
        return err
    }

    var yamlData struct {
        Tasks []YAMLTask `yaml:"tasks"`
    }
    if err := yaml.Unmarshal(data, &yamlData); err != nil {
        return err
    }

    // Convert to events and replay
    for _, t := range yamlData.Tasks {
        event := Event{
            Type:    EventTaskCreated,
            TaskID:  t.ID,
            Payload: marshal(TaskCreatedPayload{...}),
        }
        store.AppendEvent(event)
    }

    // Rename YAML to .bak
    os.Rename(yamlPath, yamlPath+".migrated.bak")

    return nil
}
```

---

## MVP Scope (Revised)

Based on Oracle review, the 12-week timeline requires scope narrowing.

### MVP (Weeks 1-8): Core Loop

**Must ship:**
- Task creation → PM refinement → spec approval → assign coder → blocked/unblock → review diff → done
- Single coding agent (expand to 2-4 in polish phase)
- Git worktree per task with safety rails
- tmux session lifecycle + streaming output
- SQLite persistence (events canonical)
- Fleet View + Focus View + Spec Review + Code Review
- Basic prompt detection (notify tier only, no auto-actions)
- Config file support

**Explicitly NOT in MVP:**
- Daemon mode (TUI is single process)
- MCP integration
- RepoMap / tree-sitter
- Checkpoint/resume beyond tmux persistence
- Push automation / PR creation
- Per-task memory
- Agent roles/profiles
- Hook system

### Post-MVP (Weeks 9-12): Polish + One Advanced Feature

**Choose one:**
- Daemon mode for background execution, OR
- Multi-agent (4 concurrent) with path locking, OR
- Push automation with PR creation

**Plus:**
- Error handling hardening
- Performance optimization
- Documentation
- Comprehensive testing

---

## Bootstrapped Patterns from Prior Art

The following patterns are bootstrapped from research across 15+ AI coding agent projects.

### From claude-squad: Daemon Architecture

**Purpose:** Allow agents to continue working when TUI is closed.

```go
package daemon

import (
    "os"
    "os/exec"
    "syscall"
)

type DaemonConfig struct {
    PIDFile    string // e.g., ~/.tandemonium/daemon.pid
    LogFile    string // e.g., ~/.tandemonium/daemon.log
    AutoResume bool   // Resume tasks on daemon start
}

// StartDaemon forks and detaches the agent manager
func StartDaemon(cfg DaemonConfig) error {
    // Write PID file
    // Redirect stdout/stderr to log file
    // Detach from terminal
}

// AttachTUI reconnects TUI to running daemon
func AttachTUI() (*DaemonClient, error) {
    // Read PID file
    // Connect via Unix socket
    // Stream updates to TUI
}
```

**UI Integration:**
- `d` key toggles daemon mode
- Status bar shows "Daemon: Running (3 agents)"
- TUI can safely close without killing agents

### From claude-squad: Diff Viewing UI

**Purpose:** Review agent changes before committing.

```go
type DiffView struct {
    Mode        DiffMode // Preview vs Diff
    Files       []FileDiff
    Selected    int
    ScrollPos   int
}

type DiffMode string
const (
    DiffModePreview DiffMode = "preview" // Shows final file state
    DiffModeDiff    DiffMode = "diff"    // Shows +/- changes
)

type FileDiff struct {
    Path     string
    OldLines []Line
    NewLines []Line
    Hunks    []DiffHunk
}
```

**Key Bindings:**
- `Tab` - Toggle preview/diff mode
- `j/k` - Navigate hunks
- `a` - Accept all changes
- `r` - Reject and request revision

### From claude-squad: Push Automation

**Purpose:** Single-key commit + push + PR creation.

```go
// PushWorkflow handles commit → push → PR flow
func (g *Manager) PushWorkflow(taskID string, commitMsg string) error {
    // 1. Stage all changes in worktree
    // 2. Commit with auto-generated message
    // 3. Push branch to remote
    // 4. Optionally create PR via gh CLI
}
```

**Key Binding:** `s` commits and pushes current agent's work

### From clark: Structured Config File

**Purpose:** User customization with sensible defaults.

```toml
# ~/.config/tandemonium/config.toml

[general]
max_agents = 5
default_model = "claude-sonnet-4-20250514"
auto_cleanup = true           # Remove worktrees after merge
log_level = "info"

[ui]
theme = "dark"                # dark, light, solarized
show_timestamps = true
animations = true
refresh_rate_ms = 250

[git]
worktree_prefix = "tandemonium-"
auto_commit = false           # Auto-commit on agent completion
branch_prefix = "tand"
auto_merge_on_approve = false

[agents]
health_check_interval = "30s"
restart_on_failure = true
max_restart_attempts = 3
idle_timeout = "30m"          # Kill idle agents after this

[costs]
daily_limit_cents = 2000
per_task_limit_cents = 500
per_refinement_limit_cents = 100
alert_threshold = 0.8

[daemon]
enabled = false
auto_start = false
log_retention_days = 7
```

**Config loading priority:**
1. CLI flags (highest)
2. Environment variables (`TANDEMONIUM_*`)
3. Project config (`.tandemonium/config.toml`)
4. User config (`~/.config/tandemonium/config.toml`)
5. Defaults (lowest)

### From clark: Rich Session States with Icons

**Purpose:** Visual clarity in agent status.

```go
type AgentState struct {
    Status      AgentStatus
    Icon        string
    Color       string
    Description string
}

var AgentStates = map[AgentStatus]AgentState{
    AgentStatusIdle:     {Icon: "○", Color: "dim",    Description: "Waiting for task"},
    AgentStatusWorking:  {Icon: "●", Color: "green",  Description: "Working"},
    AgentStatusRefining: {Icon: "◈", Color: "purple", Description: "PM Agent refining"},
    AgentStatusBlocked:  {Icon: "◐", Color: "yellow", Description: "Needs input"},
    AgentStatusReview:   {Icon: "◉", Color: "blue",   Description: "Awaiting review"},
    AgentStatusFailed:   {Icon: "✕", Color: "red",    Description: "Failed"},
    AgentStatusPaused:   {Icon: "⏸", Color: "dim",    Description: "Paused"},
    AgentStatusStarting: {Icon: "⟳", Color: "cyan",   Description: "Starting"},
}
```

### From clark: Session Persistence

**Purpose:** Resume after TUI restart or crash.

```go
type SessionStore interface {
    // Save session state for recovery
    SaveSession(agentID string, state *AgentState) error

    // Load all active sessions on startup
    LoadActiveSessions() ([]*AgentState, error)

    // Mark session as cleanly terminated
    EndSession(agentID string) error
}

// SQLite implementation
type SQLiteSessionStore struct {
    db *sql.DB
}

// On startup: check for orphaned sessions and offer to resume
func (s *SQLiteSessionStore) RecoverOrphanedSessions() ([]*AgentState, error) {
    // Find sessions that weren't cleanly terminated
    // Check if tmux sessions still exist
    // Offer to reattach or cleanup
}
```

**Recovery flow:**
1. On startup, check `sessions` table for non-terminated entries
2. For each: check if tmux session exists
3. If exists: offer to reattach
4. If not: mark as terminated, cleanup worktree

### From claude-code: Hook System

**Purpose:** Custom automation on task lifecycle events.

```go
type HookEvent string

const (
    HookPreRefine       HookEvent = "pre_refine"       // Before PM Agent starts
    HookPostRefine      HookEvent = "post_refine"      // After spec generated
    HookPreAssign       HookEvent = "pre_assign"       // Before coding agent assigned
    HookPostAssign      HookEvent = "post_assign"      // After agent starts
    HookOnBlocked       HookEvent = "on_blocked"       // When agent gets stuck
    HookPreComplete     HookEvent = "pre_complete"     // Before marking done
    HookPostComplete    HookEvent = "post_complete"    // After task done
    HookOnCostThreshold HookEvent = "on_cost_threshold" // Cost limit warning
)

type Hook struct {
    Event    HookEvent `toml:"event"`
    Command  string    `toml:"command"`
    Blocking bool      `toml:"blocking"` // Wait for completion
    Timeout  string    `toml:"timeout"`  // e.g., "30s"
}

// Exit codes:
// 0: Success, continue
// 2: Block - halt and show stderr
// Other: Warning, continue

type HookResult struct {
    ExitCode int
    Stdout   string
    Stderr   string
    Decision string // "allow", "block", "modify"
}
```

**Configuration:**
```toml
# .tandemonium/hooks.toml

[[hooks]]
event = "post_refine"
command = "./scripts/validate-spec.sh"
blocking = true
timeout = "30s"

[[hooks]]
event = "pre_complete"
command = "./scripts/run-tests.sh"
blocking = true

[[hooks]]
event = "on_cost_threshold"
command = "./scripts/notify-slack.sh"
blocking = false
```

### From claude-code: MCP Integration

**Purpose:** Abstract tool calls through standard protocol.

```go
package mcp

// MCPServer wraps tool execution behind MCP protocol
type MCPServer interface {
    ListTools() []Tool
    CallTool(name string, params map[string]interface{}) (Result, error)
}

// BuiltinMCPServer provides core tools
type BuiltinMCPServer struct {
    projectDir string
}

func (s *BuiltinMCPServer) ListTools() []Tool {
    return []Tool{
        {Name: "search_codebase", Description: "Search for code patterns"},
        {Name: "read_file", Description: "Read file contents"},
        {Name: "write_file", Description: "Write to file"},
        {Name: "run_command", Description: "Execute shell command"},
        {Name: "git_status", Description: "Get git status"},
        {Name: "git_diff", Description: "Get git diff"},
    }
}
```

**Configuration:**
```json
// .tandemonium/mcp.json
{
  "servers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {"GITHUB_TOKEN": "..."}
    },
    "postgres": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-postgres"],
      "env": {"DATABASE_URL": "..."}
    }
  }
}
```

### From Aider: Cost Tracking Per Task

**Purpose:** Fine-grained spending visibility.

```go
type CostTracker struct {
    store *SQLiteStore
}

type TokenUsage struct {
    TaskID      string    `json:"task_id"`
    AgentID     string    `json:"agent_id"`
    Timestamp   time.Time `json:"timestamp"`
    Model       string    `json:"model"`
    TokensIn    int       `json:"tokens_in"`
    TokensOut   int       `json:"tokens_out"`
    CostCents   int       `json:"cost_cents"`
    RequestType string    `json:"request_type"` // "refine", "execute", "clarify"
}

// GetTaskCost returns total cost for a task
func (c *CostTracker) GetTaskCost(taskID string) (CostSummary, error) {
    // Sum all TokenUsage for task
}

// GetDailyCost returns today's spending
func (c *CostTracker) GetDailyCost() (CostSummary, error) {
    // Sum all TokenUsage for today
}

// CheckLimits returns warnings if approaching limits
func (c *CostTracker) CheckLimits() []CostWarning {
    // Compare against config limits
    // Fire on_cost_threshold hook if needed
}
```

**UI Display:**
```
Task TAND-42: $0.54 (12,450 tokens)
Today: $14.82 / $20.00 (74%)
```

### From Aider: Per-File Token Reporting

**Purpose:** Understand context window usage.

```go
type FileTokenReport struct {
    Files       []FileTokens
    TotalTokens int
}

type FileTokens struct {
    Path   string
    Tokens int
}

// GetContextReport shows what's consuming the context window
func (pm *PMAgent) GetContextReport() FileTokenReport {
    // Tokenize each file in context
    // Sort by token count
    // Return report
}
```

**TUI Command:** `/tokens` shows current context usage

### From Aider: Repository Context Mapping (RepoMap)

**Purpose:** Smart context selection using code graph.

```go
package repomap

import (
    "github.com/smacker/go-tree-sitter"
)

type RepoMap struct {
    graph     *SymbolGraph
    tokenizer Tokenizer
}

type SymbolGraph struct {
    nodes map[string]*Symbol  // file:symbol → Symbol
    edges []SymbolReference   // caller → callee
}

// BuildGraph parses all source files and extracts symbols
func (r *RepoMap) BuildGraph(dir string) error {
    // 1. Find all source files
    // 2. Parse with tree-sitter
    // 3. Extract definitions and references
    // 4. Build directed graph
}

// GetRelevantContext returns most important files for a query
func (r *RepoMap) GetRelevantContext(query string, maxTokens int) []ContextFile {
    // 1. Find files matching query
    // 2. Run PageRank on symbol graph
    // 3. Return top files up to token limit
}
```

**Benefits:**
- Automatically includes related files
- Prioritizes heavily-referenced code
- Respects token budget

### From Windsurf: Per-Task Memory

**Purpose:** Persist agent learnings across sessions.

```go
type TaskMemory struct {
    TaskID    string            `json:"task_id"`
    Memories  []Memory          `json:"memories"`
    CreatedAt time.Time         `json:"created_at"`
    UpdatedAt time.Time         `json:"updated_at"`
}

type Memory struct {
    ID        string    `json:"id"`
    Type      string    `json:"type"`      // "user", "auto"
    Content   string    `json:"content"`
    Source    string    `json:"source"`    // "pm_agent", "coding_agent", "human"
    CreatedAt time.Time `json:"created_at"`
}

// AddMemory stores a learning for future reference
func (s *Store) AddMemory(taskID string, mem Memory) error {
    // Append to task's memory log
}

// GetMemories retrieves memories for task context
func (s *Store) GetMemories(taskID string) ([]Memory, error) {
    // Return memories sorted by relevance
}
```

**Storage:** `.tandemonium/tasks/<id>/memory.json`

**Auto-generated memories:**
- "Prefers async/await over callbacks"
- "Uses zod for validation"
- "Test files in __tests__ directory"

### From LangGraph: Checkpoint/Resume

**Purpose:** Recover from crashes mid-task.

```go
type Checkpoint struct {
    ID          string          `json:"id"`
    TaskID      string          `json:"task_id"`
    AgentID     string          `json:"agent_id"`
    State       AgentState      `json:"state"`
    Messages    []Message       `json:"messages"`    // Conversation history
    ToolResults []ToolResult    `json:"tool_results"`
    CreatedAt   time.Time       `json:"created_at"`
}

type Checkpointer interface {
    Save(checkpoint Checkpoint) error
    Load(taskID string) (*Checkpoint, error)
    List(taskID string) ([]Checkpoint, error)
}

// SQLiteCheckpointer persists to database
type SQLiteCheckpointer struct {
    db *sql.DB
}

// Checkpoint at natural boundaries
func (a *Agent) MaybeCheckpoint() {
    // After tool use
    // After PM Agent question answered
    // Every N messages
    // Before expensive operations
}
```

**Recovery:**
```go
func (m *Manager) RecoverTask(taskID string) error {
    checkpoint, err := m.checkpointer.Load(taskID)
    if err != nil {
        return err // Start fresh
    }

    // Restore agent state
    // Resume from last checkpoint
}
```

### From OpenHands: Event Sourcing for Audit

**Purpose:** Complete audit trail of agent actions.

```go
type Event struct {
    ID        string          `json:"id"`
    TaskID    string          `json:"task_id"`
    AgentID   string          `json:"agent_id"`
    Type      EventType       `json:"type"`
    Timestamp time.Time       `json:"timestamp"`
    Payload   json.RawMessage `json:"payload"`
}

type EventType string

const (
    EventTaskCreated      EventType = "task.created"
    EventTaskAssigned     EventType = "task.assigned"
    EventToolCalled       EventType = "tool.called"
    EventToolResult       EventType = "tool.result"
    EventMessageSent      EventType = "message.sent"
    EventQuestionAsked    EventType = "question.asked"
    EventQuestionAnswered EventType = "question.answered"
    EventSpecGenerated    EventType = "spec.generated"
    EventCodeWritten      EventType = "code.written"
    EventTestRun          EventType = "test.run"
    EventTaskCompleted    EventType = "task.completed"
)

// Append-only event log
type EventStore interface {
    Append(event Event) error
    GetEvents(taskID string) ([]Event, error)
    Replay(taskID string) (*TaskState, error) // Reconstruct state from events
}
```

**Benefits:**
- Complete audit trail
- Deterministic replay
- Debug agent behavior
- Learning from past executions

### From Cursor 2.0: Agent Roles

**Purpose:** Specialized agents for different tasks.

```go
type AgentRole string

const (
    RoleGeneral   AgentRole = "general"   // Default coding agent
    RoleFrontend  AgentRole = "frontend"  // React, CSS, HTML specialist
    RoleBackend   AgentRole = "backend"   // API, database specialist
    RoleTesting   AgentRole = "testing"   // Test writing specialist
    RoleRefactor  AgentRole = "refactor"  // Code cleanup specialist
    RoleSecurity  AgentRole = "security"  // Security review specialist
)

type AgentProfile struct {
    Role         AgentRole `toml:"role"`
    Model        string    `toml:"model"`
    SystemPrompt string    `toml:"system_prompt"`
    Tools        []string  `toml:"tools"` // Allowed tools
    FilePatterns []string  `toml:"file_patterns"` // e.g., ["*.tsx", "*.css"]
}
```

**Configuration:**
```toml
# .tandemonium/agents.toml

[agents.frontend]
role = "frontend"
model = "claude-sonnet-4-20250514"
system_prompt = "You are a frontend specialist..."
file_patterns = ["*.tsx", "*.css", "*.html"]

[agents.testing]
role = "testing"
model = "claude-haiku-3-5-20241022"
system_prompt = "You write comprehensive tests..."
tools = ["run_tests", "read_file", "write_file"]
```

---

## Updated File Structure

```
tandemonium/
├── go.mod
├── go.sum
├── main.go
├── tandemonium.toml.example
│
├── cmd/
│   └── tandemonium/
│       └── main.go              # Entry point
│
├── internal/
│   ├── app/
│   │   ├── app.go               # Main Bubble Tea model
│   │   ├── messages.go          # Custom messages
│   │   └── keys.go              # Key bindings
│   │
│   ├── models/
│   │   ├── task.go              # Task, RefinedSpec
│   │   ├── agent.go             # Agent, AgentStatus, AgentRole
│   │   ├── event.go             # Event sourcing types
│   │   └── log.go               # LogEntry, BlockedState
│   │
│   ├── views/
│   │   ├── fleet.go             # Fleet View
│   │   ├── focus.go             # Focus View
│   │   ├── refine.go            # Refine View (PM agent)
│   │   ├── spec_review.go       # Spec Review
│   │   ├── code_review.go       # Code Review with diff
│   │   ├── diff.go              # Diff viewer component
│   │   ├── queue.go             # Task Queue
│   │   └── modals.go            # New task, unblock
│   │
│   ├── agents/
│   │   ├── manager.go           # Agent lifecycle
│   │   ├── pm.go                # PM Agent (Claude API)
│   │   ├── tmux.go              # tmux wrapper
│   │   ├── detector.go          # Prompt detection
│   │   └── roles.go             # Agent role profiles
│   │
│   ├── services/
│   │   ├── store.go             # SQLite persistence
│   │   ├── git.go               # Git/worktree ops
│   │   ├── costs.go             # Cost tracking
│   │   ├── events.go            # Event store
│   │   ├── checkpointer.go      # Checkpoint/resume
│   │   ├── memory.go            # Task memory
│   │   └── hooks.go             # Hook system
│   │
│   ├── daemon/
│   │   ├── daemon.go            # Background execution
│   │   ├── client.go            # TUI-daemon communication
│   │   └── server.go            # Daemon server
│   │
│   ├── mcp/
│   │   ├── server.go            # MCP server interface
│   │   ├── builtin.go           # Built-in tools
│   │   └── external.go          # External MCP servers
│   │
│   ├── repomap/
│   │   ├── graph.go             # Symbol graph
│   │   ├── parser.go            # tree-sitter parsing
│   │   └── pagerank.go          # Context ranking
│   │
│   └── config/
│       └── config.go            # TOML config with hierarchy
│
└── tests/
    └── ...
```

---

## Updated Implementation Phases

### Phase 1: Foundation (Week 1-2)
- [x] Go module setup with Bubble Tea
- [ ] Basic TUI with Fleet View skeleton
- [ ] TmuxSession wrapper for coding agents
- [ ] Single coding agent spawn and stream capture
- [ ] Task model with SQLite persistence
- [ ] Simple task creation (no refinement yet)
- [ ] Focus View with log display
- [ ] **NEW:** Config file loading (hierarchical)
- [ ] **NEW:** Session persistence for recovery

### Phase 2: PM Agent (Week 3-4)
- [ ] PM Agent with Claude API integration
- [ ] Codebase search tools (ripgrep wrapper)
- [ ] Clarification dialog UI (Refine View)
- [ ] RefinedSpec model
- [ ] Spec Review View
- [ ] Refinement → Coding handoff
- [ ] **NEW:** Cost tracking integration
- [ ] **NEW:** Event sourcing for audit trail

### Phase 3: Multi-Agent (Week 5-6)
- [ ] Support 2-4 concurrent coding agents
- [ ] Prompt detection (trust dialogs, blockers)
- [ ] Auto-accept for trust prompts
- [ ] Unblock Modal
- [ ] Git worktree integration
- [ ] Cost tracking and limits
- [ ] **NEW:** Agent roles and profiles
- [ ] **NEW:** Hook system (basic lifecycle hooks)

### Phase 4: Review Flow (Week 7-8)
- [ ] Code Review View with diff display
- [ ] **NEW:** Diff viewer with syntax highlighting
- [ ] Approval/rejection workflows
- [ ] Rejection feedback loop to agent
- [ ] Task Queue View with filters
- [ ] **NEW:** Push automation (commit + push + PR)
- [ ] **NEW:** Checkpoint/resume system

### Phase 5: Advanced Features (Week 9-10)
- [ ] **NEW:** Daemon mode for background execution
- [ ] **NEW:** MCP integration for extensible tools
- [ ] **NEW:** RepoMap context optimization
- [ ] **NEW:** Per-task memory persistence
- [ ] Desktop notifications
- [ ] Documentation

### Phase 6: Polish (Week 11-12)
- [ ] Keyboard shortcut refinement
- [ ] Error handling and edge cases
- [ ] Performance optimization
- [ ] **NEW:** Hook system (full event coverage)
- [ ] Comprehensive testing
