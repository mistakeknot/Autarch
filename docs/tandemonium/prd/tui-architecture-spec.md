# Tandemonium TUI Architecture Specification

**Version:** 2.1
**Date:** 2025-01-09
**Status:** Approved (Final revision after Oracle v2 verification)
**Based on:** Oracle (GPT-5.2 Pro) Analysis + Stakeholder Interview + Oracle Follow-up Reviews (v1, v2)

---

## Executive Summary

This specification addresses the top 5 recommendations from Oracle's architectural review of Tandemonium. It defines the target architecture for the TUI-first approach, replacing the previous hybrid GUI/TUI strategy.

**Revision 2.1 Changes:** Addressed Oracle's v2 verification findings:
- Fixed MCP permission model: agent identity/registration, schema for allow/deny/prompt
- Standardized capability naming (canonical list)
- Safer permission defaults (deny until registered)
- Added scope precedence rules for permission resolution
- Fixed YAML export semantics (no inline subtasks)
- Added SQLite operational settings (foreign_keys, busy_timeout)
- Added CHECK constraints for enum validation
- Added Threat Model section
- Clarified audit DB vs tasks DB separation
- Fixed lease acquisition with lease_id for atomicity
- Clarified CLI is maintenance-only
- Added external terminal configuration
- Fixed command registry for async + input modes

**Revision 2.0 Changes:** Addressed Oracle's follow-up review findings:
- Added explicit MCP permission model (was: audit-only)
- Expanded SQLite schema to match current Task struct (scope, progress, tests, subtasks)
- De-risked terminal to command-runner first, PTY phased
- Added command registry for drift prevention
- Added PRD reconciliation section

### Key Decisions

| Area | Decision |
|------|----------|
| **Primary Interface** | TUI (GUI explicitly deferred); CLI is maintenance-only |
| **Storage** | SQLite (WAL mode) as source of truth + debounced YAML export |
| **YAML Role** | Read-only audit/debug export, one file per task (no inline subtasks) |
| **Terminal** | Phase 1: Command runner. Phase 2: Full PTY |
| **Locking** | Task-level only (path-level deferred) |
| **Agent Liveness** | File-based lease with lease_id + process monitoring fallback |
| **MCP Security** | Capability-based permissions with agent registration + audit logging |
| **Permission Defaults** | Deny until agent is registered; explicit allow/deny/prompt per capability |

---

## 0. PRD Reconciliation

**This section explicitly documents where this spec diverges from `tandemonium-mvp-prd.md`.**

| Topic | PRD Says | This Spec Says | Resolution |
|-------|----------|----------------|------------|
| Primary Interface | Tauri GUI (native macOS) | TUI | **Spec wins.** PRD vision deferred to P1+. |
| Storage | Versioned atomic YAML (`tasks.yml`) | SQLite + per-task YAML export | **Spec wins.** YAML insufficient for multi-agent. |
| Terminal | Command-runner for MVP, PTY high-risk | Phase 1: Command-runner, Phase 2: PTY | **Aligned.** Spec adopts PRD's de-risking. |
| Path Locking | Core feature for parallelism | Deferred (task-level only) | **Spec wins.** Simplicity for P0. |
| MCP Security | Network allowlist + logging | Capability permissions + audit | **Spec wins.** More comprehensive. |
| Status Naming | `in_progress` (underscore) | `inprogress` (no underscore) | **Spec wins.** Matches Rust code. |

**Action Required:** Update PRD with addendum referencing this spec, or mark PRD sections as superseded.

---

## 0.1 Threat Model

**CRITICAL:** This section defines what we protect against and what we explicitly do NOT protect against.

### In Scope (Protected)

| Threat | Mitigation |
|--------|------------|
| **Accidental destructive agent actions** | Capability checks + user prompts for destructive ops |
| **Untrusted agent gaining privileges** | Agent registration required; deny by default until registered |
| **Agent impersonation** | Lease files with random `lease_id` + PID verification |
| **Concurrent task corruption** | SQLite transactions + task-level locking |
| **Audit log tampering by agents** | Audit DB is append-only; agents cannot delete entries |
| **Permission escalation** | Capabilities stored per-agent; no wildcard inheritance |

### Out of Scope (NOT Protected)

| Threat | Why Not Protected | User Responsibility |
|--------|-------------------|---------------------|
| **Malicious local process with filesystem access** | Any process with `.tandemonium/` write access can edit SQLite directly, bypassing all capability checks | Run untrusted code in sandboxed environments |
| **Host compromise / root access** | Beyond application scope | OS-level security |
| **SQLite file corruption** | Hardware/OS issue | Backups, WAL checkpointing |
| **Denial of service via rapid operations** | Performance issue, not security | Rate limiting is a future enhancement |

### Trust Boundaries

```
┌─────────────────────────────────────────────────────────────────┐
│ TRUSTED: TUI Process                                            │
│  - Full read/write to tasks.db, audit.db                        │
│  - Can grant/deny capabilities                                  │
│  - Can force-release locks                                      │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ MCP Protocol (capability-gated)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ UNTRUSTED UNTIL REGISTERED: Agent Processes                     │
│  - Must register before any operations                          │
│  - All operations go through capability checks                  │
│  - Cannot access DBs directly (enforced by protocol, not FS)    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              │ Filesystem (no protection)
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ OUT OF SCOPE: Direct Filesystem Access                          │
│  - Any process can read/write .tandemonium/                     │
│  - Capability model only governs MCP tool calls                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 1. Storage Architecture

### 1.1 Source of Truth: SQLite

**Rationale:** Transactional correctness, concurrent access safety, and query capabilities that YAML cannot provide.

```
.tandemonium/
├── tasks.db           # SQLite database (WAL mode) - tasks, deps, leases, capabilities
├── audit.db           # SEPARATE SQLite database - audit_log table ONLY (append-only, no pruning)
├── tasks/             # YAML export directory (read-only)
│   ├── tsk_01J6QX3N2Z8.yml
│   ├── tsk_01J6QX4M1A7.yml
│   └── ...
├── leases/            # Agent lease files
└── config.yml         # Project configuration
```

**Database Separation:** `tasks.db` and `audit.db` are SEPARATE files. This allows:
- Independent backup/archival of audit logs
- No transaction coordination needed (audit is append-only)
- Audit DB can grow unbounded without affecting task DB performance

### 1.1.1 SQLite Operational Settings

**CRITICAL:** These PRAGMAs must be set on every connection.

```sql
-- Required for multi-agent safety
PRAGMA journal_mode = WAL;           -- Write-Ahead Logging for concurrent reads
PRAGMA foreign_keys = ON;            -- Enforce referential integrity
PRAGMA busy_timeout = 5000;          -- 5 second timeout on lock contention
PRAGMA synchronous = NORMAL;         -- Balance durability and performance

-- Connection pool guidance
-- Use a single writer connection (serialize writes)
-- Allow multiple reader connections
-- On SQLITE_BUSY: retry with exponential backoff (100ms, 200ms, 400ms, max 3 retries)
```

**Soft Ceiling:** Design assumes ≤10 concurrent agents. Beyond this, consider batched writes or a dedicated writer process.

### 1.2 SQLite Schema

**IMPORTANT:** This schema matches the current `Task` struct in `tandemonium-core/src/task.rs`.

```sql
-- Core task table (matches Task struct fully)
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,                    -- ULID (e.g., "tsk_01J6QX3N2Z8")
    slug TEXT UNIQUE NOT NULL,              -- Human-readable identifier
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'todo'
        CHECK(status IN ('draft', 'todo', 'inprogress', 'review', 'done', 'blocked')),
    assigned_to TEXT,                       -- Agent name or NULL
    branch TEXT,                            -- Git branch name
    worktree TEXT,                          -- Worktree path
    base_sha TEXT,                          -- Git SHA at task start
    pr_url TEXT,
    parent_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,  -- For subtask hierarchy
    progress_mode TEXT NOT NULL DEFAULT 'automatic'
        CHECK(progress_mode IN ('automatic', 'subtasks')),
    progress_value INTEGER NOT NULL DEFAULT 0
        CHECK(progress_value BETWEEN 0 AND 100),
    created_at TEXT NOT NULL,               -- ISO 8601 UTC (e.g., "2025-01-09T10:00:00Z")
    updated_at TEXT NOT NULL                -- ISO 8601 UTC
);

-- Separate dependencies table (junction)
CREATE TABLE task_dependencies (
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    PRIMARY KEY (task_id, depends_on)
);

-- Acceptance criteria
CREATE TABLE acceptance_criteria (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    completed INTEGER NOT NULL DEFAULT 0,
    position INTEGER NOT NULL DEFAULT 0
);

-- File scope (when path-level features are enabled)
CREATE TABLE task_file_scope (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    files_glob TEXT NOT NULL,              -- JSON array of glob patterns
    files_resolved TEXT,                   -- JSON array of ResolvedFile objects
    locked_paths TEXT,                     -- JSON array of locked path strings
    shared_with TEXT                       -- JSON array of task IDs
);

-- Test files associated with tasks
CREATE TABLE task_tests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    test_path TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

-- Task Master metadata (for imported tasks)
CREATE TABLE taskmaster_metadata (
    task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    original_id TEXT NOT NULL
);

-- Agent registration (required before any operations)
CREATE TABLE registered_agents (
    agent_name TEXT PRIMARY KEY,
    executable_path TEXT NOT NULL,         -- Full path to agent executable
    first_seen_at TEXT NOT NULL,           -- ISO 8601 UTC
    last_seen_at TEXT NOT NULL,            -- ISO 8601 UTC
    trust_level TEXT NOT NULL DEFAULT 'untrusted'
        CHECK(trust_level IN ('untrusted', 'registered', 'trusted')),
    registered_by TEXT NOT NULL,           -- "auto_discovery" | "user" | "config"
    notes TEXT                             -- Optional user notes
);

-- Agent leases (task-level locking) - with lease_id for atomicity
CREATE TABLE agent_leases (
    task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    agent_name TEXT NOT NULL REFERENCES registered_agents(agent_name),
    agent_pid INTEGER NOT NULL,            -- For process monitoring
    lease_id TEXT NOT NULL,                -- Random UUID to prevent PID reuse confusion
    lease_file TEXT NOT NULL,              -- Path to lease file
    acquired_at TEXT NOT NULL,             -- ISO 8601 UTC
    last_heartbeat TEXT NOT NULL           -- ISO 8601 UTC
);

-- Agent capabilities (MCP permission model) - with allow/deny/prompt
CREATE TABLE agent_capabilities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_name TEXT NOT NULL REFERENCES registered_agents(agent_name),
    capability TEXT NOT NULL
        CHECK(capability IN (
            'task.read', 'task.write', 'task.claim',
            'git.worktree.create', 'git.worktree.delete', 'git.pr.create',
            'lock.acquire', 'lock.release', 'lock.force_release',
            'shell.exec'
        )),
    scope TEXT,                            -- Optional: task ID, "*" for all, NULL for global
    effect TEXT NOT NULL DEFAULT 'prompt'
        CHECK(effect IN ('allow', 'deny', 'prompt')),
    granted_at TEXT NOT NULL,              -- ISO 8601 UTC
    granted_by TEXT NOT NULL,              -- "user" | "config" | "prompt_response"
    expires_at TEXT,                       -- Optional: ISO 8601 UTC for time-limited grants
    reason TEXT,                           -- Optional: why this was granted/denied
    UNIQUE(agent_name, capability, scope)  -- One rule per agent+capability+scope
);

-- ============================================================
-- AUDIT.DB (separate database file)
-- ============================================================

-- Audit log (no auto-pruning, kept forever, append-only)
CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL,               -- ISO 8601 UTC
    event_type TEXT NOT NULL               -- task_created|task_claimed|mcp_tool_called|permission_denied|...
        CHECK(event_type IN (
            'agent_registered', 'agent_trust_changed',
            'task_created', 'task_updated', 'task_deleted',
            'task_claimed', 'task_released',
            'mcp_tool_called', 'permission_checked',
            'permission_granted', 'permission_denied', 'permission_prompted',
            'lock_acquired', 'lock_released', 'lock_force_released',
            'lease_expired', 'agent_crashed'
        )),
    task_id TEXT,
    agent_name TEXT,
    details TEXT,                          -- JSON payload
    correlation_id TEXT,                   -- For tracing related events
    permission_checked TEXT,               -- Which capability was checked (if applicable)
    permission_result TEXT                 -- allow|deny|prompt
        CHECK(permission_result IS NULL OR permission_result IN ('allow', 'deny', 'prompt'))
);

-- Indexes
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_assigned ON tasks(assigned_to);
CREATE INDEX idx_tasks_parent ON tasks(parent_id);
CREATE INDEX idx_deps_depends_on ON task_dependencies(depends_on);
CREATE INDEX idx_audit_task ON audit_log(task_id);
CREATE INDEX idx_audit_time ON audit_log(timestamp);
CREATE INDEX idx_audit_agent ON audit_log(agent_name);
CREATE INDEX idx_caps_agent ON agent_capabilities(agent_name);
```

### 1.3 YAML Export

**Purpose:** Read-only audit trail and debugging aid. Humans do not edit YAML directly.

**CRITICAL:** The YAML directory MUST NOT be read by runtime code. Only SQLite is authoritative.

**Trigger:** Debounced export (500ms) on SQLite write. Only changed tasks are re-exported.

**Atomic Write:** Write to temp file, fsync, rename to final path.

**Format:** One file per task. **NO INLINE SUBTASKS** - subtasks are separate files with `parent_id` reference.

```yaml
# .tandemonium/tasks/tsk_01J6QX3N2Z8.yml
id: tsk_01J6QX3N2Z8
slug: login-form
title: Implement login form component
description: Create React component for user login
status: inprogress
assigned_to: claude-code-agent-1
branch: feature/tsk-01J6-login-form
worktree: .tandemonium/worktrees/tsk_01J6QX3N2Z8
base_sha: abc123def456
pr_url: null
parent_id: null                              # NULL for top-level tasks
subtask_ids:                                 # Reference only, NOT inline content
  - tsk_01J6QX3N2Z9
  - tsk_01J6QX3N2ZA
progress:
  mode: automatic
  value: 50
depends_on:
  - tsk_01J6QX2K9P3
acceptance_criteria:
  - text: Form validates email format
    completed: true
  - text: Shows error for invalid credentials
    completed: false
scope:
  files_glob:
    - "src/components/LoginForm.tsx"
  files_resolved:
    - path: "src/components/LoginForm.tsx"
      resolved_at: "2025-01-09T10:05:00Z"
      git_sha: "abc123..."
  locked_paths:
    - "src/components/LoginForm.tsx"
  shared_with: []
tests:
  - "src/components/LoginForm.test.tsx"
created_at: "2025-01-09T10:00:00Z"
updated_at: "2025-01-09T14:30:00Z"
exported_at: "2025-01-09T14:30:01Z"
```

**Subtask Example:**
```yaml
# .tandemonium/tasks/tsk_01J6QX3N2Z9.yml (subtask file)
id: tsk_01J6QX3N2Z9
slug: login-form-validation
title: Add email validation
parent_id: tsk_01J6QX3N2Z8              # Points to parent
subtask_ids: []                          # Can have nested subtasks
# ... rest of fields
```

### 1.4 Migration Strategy

**Approach:** Clean break. No backwards compatibility with existing `tasks.yml`.

**Migration Path:**
1. Detect existing `.tandemonium/tasks.yml`
2. Import all tasks into SQLite (including subtasks, scope, progress, tests)
3. Export to new per-task YAML format
4. Rename old file to `tasks.yml.backup`
5. Log migration in audit log

---

## 2. Interface Architecture

### 2.1 TUI as Primary Interface

**Default Behavior:** `tandemonium` with no args launches TUI (current behavior preserved).

**GUI Status:** Explicitly deferred. No architectural preparation for GUI in this phase.

### 2.2 Terminal Integration (Phased)

**Phase 1 (P0): Command Runner**
- Non-interactive command execution
- `tokio::process::Command` with process group isolation
- SIGINT → SIGTERM → SIGKILL cascade (3s/10s timeouts)
- Output streamed to terminal pane
- "Open external terminal" fallback for interactive needs

**Phase 2 (P1): Full PTY**
- Interactive shell with `wezterm-term` + `portable-pty`
- Viewing vs Interactive mode (`Ctrl+O` toggle)
- Scoped capability: "good enough for agent output + basic shell"
- Explicitly NOT a full terminal (no vim, no tmux, no full-screen apps)
- **Technical constraint:** Detect and block alternate screen buffer mode; kill known offenders with clear messaging

**Rationale:** PRD correctly identified PTY as high-risk. De-risking to command-runner for P0 reduces timeline risk while providing 90% of value.

**Supported Platforms (P0):** macOS, Linux. Windows support deferred (requires different signal/process handling).

### 2.2.1 External Terminal Configuration

**Purpose:** Allow users to open tasks in their preferred terminal for interactive workflows.

```yaml
# .tandemonium/config.yml
terminal:
  # External terminal command template
  # {worktree} = task worktree path
  # {task_id} = task ID
  # {task_slug} = task slug
  external_command:
    macos: ["open", "-a", "Terminal", "{worktree}"]
    linux: ["x-terminal-emulator", "-e", "cd {worktree} && $SHELL"]
    # Or user override:
    # custom: ["wezterm", "start", "--cwd", "{worktree}"]
```

**Default Behavior:**
- macOS: Open in Terminal.app
- Linux: Use `x-terminal-emulator` (follows system default)
- User can override with `terminal.external_command.custom`

### 2.3 Progress Visibility

Progress updates visible in multiple locations:

1. **Terminal Output Pane:** Raw stdout/stderr from agent
2. **Structured Progress:** Parsed progress indicators (if agent provides them)
3. **File Change Indicators:** Show files modified in worktree
4. **Status Bar:** Current task status + any anomalies
5. **Task Detail Refresh:** Acceptance criteria completion updates

### 2.4 Event Architecture

**Approach:** Hybrid polling + optional push.

**YAML Export Debouncing:** To prevent event storms:
- Export triggered on DB write
- Debounced to 500ms (coalesce rapid writes)
- Only re-export changed task files
- TUI watches SQLite directly, NOT YAML directory

```rust
tokio::select! {
    // Keyboard input (crossterm)
    key_event = crossterm_events.next() => { /* handle */ }

    // SQLite change detection (500ms interval)
    _ = db_poll_interval.tick() => { /* check for agent updates */ }

    // Command output (for command-runner mode)
    Some(output) = command_stdout.recv() => { /* update terminal pane */ }

    // Optional: Agent push channel (if MCP provides it)
    Some(progress) = agent_progress_rx.recv() => { /* immediate update */ }

    // Periodic UI tick (60fps)
    _ = tick_interval.tick() => { /* redraw */ }
}
```

---

## 3. Command Palette & Registry

### 3.1 Dual Palette Design

| Shortcut | Purpose | Scope |
|----------|---------|-------|
| `Ctrl+K` | Command palette | All commands (~30) |
| `/` | Task search | Filter task list only |

### 3.2 Command Registry (Drift Prevention)

**CRITICAL:** All commands MUST be defined in a single registry. No hardcoded keybindings or palette entries elsewhere.

```rust
pub struct Command {
    pub id: &'static str,           // Unique identifier
    pub title: &'static str,        // Display name
    pub description: &'static str,  // Help text
    pub category: CommandCategory,  // Task|Navigation|Git|View|System
    pub shortcut: Option<KeyBinding>,
    pub input_mode: InputMode,      // When this shortcut is active
    pub enabled: fn(&AppState) -> bool,
    pub execute: CommandAction,     // Supports both sync and async
}

pub enum CommandCategory {
    Task,
    Navigation,
    Git,
    View,
    System,
}

/// Defines when a keyboard shortcut is active
pub enum InputMode {
    /// Active only when no text input is focused (default for most shortcuts)
    Global,
    /// Active only in specific focus contexts
    Context(FocusContext),
    /// Always active, even during text input (e.g., Escape, Ctrl+C)
    Always,
}

pub enum FocusContext {
    TaskList,
    TaskDetail,
    Terminal,
    Search,
    CommandPalette,
}

/// Command execution - supports async via effect queue
pub enum CommandAction {
    /// Synchronous, immediate effect (e.g., navigation)
    Sync(fn(&mut AppState) -> Result<()>),
    /// Async operation - returns intent, executed by event loop
    Async(fn(&AppState) -> CommandIntent),
}

pub enum CommandIntent {
    /// Spawn a process and stream output
    SpawnCommand { cmd: String, args: Vec<String>, cwd: PathBuf },
    /// Database operation (runs on background thread)
    DbOperation(Box<dyn FnOnce(&Connection) -> Result<()> + Send>),
    /// MCP tool call
    McpCall { tool: String, params: serde_json::Value },
    /// No-op (command handled synchronously)
    None,
}

// All UI surfaces derive from this registry:
// - Ctrl+K palette list
// - Bottom keybinding hints
// - ? help screen
// - Direct shortcut handlers
```

### 3.2.1 Input Mode Handling

**Problem:** Shortcuts like `q` (quit) and `n` (new task) conflict with text input.

**Solution:** Commands declare their `InputMode`:
- `Global`: Only active when NOT in a text input (search box, edit field)
- `Always`: Active even during text input (Escape, Ctrl+C, Ctrl+K)
- `Context(...)`: Only active in specific focus areas

**Enforcement:** A test MUST verify:
1. No duplicate shortcut + input_mode combinations
2. All keyboard handling routes through the registry
3. Text input fields suppress Global shortcuts

### 3.3 Fuzzy Matching Algorithm

**Style:** VS Code-style with frecency boost.

**Ranking Factors:**
1. Word boundary matches prioritized (`ct` matches "**C**reate **T**ask" over "cal**c**ula**t**or")
2. Consecutive matches score higher
3. Recently used commands boosted
4. Frequently used commands boosted

**Implementation:** Use `nucleo` or `fuzzy-matcher` crate with custom scoring.

### 3.4 Command List (Target: <30)

```rust
// All commands registered here - single source of truth
pub static COMMANDS: &[Command] = &[
    // Task Operations
    Command { id: "task.create", title: "Create Task", shortcut: Some(Key::Char('n')), .. },
    Command { id: "task.start", title: "Start Task", shortcut: Some(Key::Char('s')), .. },
    Command { id: "task.complete", title: "Complete Task", shortcut: Some(Key::Char('c')), .. },
    Command { id: "task.delete", title: "Delete Task", shortcut: Some(Key::Char('d')), .. },
    Command { id: "task.edit", title: "Edit Task", .. },

    // Navigation
    Command { id: "nav.task_list", title: "Go to Task List", .. },
    Command { id: "nav.terminal", title: "Go to Terminal", .. },
    Command { id: "nav.task_detail", title: "Go to Task Detail", .. },
    Command { id: "nav.next_pane", title: "Focus Next Pane", shortcut: Some(Key::Tab), .. },
    Command { id: "nav.prev_pane", title: "Focus Previous Pane", .. },

    // Git Operations
    Command { id: "git.diff", title: "View Diff", .. },
    Command { id: "git.create_pr", title: "Create PR", .. },
    Command { id: "git.sync", title: "Sync Branch", .. },

    // Lock Management
    Command { id: "lock.force_release", title: "Force Release Lock", .. },
    Command { id: "lock.show_status", title: "Show Lock Status", .. },

    // View
    Command { id: "view.full_terminal", title: "Toggle Full Terminal", .. },
    Command { id: "view.audit_log", title: "Show Audit Log", .. },
    Command { id: "view.agent_status", title: "Show Agent Status", .. },

    // System
    Command { id: "system.settings", title: "Settings", shortcut: Some(Key::Char(',')), .. },
    Command { id: "system.help", title: "Help", shortcut: Some(Key::Char('?')), .. },
    Command { id: "system.quit", title: "Quit", shortcut: Some(Key::Char('q')), .. },
];
```

---

## 4. Multi-Agent Coordination

### 4.1 Agent Topology

**Assumption:** Agents run on same machine with shared filesystem access.

### 4.2 Task-Level Locking

**Scope:** One agent per task. No path-level locking in P0.

**Mechanism:** SQLite row + filesystem lease file.

```rust
pub struct AgentLease {
    task_id: String,
    agent_name: String,
    agent_pid: Option<u32>,
    lease_file: PathBuf,
    acquired_at: DateTime<Utc>,
    last_heartbeat: DateTime<Utc>,
}
```

### 4.3 Liveness Detection

**Primary:** File-based lease.

```
.tandemonium/leases/
├── tsk_01J6QX3N2Z8.lease.json
└── tsk_01J6QX4M1A7.lease.json
```

**Lease File Format:**
```json
{
    "task_id": "tsk_01J6QX3N2Z8",
    "agent_name": "claude-code-agent-1",
    "agent_pid": 12345,
    "lease_id": "550e8400-e29b-41d4-a716-446655440000",
    "acquired_at": "2025-01-09T10:00:00Z",
    "last_heartbeat": "2025-01-09T14:30:00Z"
}
```

**Lease Acquisition (Atomic):**
1. Generate random `lease_id` (UUID v4)
2. Attempt SQLite INSERT with `lease_id`
3. If INSERT succeeds, write lease file with same `lease_id`
4. If INSERT fails (conflict), another agent holds the lease
5. On heartbeat, verify `lease_id` matches DB before updating

**Why lease_id:** Prevents PID reuse confusion. If process 12345 dies and a new process gets the same PID, the `lease_id` won't match, so the stale lease is detected.

**Agent Responsibility:** Touch lease file periodically (every 30 seconds). Verify `lease_id` matches on each heartbeat.

**Fallback:** Process monitoring via PID. If lease file is stale AND process is dead AND `lease_id` matches, consider agent crashed.

### 4.4 Lock Release Policy

**No auto-release.** Manual intervention required.

**Stale Lock Handling:**
1. TUI shows warning: "Agent 'X' on task 'Y' appears unresponsive"
2. User must explicitly release via `Force Release Lock` command (in registry)
3. Action logged in audit log with user confirmation

---

## 5. MCP Security & Permissions

### 5.1 Agent Discovery & Registration

**CRITICAL CHANGE from v1.0 and v2.0:** Agents must be REGISTERED before any operations.

#### Discovery Process
1. **Auto-detection:** Process scanning identifies potential agents (Claude Code, Cursor, etc.)
2. **Registration required:** Discovered agents start as `untrusted`
3. **First operation triggers:** TUI prompts user to register or reject the agent
4. **Registration stored:** In `registered_agents` table with trust level

#### Trust Levels

| Level | Meaning | Default Permissions |
|-------|---------|---------------------|
| `untrusted` | Just discovered, not yet approved | ALL operations → Deny |
| `registered` | User acknowledged, basic access | Config defaults apply |
| `trusted` | Elevated trust, fewer prompts | More permissive defaults |

```rust
pub struct AgentIdentity {
    pub name: String,              // e.g., "claude-code-agent-1"
    pub executable_path: PathBuf,  // Full path to binary
    pub pid: u32,                  // Current process ID
    pub trust_level: TrustLevel,
}

pub enum TrustLevel {
    Untrusted,   // Deny all, prompt to register
    Registered,  // Basic access, config defaults
    Trusted,     // Elevated permissions
}
```

### 5.2 Capability-Based Permission Model

**CRITICAL:** All capabilities are DENIED by default until agent is registered.

#### Canonical Capability List

**All capabilities use this exact naming. No aliases.**

| Capability | Description | Registered Default | Trusted Default | Destructive? |
|------------|-------------|-------------------|-----------------|--------------|
| `task.read` | List and view tasks | allow | allow | No |
| `task.write` | Create, update tasks | prompt | allow | Yes |
| `task.claim` | Claim/unclaim tasks | allow | allow | No |
| `git.worktree.create` | Create worktrees | prompt | allow | Yes |
| `git.worktree.delete` | Delete worktrees | prompt | allow | Yes |
| `git.pr.create` | Create pull requests | prompt | prompt | Yes |
| `lock.acquire` | Acquire task lock | allow | allow | No |
| `lock.release` | Release own lock | allow | allow | No |
| `lock.force_release` | Force release another's lock | deny | prompt | Yes |
| `shell.exec` | Execute shell commands | prompt | prompt | Yes |

#### Permission Resolution (with Scope Precedence)

```rust
pub enum PermissionEffect {
    Allow,
    Deny,
    Prompt(String),  // Prompt message
}

pub fn check_permission(
    agent: &str,
    capability: &str,
    scope: Option<&str>,  // e.g., task_id, "*", or None
) -> PermissionEffect {
    // 1. Check agent registration
    let agent_info = get_registered_agent(agent);
    if agent_info.is_none() || agent_info.trust_level == Untrusted {
        return PermissionEffect::Deny;  // Unregistered = deny all
    }

    // 2. Check explicit rules with scope precedence
    //    Priority: task-specific > global (*) > no scope (NULL)
    //    Within same specificity: deny > prompt > allow
    let rules = get_capability_rules(agent, capability);

    // Most specific scope first
    if let Some(task_scope) = scope {
        if let Some(rule) = rules.find(|r| r.scope == Some(task_scope)) {
            return rule.effect;
        }
    }
    // Then wildcard
    if let Some(rule) = rules.find(|r| r.scope == Some("*")) {
        return rule.effect;
    }
    // Then global (no scope)
    if let Some(rule) = rules.find(|r| r.scope.is_none()) {
        return rule.effect;
    }

    // 3. Fall back to trust-level default
    get_default_for_trust_level(agent_info.trust_level, capability)
}
```

#### Scope Precedence Rules

| Priority | Scope | Example | Meaning |
|----------|-------|---------|---------|
| 1 (highest) | Specific task ID | `tsk_01J6QX3N2Z8` | Rule applies only to this task |
| 2 | Wildcard | `*` | Rule applies to all tasks |
| 3 (lowest) | NULL/global | (none) | Default rule for this capability |

**Conflict Resolution:** When multiple rules match at the same specificity level:
- `deny` wins over `prompt`
- `prompt` wins over `allow`

#### User Prompts (TUI)

When an agent requests a `Prompt`-level capability:

```
┌─────────────────────────────────────────────────────────────┐
│ Permission Request                                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ Agent "claude-code-agent-1" (registered) wants to:          │
│   Create a pull request                                     │
│                                                             │
│ Capability: git.pr.create                                   │
│ Task: login-form (tsk_01J6QX3N2Z8)                          │
│ Branch: feature/tsk-01J6-login-form                         │
│                                                             │
│ [A] Allow once   [T] Allow for this task                    │
│ [S] Always allow [D] Deny once   [N] Always deny            │
└─────────────────────────────────────────────────────────────┘
```

#### Agent Registration Prompt

When an unregistered agent first attempts any operation:

```
┌─────────────────────────────────────────────────────────────┐
│ New Agent Detected                                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ An unregistered agent is attempting to access Tandemonium:  │
│                                                             │
│ Name: claude-code-agent-1                                   │
│ Path: /usr/local/bin/claude-code                            │
│ PID: 12345                                                  │
│                                                             │
│ [R] Register (basic access)                                 │
│ [T] Register as Trusted (elevated access)                   │
│ [D] Deny (block this agent)                                 │
│ [?] More info                                               │
└─────────────────────────────────────────────────────────────┘
```

#### Configuration File

```yaml
# .tandemonium/config.yml
agents:
  # Defaults for REGISTERED agents (untrusted agents get deny for everything)
  registered_defaults:
    task.read: allow
    task.write: prompt
    task.claim: allow
    git.worktree.create: prompt
    git.worktree.delete: prompt
    git.pr.create: prompt
    lock.acquire: allow
    lock.release: allow
    lock.force_release: deny
    shell.exec: prompt

  # Defaults for TRUSTED agents
  trusted_defaults:
    task.read: allow
    task.write: allow
    task.claim: allow
    git.worktree.create: allow
    git.worktree.delete: allow
    git.pr.create: prompt
    lock.acquire: allow
    lock.release: allow
    lock.force_release: prompt
    shell.exec: prompt

  # Pre-register agents by executable path
  pre_registered:
    - path: "/usr/local/bin/claude-code"
      trust_level: trusted
    - path: "/Applications/Cursor.app/Contents/MacOS/Cursor"
      trust_level: registered

  # Per-agent overrides (applied after defaults)
  overrides:
    claude-code-agent-1:
      git.pr.create: allow  # This specific agent can create PRs without prompt
      git.pr.create@tsk_01J6QX3N2Z8: deny  # But not for this specific task
```

### 5.3 Audit Log

**All MCP tool invocations logged**, including:
- Which capability was checked
- Permission result (granted/denied/prompted)
- User response to prompts
- Full tool parameters

**Three access methods:**

1. **SQLite:** Query `audit_log` table directly
2. **TUI View:** `Show Audit Log` command (searchable, filterable)
3. **File Export:** `tandemonium audit export --format=json`

### 5.4 Anomaly Detection

**Real-time TUI Notification:**
- Permission denied events
- Unusual operation patterns (rapid claims, bulk deletes)
- Lock conflicts
- Failed capability checks

**Implementation:** Status bar indicator + modal for critical anomalies.

### 5.5 Retention Policy

**No auto-pruning.** Audit log kept forever. User responsible for archival.

---

## 6. Error Handling

### 6.1 Database Errors

**Approach:** Modal error + read-only mode with YAML fallback.

**Flow:**
1. SQLite error detected
2. Modal appears explaining situation
3. Status bar shows persistent "READ-ONLY (DB unavailable)"
4. User can browse tasks from YAML exports
5. Write operations fail with clear message: "Cannot save: DB unavailable"
6. Recovery: `tandemonium repair` command

### 6.2 Terminal Errors

**Phase 1 (Command Runner):**
- Command spawn failure: Show error in terminal pane, allow retry
- Command timeout: Kill process group, show timeout message

**Phase 2 (PTY):**
- PTY spawn failure: Show error in terminal pane, offer "Open external terminal"
- PTY crash: Detect via process exit, show "Terminal disconnected" with restart option

### 6.3 Agent Communication Errors

- MCP timeout: Retry with backoff, log in audit
- Agent crash: Detect via liveness, notify user, do NOT auto-release lock
- Permission denied: Log, notify agent, show in TUI anomaly indicator

---

## 7. Implementation Phases

### Phase 1: Storage Migration (Week 1)
- [ ] Implement complete SQLite schema (all tables from section 1.2)
- [ ] Implement debounced YAML export (500ms, atomic writes)
- [ ] Migration tool from old `tasks.yml` (preserve all fields)
- [ ] Update `tandemonium-core` to use SQLite
- [ ] TUI watches SQLite, not YAML directory

### Phase 2: Locking & Liveness (Week 2)
- [ ] Task-level locking in SQLite
- [ ] Lease file creation/renewal
- [ ] Process monitoring fallback
- [ ] Stale lock detection + TUI warning
- [ ] `Force Release Lock` command

### Phase 3: Command Registry & Palette (Week 3)
- [ ] Implement Command struct and registry
- [ ] Migrate all keybindings to registry
- [ ] VS Code-style fuzzy matcher with frecency
- [ ] Derive palette, help screen, and hints from registry
- [ ] Separate `/` search

### Phase 4: MCP Permissions (Week 4)
- [ ] agent_capabilities table and CRUD
- [ ] Permission checking in MCP tool handlers
- [ ] TUI permission prompt modal
- [ ] Config file parsing for defaults/overrides
- [ ] Enhanced audit logging

### Phase 5: Polish (Week 5)
- [ ] Error handling (read-only mode)
- [ ] Anomaly detection and notifications
- [ ] Progress visibility improvements
- [ ] Documentation
- [ ] PRD reconciliation addendum

---

## 8. Non-Goals (Explicitly Deferred)

- GUI development
- Path-level file locking
- Auto-release of stale locks
- Audit log auto-pruning
- Multi-machine agent support
- WebSocket/HTTP MCP transport
- Full-screen terminal applications (vim, tmux)

---

## Appendix A: Decision Log

| Decision | Options Considered | Choice | Rationale |
|----------|-------------------|--------|-----------|
| Primary interface | TUI vs GUI vs Both | TUI (CLI is maintenance-only) | User preference, terminal-centric workflow |
| Storage | SQLite vs YAML vs PostgreSQL | SQLite + YAML export | Transactional + debuggable |
| YAML export format | Single file vs Per-task vs By-status | Per-task (no inline subtasks) | Clean git diffs, no duplication |
| YAML export timing | Real-time vs Debounced | Debounced (500ms) | Prevent event storms |
| YAML subtasks | Inline vs Reference | Reference (subtask_ids list) | Avoid duplicate representation |
| Dependencies schema | JSON array vs Junction table vs Closure | Junction table | Referential integrity, clean queries |
| Audit DB | Same DB vs Separate DB | Separate (audit.db) | Independent archival, no performance impact |
| Lock scope | Task vs Path vs Both | Task-level only | Simplicity for P0 |
| Liveness detection | Heartbeat vs Process vs Lease vs All | Lease + Process + lease_id | Debuggable + crash detection + PID reuse safety |
| Lock timeout | Auto-release vs Manual | Manual | Prefer intervention over data loss |
| Command palette | Single vs Dual | Dual (Ctrl+K, /) | Separate concerns |
| Command execution | Sync only vs Async support | Sync + Async (CommandIntent) | Non-blocking UI for DB/MCP operations |
| Input modes | Global shortcuts only vs Mode-aware | Mode-aware (Global/Context/Always) | Prevent conflicts with text input |
| Fuzzy algorithm | Sublime vs VS Code vs Contains | VS Code + frecency | Better ranking |
| Terminal approach | Full PTY vs Command runner vs Phased | Phased | De-risk P0, full PTY in P1 |
| External terminal | Hardcoded vs Configurable | Configurable (platform-specific + custom) | User choice, cross-platform |
| MCP permissions | Audit-only vs Capabilities vs Full RBAC | Capabilities + registration + audit | Balance security + usability |
| Permission defaults | Allow by default vs Deny by default | Deny until registered | Security-first, then user grants |
| Agent identity | Trust on first use vs Registration required | Registration required | Explicit user approval |
| Scope precedence | First match vs Most specific | Most specific wins, deny beats allow | Predictable, secure |
| Audit retention | Time vs Size vs Count vs Forever | Forever | User archives when needed |
| Error recovery | Modal vs Inline vs Fatal | Modal + read-only | Clear signal + usable fallback |

---

## Appendix B: File Layout

```
.tandemonium/
├── tasks.db                    # SQLite database (WAL mode)
│                               #   Tables: tasks, task_dependencies, acceptance_criteria,
│                               #           task_file_scope, task_tests, taskmaster_metadata,
│                               #           registered_agents, agent_leases, agent_capabilities
├── audit.db                    # SEPARATE SQLite database (append-only, no pruning)
│                               #   Tables: audit_log
├── config.yml                  # Project configuration
│                               #   - Agent permission defaults (registered_defaults, trusted_defaults)
│                               #   - Pre-registered agents
│                               #   - Per-agent overrides
│                               #   - Terminal configuration
├── tasks/                      # YAML exports (READ-ONLY, debounced, one file per task)
│   ├── tsk_01J6QX3N2Z8.yml    #   NO inline subtasks - subtasks are separate files
│   ├── tsk_01J6QX3N2Z9.yml    #   Subtask with parent_id reference
│   └── ...
├── leases/                     # Agent lease files (JSON)
│   ├── tsk_01J6QX3N2Z8.lease.json  # Includes lease_id for atomicity
│   └── ...
├── worktrees/                  # Git worktrees
│   ├── tsk_01J6QX3N2Z8/
│   └── ...
└── tasks.yml.backup            # Migrated legacy file (if applicable)
```

**Database Contents:**

| Database | Tables | Purpose |
|----------|--------|---------|
| `tasks.db` | tasks, task_dependencies, acceptance_criteria, task_file_scope, task_tests, taskmaster_metadata, registered_agents, agent_leases, agent_capabilities | All operational data |
| `audit.db` | audit_log | Append-only audit trail (separate for independent archival) |

---

## Appendix C: Schema Migration from Task Struct

This table maps `Task` struct fields to SQLite columns:

| Task Struct Field | SQLite Location | Notes |
|-------------------|-----------------|-------|
| `id` | `tasks.id` | |
| `slug` | `tasks.slug` | |
| `title` | `tasks.title` | |
| `description` | `tasks.description` | |
| `status` | `tasks.status` | |
| `assigned_to` | `tasks.assigned_to` | |
| `branch` | `tasks.branch` | |
| `worktree` | `tasks.worktree` | |
| `base_sha` | `tasks.base_sha` | |
| `pr_url` | `tasks.pr_url` | |
| `scope` | `task_file_scope` table | JSON columns for nested arrays |
| `acceptance_criteria` | `acceptance_criteria` table | Normalized |
| `progress` | `tasks.progress_mode`, `tasks.progress_value` | Denormalized for perf |
| `tests` | `task_tests` table | Normalized |
| `depends_on` | `task_dependencies` table | Junction table |
| `parent_id` | `tasks.parent_id` | Self-referential FK |
| `subtasks` | Derived via `parent_id` query | Not stored directly |
| `created_at` | `tasks.created_at` | |
| `updated_at` | `tasks.updated_at` | |
| `taskmaster_metadata` | `taskmaster_metadata` table | Normalized |

---

*This specification supersedes conflicting sections in `tandemonium-mvp-prd.md` regarding storage, interface, terminal scope, and multi-agent coordination. See Section 0 for explicit reconciliation.*
