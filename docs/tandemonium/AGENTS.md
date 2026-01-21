# Tandemonium - Agent Integration Guide

## 2026-01-15 Direction Update (No Worktrees + Praude Integration)
- Current direction favors a no-worktree, hybrid lock + patch-queue model.
- Praude is the source of truth for PRDs/CUJs; Tandemonium reads `.praude/specs/`.
- Use the latest design/plan docs for this direction:
  - `docs/plans/2026-01-15-coordination-spec-graph-design.md`
  - `docs/plans/2026-01-15-coordination-spec-graph-implementation-plan.md`
- Worktree guidance below is legacy and may be superseded unless explicitly needed.

Tandemonium is a task orchestration tool optimized for human-AI collaboration, featuring git worktree isolation for parallel task development.

## Quick Start

```bash
# Launch TUI (default)
tandemonium

# Or use CLI commands directly
tandemonium list
tandemonium add "Task title"
tandemonium start <task-id>
```

## TUI (Terminal User Interface)

The TUI is the primary interface. Running `tandemonium` with no arguments launches it.

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `j` / `k` or `↓` / `↑` | Navigate task list |
| `Enter` | Select task / confirm action |
| `Esc` | Cancel / go back |
| `Tab` | Cycle focus: list → detail → terminal |
| `n` | Create new task |
| `s` | Start task (creates git worktree) |
| `c` | Complete selected task |
| `d` | Delete selected task |
| `/` | Search tasks by title/slug |
| `Ctrl+K` | Open command palette |
| `Ctrl+O` | Toggle terminal interactive mode |
| `,` | Open settings |
| `?` | Show help modal |
| `q` | Quit (from task list) |

### Features

- **Task list with titles** - Shows slug (bold) and title (gray) for each task
- **Task detail panel** - View full task info, acceptance criteria, and status
- **Integrated terminal** - Each task has a PTY session in its worktree
- **Live updates** - File watcher auto-reloads when tasks.yml changes externally
- **Search/filter** - Quick fuzzy search across title, slug, and description

### Focus Modes

1. **TaskList** - Navigate and select tasks
2. **TaskDetail** - View task information, acceptance criteria
3. **Terminal** - Interact with PTY session (Ctrl+O for interactive mode)

## CLI Commands

All commands are available via CLI for scripting and automation:

```bash
# Project Setup
tandemonium init                          # Initialize in current repo

# Task Management
tandemonium add "Task title"              # Create new task
tandemonium list                          # Show all tasks
tandemonium list --status=todo            # Filter by status
tandemonium show <task-id>                # View task details

# Workflow
tandemonium start <task-id>               # Start task (creates worktree)
tandemonium complete <task-id>            # Complete task

# Task Updates
tandemonium update <task-id> --title="New title"
tandemonium update <task-id> --status=done

# Git Integration
tandemonium branch                        # List task branches
tandemonium branch <task-id> --switch     # Switch to task branch
tandemonium cleanup                       # Clean completed worktrees
```

### Output Formats

```bash
# Human-readable (default)
tandemonium list

# Machine-readable JSON
tandemonium list --json
tandemonium show <task-id> --json
```

## Project Structure

```
project/
├── .tandemonium/
│   ├── tasks.yml          # Task definitions (versioned, atomic writes)
│   ├── config.yml         # Project configuration
│   ├── activity.log       # Audit log (JSONL)
│   └── worktrees/         # Isolated git worktrees per task
├── CLAUDE.md              # Project instructions for Claude Code
└── AGENTS.md              # This file
```

### Task Storage (tasks.yml)

```yaml
version: 1
updated_at: "2025-01-08T10:30:00Z"
tasks:
  - id: tsk_01J6QX3N2Z8
    slug: login-form
    title: "Implement login form component"
    description: "Create React component for user login"
    status: in_progress  # todo | in_progress | review | done | blocked
    branch: feature/tsk-01J6QX3-login-form
    worktree: ".tandemonium/worktrees/tsk_01J6QX3N2Z8"
    acceptance_criteria:
      - text: "Form validates email format"
        completed: false
    depends_on: []
```

### Task States

- `todo` - Ready to work on
- `in_progress` - Currently being worked on
- `review` - Pending review
- `done` - Completed
- `blocked` - Waiting on dependencies

## Git Worktree Workflow

Each task gets an isolated git worktree to prevent conflicts:

```bash
# Start task - creates worktree and branch
tandemonium start login-form
# Creates: .tandemonium/worktrees/tsk_01J6QX3N2Z8/
# Branch: feature/tsk-01J6QX3-login-form

# Work in the worktree (TUI terminal auto-opens here)
cd .tandemonium/worktrees/tsk_01J6QX3N2Z8/

# Complete task - optionally creates PR
tandemonium complete login-form

# Clean up completed worktrees
tandemonium cleanup
```

### Parallel Development

Multiple tasks can run in parallel without conflicts:

```bash
# Terminal 1: Work on auth
tandemonium start auth-system

# Terminal 2: Work on API (different worktree)
tandemonium start api-refactor

# Each has isolated worktree, branch, and terminal session
```

## Development

### Building

```bash
# Build TUI
cargo build -p tandemonium-tui --release

# Build CLI
cargo build --bin tandemonium --release

# Install both
cargo install --path crates/tandemonium-tui
cargo install --path app/src-tauri --bin tandemonium
```

### Crate Structure

```
crates/
├── tandemonium-core/      # Shared library: storage, git, task model
├── tandemonium-terminal/  # PTY management, terminal emulation
└── tandemonium-tui/       # TUI application (ratatui)

app/src-tauri/
└── src/cli/               # CLI binary
```

### Key Dependencies

- **ratatui** - TUI framework
- **crossterm** - Terminal backend
- **wezterm-term** - Terminal emulation
- **portable-pty** - Cross-platform PTY
- **notify** - File watching
- **git2** - Git operations
- **serde_yaml** - YAML persistence

### Requirements

- macOS 11+ (Big Sur and later)
- Git 2.23+ (for `git worktree` and `git switch`)
- Rust toolchain

### Process Management

**CRITICAL:** NEVER use `killall`, `pkill node`, `pkill pnpm`, `pkill cargo`, or similar broad kill commands. The user runs multiple parallel AI agents that must never be interrupted.

**Forbidden commands:**
- `killall -9 node` / `pkill node`
- `killall -9 pnpm` / `pkill pnpm`
- `killall -9 cargo` / `pkill cargo`

**Why:** Killing all Node/pnpm/cargo processes disrupts Claude Code agents, Codex, and other critical workflows.

### Debugging

```bash
# Enable debug logging
RUST_LOG=debug tandemonium

# Run TUI with tracing
RUST_LOG=tandemonium_tui=debug cargo run -p tandemonium-tui
```

## Claude Code Integration

### Workflow with Claude Code

```bash
# Start session
tandemonium                    # Open TUI
# Press 's' to start a task   # Creates worktree
# Press Tab to focus terminal # Opens in worktree
# Press Ctrl+O for interactive mode

# Or use CLI in Claude Code
tandemonium list
tandemonium start <task-id>
tandemonium complete <task-id>
```

### Custom Slash Commands

Create `.claude/commands/next-task.md`:

```markdown
Find the next available task and start working on it.

1. Run `tandemonium list --status=todo`
2. Pick the first task without blocking dependencies
3. Run `tandemonium start <task-id>`
4. Summarize what needs to be implemented
```

## Skills

Skills provide specialized workflows. Invoke with: `Bash("openskills read <skill-name>")`

| Skill | When to Use |
|-------|-------------|
| `brainstorming` | Before writing code - refine ideas into designs |
| `systematic-debugging` | Bug investigation before proposing fixes |
| `test-driven-development` | Write tests first, watch fail, then implement |
| `git_worktree` | Managing worktrees, resolving conflicts |
| `verification-before-completion` | Before claiming work is done |

See full skills list at bottom of this file.

---

<skills_system priority="1">

## Available Skills

<!-- SKILLS_TABLE_START -->
<usage>
When users ask you to perform tasks, check if any of the available skills below can help complete the task more effectively. Skills provide specialized capabilities and domain knowledge.

How to use skills:
- Invoke: Bash("openskills read <skill-name>")
- The skill content will load with detailed instructions on how to complete the task
- Base directory provided in output for resolving bundled resources (references/, scripts/, assets/)

Usage notes:
- Only use skills listed in <available_skills> below
- Do not invoke a skill that is already loaded in your context
- Each skill invocation is stateless
</usage>

<available_skills>

<skill>
<name>brainstorming</name>
<description>Use when creating or developing anything, before writing code or implementation plans - refines rough ideas into fully-formed designs through structured Socratic questioning, alternative exploration, and incremental validation</description>
<location>project</location>
</skill>

<skill>
<name>condition-based-waiting</name>
<description>Use when tests have race conditions, timing dependencies, or inconsistent pass/fail behavior - replaces arbitrary timeouts with condition polling to wait for actual state changes, eliminating flaky tests from timing guesses</description>
<location>project</location>
</skill>

<skill>
<name>defense-in-depth</name>
<description>Use when invalid data causes failures deep in execution, requiring validation at multiple system layers - validates at every layer data passes through to make bugs structurally impossible</description>
<location>project</location>
</skill>

<skill>
<name>dispatching-parallel-agents</name>
<description>Use when facing 3+ independent failures that can be investigated without shared state or dependencies - dispatches multiple Claude agents to investigate and fix independent problems concurrently</description>
<location>project</location>
</skill>

<skill>
<name>executing-plans</name>
<description>Use when partner provides a complete implementation plan to execute in controlled batches with review checkpoints - loads plan, reviews critically, executes tasks in batches, reports for review between batches</description>
<location>project</location>
</skill>

<skill>
<name>finishing-a-development-branch</name>
<description>Use when implementation is complete, all tests pass, and you need to decide how to integrate the work - guides completion of development work by presenting structured options for merge, PR, or cleanup</description>
<location>project</location>
</skill>

<skill>
<name>git_worktree</name>
<description>Managing git worktrees for isolated task development in Tandemonium. Use when creating worktrees, resolving conflicts, cleaning up worktrees, troubleshooting worktree issues, or implementing path locking for concurrent task development.</description>
<location>project</location>
</skill>

<skill>
<name>receiving-code-review</name>
<description>Use when receiving code review feedback, before implementing suggestions, especially if feedback seems unclear or technically questionable - requires technical rigor and verification, not performative agreement or blind implementation</description>
<location>project</location>
</skill>

<skill>
<name>requesting-code-review</name>
<description>Use when completing tasks, implementing major features, or before merging to verify work meets requirements - dispatches superpowers:code-reviewer subagent to review implementation against plan or requirements before proceeding</description>
<location>project</location>
</skill>

<skill>
<name>root-cause-tracing</name>
<description>Use when errors occur deep in execution and you need to trace back to find the original trigger - systematically traces bugs backward through call stack, adding instrumentation when needed, to identify source of invalid data or incorrect behavior</description>
<location>project</location>
</skill>

<skill>
<name>sharing-skills</name>
<description>Use when you've developed a broadly useful skill and want to contribute it upstream via pull request - guides process of branching, committing, pushing, and creating PR to contribute skills back to upstream repository</description>
<location>project</location>
</skill>

<skill>
<name>subagent-driven-development</name>
<description>Use when executing implementation plans with independent tasks in the current session - dispatches fresh subagent for each task with code review between tasks, enabling fast iteration with quality gates</description>
<location>project</location>
</skill>

<skill>
<name>systematic-debugging</name>
<description>Use when encountering any bug, test failure, or unexpected behavior, before proposing fixes - four-phase framework (root cause investigation, pattern analysis, hypothesis testing, implementation) that ensures understanding before attempting solutions</description>
<location>project</location>
</skill>

<skill>
<name>test-driven-development</name>
<description>Use when implementing any feature or bugfix, before writing implementation code - write the test first, watch it fail, write minimal code to pass; ensures tests actually verify behavior by requiring failure first</description>
<location>project</location>
</skill>

<skill>
<name>testing-anti-patterns</name>
<description>Use when writing or changing tests, adding mocks, or tempted to add test-only methods to production code - prevents testing mock behavior, production pollution with test-only methods, and mocking without understanding dependencies</description>
<location>project</location>
</skill>

<skill>
<name>testing-skills-with-subagents</name>
<description>Use when creating or editing skills, before deployment, to verify they work under pressure and resist rationalization - applies RED-GREEN-REFACTOR cycle to process documentation by running baseline without skill, writing to address failures, iterating to close loopholes</description>
<location>project</location>
</skill>

<skill>
<name>using-git-worktrees</name>
<description>Use when starting feature work that needs isolation from current workspace or before executing implementation plans - creates isolated git worktrees with smart directory selection and safety verification</description>
<location>project</location>
</skill>

<skill>
<name>using-superpowers</name>
<description>Use when starting any conversation - establishes mandatory workflows for finding and using skills, including using Skill tool before announcing usage, following brainstorming before coding, and creating TodoWrite todos for checklists</description>
<location>project</location>
</skill>

<skill>
<name>verification-before-completion</name>
<description>Use when about to claim work is complete, fixed, or passing, before committing or creating PRs - requires running verification commands and confirming output before making any success claims; evidence before assertions always</description>
<location>project</location>
</skill>

<skill>
<name>writing-plans</name>
<description>Use when design is complete and you need detailed implementation tasks for engineers with zero codebase context - creates comprehensive implementation plans with exact file paths, complete code examples, and verification steps assuming engineer has minimal domain knowledge</description>
<location>project</location>
</skill>

<skill>
<name>writing-skills</name>
<description>Use when creating new skills, editing existing skills, or verifying skills work before deployment - applies TDD to process documentation by testing with subagents before writing, iterating until bulletproof against rationalization</description>
<location>project</location>
</skill>

<skill>
<name>yaml_atomic_ops</name>
<description>Implementing atomic YAML file operations with conflict detection and advisory locking. Use when reading/writing tasks.yml, handling concurrent access, implementing versioning, or debugging file corruption issues.</description>
<location>project</location>
</skill>

</available_skills>
<!-- SKILLS_TABLE_END -->

</skills_system>
