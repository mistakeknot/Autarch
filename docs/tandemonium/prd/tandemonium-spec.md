# Tandemonium Product Specification

**Version:** 3.6
**Date:** 2025-01-10
**Status:** Active

---

## Overview

Tandemonium is a terminal-based tool for solo developers that bridges the gap between **what to build** and **how to build it** with AI agents.

**Tagline:** Orchestrated chaos. From vision to launch.

### What It Does

1. **Plan strategically** — Guided flow to define vision, users, journeys, and MVP scope
2. **Refine tasks** — PM-mode transforms vague requests into detailed specs
3. **Execute with agents** — Orchestrate multiple Claude Code sessions in parallel
4. **Stay aligned** — Catch drift between your plan and implementation

### Key Principles

- **Free and open source** — MIT licensed, no accounts, no paywalls
- **Uses your Claude Code** — No separate API keys for coding agents
- **Local-first storage** — All task data, specs, and plans stay on your machine in `.tandemonium/` (API calls to Claude are transient, not stored remotely)
- **TUI-native** — Built for developers who live in the terminal
- **Solo-focused** — No team features, no enterprise complexity

---

## The Problem

### The Solo Developer's Dilemma

You have an idea. You're excited. You open Claude Code and start prompting.

Three weeks later:
- You've built features nobody asked for
- Your architecture is a mess
- A competitor launched something similar
- You still don't know if anyone will pay for this

**This happens because you skipped the most important step: figuring out what to build.**

AI coding tools are powerful at *how* to build. They're useless at *what* to build.

### Current AI Workflow Problems

1. **Vague-in, vague-out**: Half-formed requests → half-formed results → painful iteration
2. **Agent babysitting**: Watching one agent, missing blockers, context-switching overhead
3. **No audit trail**: What did the agent do? Why? What files?
4. **Isolation failures**: Multiple agents step on each other's work
5. **Scope creep**: Building features that don't matter

---

## The Solution

### Three-Phase Workflow

```
PLAN (once) → REFINE (per task) → EXECUTE (parallel agents)
     │              │                    │
     ▼              ▼                    ▼
  Strategic      Detailed            Code in
   Context        Specs             Worktrees
```

**Phase 1 - Plan (30-60 min, once per project):**
AI-guided conversation producing vision, users, journeys, features, MVP scope.

**Phase 2 - Refine (per task):**
PM Agent researches codebase, asks clarifying questions, produces structured spec with acceptance criteria.

**Phase 3 - Execute (parallel):**
Coding agents work against approved specs in isolated git worktrees.

---

## Core Value Propositions

### 1. Strategic Planning Layer (NEW)

Before writing any code, establish what you're building:
- **Vision**: One-sentence purpose, problem statement, differentiators
- **Users**: Specific personas, anti-personas, context
- **Journeys**: Critical user journeys (CUJs) that must work flawlessly
- **Features**: Prioritized list mapped to journeys
- **MVP**: Ruthlessly scoped minimum viable product

These artifacts become context for every AI interaction.

### 2. PM Agent Refinement Layer

Transform vague requests into detailed specs:
- Codebase research using full Claude Code toolset
- Sequential/adaptive clarifying questions
- Structured output: requirements, acceptance criteria, files to modify
- Strategic context injection (which CUJ, which feature, MVP status)

### 3. Multi-Agent Fleet Management

Run up to 4 coding agents in parallel:
- Each agent isolated in its own git worktree
- Real-time status visibility (working/blocked/review-ready/done)
- Surface blockers with question + context, quick response buttons
- Optional auto-accept mode (`-y`) for trusted workflows
- Health monitoring with optional auto-restart

### 4. Structured Review Workflow

Human-in-the-loop at key checkpoints:
- Batch review queue for multiple completed tasks
- File-by-file diff navigation with inline unified diffs
- Strategic alignment check (does this advance a CUJ?)
- Drift detection (did we go outside MVP scope?)

### 5. Canonical Artifacts

Every project produces durable, reviewable documents:
- **Plan documents**: Committed to git, inform all future work
- **Task specs**: Machine-readable YAML + human-readable markdown
- **Audit trail**: What was requested, what was approved, what was built

---

## User Persona

### Primary: Solo Developer

- Has an idea, wants to ship fast
- Manages 1-4 concurrent tasks
- Wants to "set and forget" agents on well-defined work
- Reviews results in batches
- Cost-conscious (uses existing Claude subscription)

### The Name

> *"Tandem" (together, in coordination) + "Pandemonium" (wild chaos)*

Building software is chaos. Tandemonium gives you the tools to orchestrate that chaos productively.

---

## Architecture

### Process Model

```
┌─────────────────────────────────────────────────────────────┐
│                         TUI Process                          │
│  (Bubble Tea app, main UI, SQLite writer, notifications)    │
└─────────────────────────────────────────────────────────────┘
         │              │              │              │
    pipe-pane      pipe-pane      pipe-pane      pipe-pane
         │              │              │              │
    ┌────────┐    ┌────────┐    ┌────────┐    ┌────────┐
    │ tmux   │    │ tmux   │    │ tmux   │    │ tmux   │
    │session │    │session │    │session │    │session │
    │Agent 1 │    │Agent 2 │    │Agent 3 │    │Agent 4 │
    └────────┘    └────────┘    └────────┘    └────────┘
         │              │              │              │
    ┌────────┐    ┌────────┐    ┌────────┐    ┌────────┐
    │worktree│    │worktree│    │worktree│    │worktree│
    │  /w1   │    │  /w2   │    │  /w3   │    │  /w4   │
    └────────┘    └────────┘    └────────┘    └────────┘
```

### Key Design Decisions

- **No daemon for MVP**: TUI is the only process. Simpler architecture.
- **Agents survive TUI quit**: tmux sessions persist independently. Reattach on next launch.
- **Single TUI at a time**: One TUI instance per project (enforced via file lock).
- **Auto-resume on startup**: TUI reconnects to existing tmux sessions.

### tmux Output Streaming Contract

**The Problem:** Reliable completion/blocker detection requires clean, recoverable output streaming from tmux sessions.

**Output Pipeline:**
```
tmux session → pipe-pane → log file → TUI reader → detection pipeline
```

**File Contract:**
```
.tandemonium/sessions/<session-id>.log   # Append-only output log
.tandemonium/sessions/<session-id>.meta  # JSON metadata (offsets, state)
```

**Log File Format:**
- Raw bytes from tmux `pipe-pane` (append-only)
- ANSI escape sequences preserved in file (stripped at display time)
- UTF-8 with replacement for invalid sequences

**Metadata File (`*.meta`):**
```json
{
  "session_id": "tand-TAND-004",
  "task_id": "TAND-004",
  "created_at": "2025-01-10T14:30:00Z",
  "last_read_offset": 8192,
  "last_detection_state": "working",
  "last_detection_at": "2025-01-10T14:45:30Z"
}
```

**Streaming Contract:**
```go
type SessionLogReader struct {
    sessionID    string
    logFile      *os.File
    metaFile     string
    lastOffset   int64
    outputChan   chan []byte
}

func (r *SessionLogReader) Stream(ctx context.Context) <-chan []byte {
    go func() {
        defer close(r.outputChan)
        for {
            select {
            case <-ctx.Done():
                return
            default:
                // Read new bytes from last offset
                data, err := r.readFromOffset()
                if err != nil {
                    continue
                }
                if len(data) > 0 {
                    r.outputChan <- data
                    r.updateOffset(len(data))
                }
                time.Sleep(100 * time.Millisecond)  // Poll interval
            }
        }
    }()
    return r.outputChan
}

// Recovery: on TUI restart, read from last_read_offset
func (r *SessionLogReader) RecoverFromMeta() error {
    meta, err := r.loadMeta()
    if err != nil {
        return err  // Fresh start, offset = 0
    }
    r.lastOffset = meta.LastReadOffset
    return nil
}
```

**ANSI Handling:**
```go
// Strip ANSI for detection, preserve for display
func normalizeForDetection(raw []byte) string {
    // Remove ANSI escape sequences
    stripped := ansiRegex.ReplaceAll(raw, nil)
    // Normalize newlines
    normalized := bytes.ReplaceAll(stripped, []byte("\r\n"), []byte("\n"))
    // Replace invalid UTF-8
    return strings.ToValidUTF8(string(normalized), "�")
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
```

**Deduplication (live vs recovery):**
- On restart, TUI compares `last_read_offset` vs current file size
- If offset < size: replay from offset (recovery mode)
- If offset >= size: live tailing (normal mode)
- Detection pipeline sees same bytes once, regardless of restart

**Single TUI Lock:**
```go
const lockPath = ".tandemonium/tui.lock"

type TUILock struct {
    file *os.File
    pid  int
}

func AcquireLock() (*TUILock, error) {
    f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
    if err != nil {
        return nil, err
    }

    // Try non-blocking exclusive lock
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
        // Lock failed - check if stale BEFORE closing
        if isStale(f) {
            // Stale lock - force acquire (file still open)
            return forceAcquire(f)
        }
        f.Close()
        return nil, fmt.Errorf("another TUI instance is running (pid in lock file)")
    }

    // Write PID for stale detection
    f.Truncate(0)
    f.Seek(0, 0)
    fmt.Fprintf(f, "%d\n%d\n", os.Getpid(), time.Now().Unix())
    f.Sync()

    return &TUILock{file: f, pid: os.Getpid()}, nil
}

func isStale(f *os.File) bool {
    f.Seek(0, 0)
    var pid int
    var ts int64
    if _, err := fmt.Fscanf(f, "%d\n%d\n", &pid, &ts); err != nil {
        return true  // Can't parse - treat as stale
    }

    // Check if process exists (signal 0 = just check existence)
    if err := syscall.Kill(pid, 0); err != nil {
        return true  // Process dead, lock is stale
    }
    return false
}

func forceAcquire(f *os.File) (*TUILock, error) {
    // File already open, just overwrite PID and claim lock
    f.Truncate(0)
    f.Seek(0, 0)
    fmt.Fprintf(f, "%d\n%d\n", os.Getpid(), time.Now().Unix())
    f.Sync()
    return &TUILock{file: f, pid: os.Getpid()}, nil
}

func (l *TUILock) Release() error {
    if l.file != nil {
        syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
        return l.file.Close()
    }
    return nil
}
```

### Tech Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| **Language** | Go 1.22+ | Fast iteration, excellent TUI libraries, single binary |
| **TUI Framework** | Bubble Tea + Lip Gloss | Charmbracelet ecosystem, active community |
| **Process Isolation** | tmux | Sessions survive TUI quit, proven reliability |
| **PM Agent** | Claude Code CLI (default) / Claude API | No extra setup; API mode available for structured output |
| **Coding Agent** | Claude Code CLI | User's existing subscription |
| **Persistence** | SQLite (WAL mode) | Zero setup, survives crashes |
| **Config** | TOML | Human-readable, standard |
| **Git** | Native commands | Worktree isolation |

### Dependencies

**Required:**
- Claude Code CLI (`claude` command available)
- Git (for worktree isolation)
- tmux (for session persistence)

**Optional:**
- `ANTHROPIC_API_KEY` environment variable (for PM Agent API mode)
- `gh` CLI (for push/PR operations)

### PM Agent Configuration

The PM Agent can run in two modes. Choose based on your setup:

| Mode | Interface | Auth | Cost | Pros/Cons |
|------|-----------|------|------|-----------|
| **Claude Code** | Claude Code CLI | Your subscription | Included | Simpler setup; uses your existing auth |
| **API Direct** | Claude API | `ANTHROPIC_API_KEY` | ~$0.02/task | Structured output; faster; works headless |

**Default:** Claude Code mode (no extra setup required).

```toml
# .tandemonium/config.toml (or ~/.config/tandemonium/config.toml)
[pm_agent]
mode = "claude-code"      # "claude-code" | "api"
# api_key handled via env var only (see below)
```

**API mode setup:**
```bash
# Environment variable ONLY
export ANTHROPIC_API_KEY="sk-ant-..."
```

**⚠️ Security Policy:**
- API keys must **ONLY** be set via environment variable (`ANTHROPIC_API_KEY`)
- Keys must **NEVER** be stored in any config file (project or user)
- Tandemonium will refuse to start if it detects secrets in config files
- Rationale: config files can be accidentally committed, shared, or logged

**No API key + API mode?** Falls back to Claude Code mode automatically.

---

## Planning Flow

### Overview

The planning flow guides strategic thinking before coding. Each stage produces a markdown document that becomes context for all future work.

```
Vision → Users → Journeys → Features → MVP
  │        │        │          │        │
  ▼        ▼        ▼          ▼        ▼
vision.md users.md journeys.md features.md mvp.md
```

**Time investment:** 30-60 minutes (one time per project)

### Stage 1: Vision

**Goal**: Define what you're building and why.

**Output**: `.tandemonium/plan/vision.md`

```markdown
# Vision

## Vision Statement
Zero-effort invoicing for freelancers who hate admin work.

## Problem
Freelancers spend 2-5 hours per month creating invoices and chasing payments.

## Solution
An invoicing app that generates invoices automatically from time tracking.

## Key Differentiators
- Automatic invoice generation from time tracking
- Zero ongoing manual work after setup
- Designed for creative freelancers, not accountants

## 6-Month Success Criteria
- 100 paying users ($15/month)
- <5 minutes from signup to first invoice
```

### Stage 2: Users

**Goal**: Get specific about who you're building for.

**Output**: `.tandemonium/plan/users.md`

- Primary persona (detailed)
- Secondary personas (brief)
- Anti-personas (who you're NOT building for)

### Stage 3: Journeys (CUJs)

**Goal**: Map critical paths users must complete successfully.

**Output**: `.tandemonium/plan/journeys.md`

**Format:** YAML frontmatter + markdown body for machine parsing

```markdown
---
# journeys.md frontmatter - parsed for CUJ tracking
cujs:
  - id: CUJ-1
    name: First Invoice
    priority: critical
    features:
      - id: time-entry
        name: Time entry integration
        status: pending
      - id: invoice-gen
        name: Invoice generation
        status: pending
      - id: pdf-render
        name: PDF rendering
        status: pending
      - id: email-delivery
        name: Email delivery
        status: pending
  - id: CUJ-2
    name: Get Paid
    priority: high
    features:
      - id: payment-links
        name: Stripe payment links
        status: pending
      - id: reminders
        name: Automated reminders
        status: pending
---

# Critical User Journeys

## CUJ-1: First Invoice (Priority: Critical)

**Trigger**: User completed work and needs to bill.

**Success**: Invoice sent within 5 minutes.

**Steps**:
1. Select client and date range
2. System pulls time entries
3. Review/adjust line items
4. Click "Send"
5. Client receives PDF via email

**Required Features**:
- Time entry integration
- Invoice generation
- PDF rendering
- Email delivery
```

**Why structured frontmatter:**
- Enables CUJ progress tracking in TUI (`3/4 features complete`)
- Powers drift detection (task modifies file outside any CUJ → warning)
- Links tasks to features to journeys in a parseable way
- Human-readable markdown body preserved for documentation

### Feature ID Naming Convention

**Canonical format:** `kebab-case`, lowercase, alphanumeric + hyphens only.

```
✓ pdf-render
✓ time-entry
✓ payment-links
✗ pdf_render       (underscores)
✗ PDF-Render       (uppercase)
✗ pdfRender        (camelCase)
```

**Source of truth:** Feature IDs are defined in `journeys.md` frontmatter. All references elsewhere (specs, tasks, MVP scope) must match exactly.

**Validation:** Tandemonium validates feature ID references on:
- Task spec creation (PM Agent)
- MVP scope parsing
- Drift detection

```go
var featureIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

func ValidateFeatureID(id string, knownIDs []string) error {
    if !featureIDPattern.MatchString(id) {
        return fmt.Errorf("invalid feature ID %q: must be kebab-case", id)
    }
    for _, known := range knownIDs {
        if id == known {
            return nil
        }
    }
    return fmt.Errorf("unknown feature ID %q: not found in journeys.md", id)
}
```

### Stage 4: Features

**Goal**: List all features, map to journeys, prioritize.

**Output**: `.tandemonium/plan/features.md`

- Features grouped by CUJ
- Priority score (value / complexity)
- Dependencies noted

### Stage 5: MVP

**Goal**: Ruthlessly scope the minimum viable product.

**Output**: `.tandemonium/plan/mvp.md`

**Format:** YAML frontmatter + markdown body for machine parsing

```markdown
---
# mvp.md frontmatter - parsed for scope checking
mvp:
  hypothesis: "Freelancers will pay $15/month for zero-effort invoicing"
  included_features:
    - time-entry
    - invoice-gen
    - pdf-render
    - email-delivery
    - payment-links
    - payment-tracking
    - reminders
    - client-mgmt
  excluded_features:
    - time-integrations   # Reason: manual entry first
    - multi-currency      # Reason: USD only
    - mobile-app          # Reason: web only
  architecture:
    backend: "Python + FastAPI"
    database: "PostgreSQL"
    frontend: "HTMX + Tailwind"
---

# MVP Scope

## Hypothesis to Validate
Freelancers will pay $15/month for zero-effort invoicing.

## Included Features (8)
- Manual time entry
- Invoice generation
- PDF rendering
- Email delivery
- Stripe payment links
- Payment tracking
- Automated reminders
- Client management

## Excluded from MVP
- Time tracking integrations (manual entry first)
- Multiple currencies (USD only)
- Mobile app (web only)

## Architecture Decisions
- Backend: Python + FastAPI
- Database: PostgreSQL
- Frontend: HTMX + Tailwind
```

**How drift detection uses this:**
```go
func checkMVPBoundary(task *Task, mvp *MVPConfig) DriftResult {
    // No-plan mode or untracked task: skip MVP boundary checks
    if mvp == nil || task.FeatureID == nil {
        return DriftResult{Level: DriftNone}
    }

    featureID := *task.FeatureID

    // Task references a feature in excluded_features?
    for _, excluded := range mvp.ExcludedFeatures {
        if featureID == excluded {
            return DriftResult{
                Level:   DriftBlock,
                Message: fmt.Sprintf("Feature %q excluded from MVP", excluded),
                Line:    mvp.ExclusionLine(excluded),
            }
        }
    }
    // Task doesn't map to any known feature?
    if featureID != "" && !mvp.HasFeature(featureID) {
        return DriftResult{
            Level:   DriftWarn,
            Message: "Feature not in MVP scope - possible scope creep",
        }
    }
    return DriftResult{Level: DriftNone}
}
```

### Planning is Optional

- **Skip entirely**: `tand --skip-planning` or config option
- **Skip stages**: Complete Vision + MVP, skip middle stages
- **Revisit later**: Re-enter planning mode to update decisions

### Partial Planning Behavior Matrix

Planning stages can be completed independently. Features degrade gracefully based on what's available:

| Plan State | CUJ Tracking | Feature Validation | MVP Boundary | Task Derivation | Persona |
|------------|--------------|-------------------|--------------|-----------------|---------|
| **Full plan** (all stages) | ✓ Full | ✓ Strict | ✓ Full | ✓ All CUJs | From users.md |
| **Vision + MVP only** | ✗ Disabled | ✗ None | ✓ Full | ✗ Disabled | "user" |
| **Journeys + MVP** (no users) | ✓ Full | ✓ Strict | ✓ Full | ✓ All CUJs | "user" |
| **Journeys only** (no MVP) | ✓ Full | ✓ Registry | ⚠ Warn-only | ✓ All CUJs | "user" |
| **MVP only** | ✗ Disabled | ⚠ Loose* | ✓ Full | ✗ Disabled | "user" |
| **No plan** | ✗ Disabled | ✗ None | ✗ Disabled | ✗ Disabled | "user" |

*Loose validation: feature IDs in MVP are accepted without checking journeys registry.

**Feature registry source of truth:**
1. If `journeys.md` exists → features defined in journeys frontmatter
2. Else if `mvp.md` exists → features in `included_features` + `excluded_features`
3. Else → no feature registry (validation disabled)

```go
func getFeatureRegistry(plan *PlanConfig) []string {
    if plan.Journeys != nil {
        return plan.Journeys.AllFeatureIDs()
    }
    if plan.MVP != nil {
        return append(plan.MVP.IncludedFeatures, plan.MVP.ExcludedFeatures...)
    }
    return nil // No registry - validation disabled
}
```

### No-Plan Mode Semantics

When planning is skipped (`--skip-planning` or no plan documents exist), these behaviors change:

| Feature | With Plan | No Plan |
|---------|-----------|---------|
| CUJ field in spec | Required | `null` (untracked) |
| Feature ID field | Required | `null` (untracked) |
| MVP scope check | Warns on exclusion | Disabled |
| Drift detection | Full (file + scope) | Files only |
| PM persona fallback | From `users.md` | "user" (generic) |
| Task derivation | Available | Disabled (no source) |
| Review alignment | Shows CUJ progress | Shows "Untracked task" |

**Spec YAML in no-plan mode:**
```yaml
strategic_context:
  cuj_id: null           # No plan = no CUJ mapping
  cuj_name: null
  feature_id: null       # Untracked feature
  mvp_included: null     # Unknown (no MVP defined)
```

**Review UI in no-plan mode:**
```
STRATEGIC ALIGNMENT
───────────────────
ℹ️ No plan configured - task is untracked

Drift detection (files only):
✓ All files in spec were modified
⚠ 1 extra file modified: src/utils/helper.py
```

**PM refinement in no-plan mode:**
- Skips "which CUJ does this advance?" question
- Uses generic "user" persona for story generation
- Still generates full spec with AC, files_to_modify, etc.

**Enabling planning later:**
```bash
tand plan   # Starts planning flow, creates plan documents
# After planning, existing tasks remain "untracked"
# New tasks get proper CUJ/feature mapping
```

---

## Task Flow

### Task Lifecycle

```
┌────────┐     ┌──────────┐     ┌───────┐     ┌──────────┐     ┌─────────┐     ┌────────┐
│ DRAFT  │────▶│ REFINING │────▶│ READY │────▶│ ASSIGNED │────▶│ WORKING │────▶│ REVIEW │
└────────┘     └──────────┘     └───────┘     └──────────┘     └─────────┘     └────────┘
     │                               │                              │               │
     │ (quick mode)                  │                         ┌────────┐      ┌────────┐
     └───────────────────────────────┼────────────────────────▶│BLOCKED │      │  DONE  │
                                     │                         └────────┘      └────────┘
                                     │                              │               │
                                     │                         ┌────────┐      ┌──────────┐
                                     │                         │ PAUSED │      │ REJECTED │
                                     │                         └────────┘      └──────────┘
                                     │                              │
                                     └──────────────────────────────┘
                                           (resume paused task)
```

### Task States

| Status | Meaning | Transitions To |
|--------|---------|----------------|
| `draft` | Raw input, not yet refined | `refining`, `assigned` (quick mode) |
| `refining` | PM Agent working on spec | `ready`, `draft` (cancelled), `failed` (API error) |
| `ready` | Spec complete, awaiting assignment | `assigned` |
| `assigned` | Agent allocated, worktree being created | `working`, `failed` (worktree error) |
| `working` | Agent actively executing | `blocked`, `review`, `paused`, `failed` (crash) |
| `blocked` | Agent waiting for human input | `working` (after unblock), `failed` (timeout) |
| `paused` | Manually paused or interrupted | `ready` (reassign), `working` (resume) |
| `review` | Work complete, awaiting human review | `done`, `rejected`, `failed` (merge conflict) |
| `done` | Approved and merged | Terminal state |
| `rejected` | Review failed, needs rework | `ready` (with feedback) |
| `failed` | Error requiring intervention | `ready` (retry), `draft` (re-spec) |

**`failed` state details:**
- Entered when: tmux session crashes, worktree creation fails, PM refinement errors, merge conflicts
- Requires human intervention before retry
- Error message stored in `task_state.error_message` (SQLite)
- UI shows error with options: `[r]etry` (→ `ready`), `[e]dit spec` (→ `draft`), `[d]elete task`

### Session States (Agent-Level)

Separate from task status, each agent session has an operational state:

| State | Meaning |
|-------|---------|
| `starting` | tmux session being created |
| `working` | Agent actively producing output |
| `idle` | Agent at prompt, no recent output |
| `blocked` | Blocker detected, awaiting input |
| `paused` | Session suspended (task may be `paused` or `working`) |
| `complete` | Agent signaled completion |
| `failed` | Session crashed or errored |

### Task Creation

**Path 1: Derive from Plan**
```
User: "Derive tasks from CUJ-2"
System: Analyzes journeys.md and features.md
System: Suggests tasks for incomplete features
User: Approves/edits suggestions
```

**Path 2: Manual Creation**
```
User: "New task: add dark mode"
System: Creates draft task
System: Offers PM refinement (or skip with `N`)
```

### Task Derivation View

```
┌─ DERIVE TASKS ─ CUJ-2: Get Paid ────────────────────────────────────────────┐
│                                                                             │
│ This journey requires 3 features. 1 complete, 2 remaining.                  │
│                                                                             │
│ ✓ Feature: Invoice sending                                                  │
│   └─ TAND-003 (DONE): Email delivery with PDF attachment                   │
│                                                                             │
│ ○ Feature: Payment reminders                   [MVP: INCLUDED]              │
│   │                                                                         │
│   │  Suggested task:                                                        │
│   │  "Implement automated payment reminder emails at 7, 14, 30 days"       │
│   │                                                                         │
│   │  Context from plan:                                                     │
│   │  • User: Freelancers who "hate chasing payments"                       │
│   │  • Must feel automatic, not spammy                                      │
│   │                                                                         │
│   └─ [c]reate task    [e]dit suggestion    [s]kip                          │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ [c]reate    [Tab] next feature    [Esc] back                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## PM Agent

### Capabilities

- **Full Claude Code toolset**: Read, grep, glob, git, web fetch
- **Research only**: PM creates spec, does NOT create scaffolding files
- **Strategic context**: Injects vision, users, CUJs into research
- **Web research**: Can browse documentation, APIs, examples

### Refinement Flow

1. PM reads rough task description
2. PM researches codebase (searches, reads files)
3. PM asks clarifying questions (sequential, adaptive)
4. User answers or skips
5. PM generates structured spec with user story

### User Story Generation

The PM Agent generates a user story as part of refinement:

**Format:** `As a [persona], I want [goal] so that [benefit].`

**Generation rules:**
- Persona comes from `users.md` (primary persona by default)
- Goal derived from task description + clarifying answers
- Benefit linked to CUJ success criteria

**Example flow:**
```
Raw input: "add PDF invoices"

PM researches → finds Invoice model, no PDF generation

PM asks: "Should PDFs include the company logo?"
User: "Yes, from settings"

PM generates:
  user_story:
    text: "As a freelancer, I want to generate branded PDF invoices
           so that I can bill clients professionally without manual work."
```

**User can edit:** Story is shown during spec review. User can modify before approving.

### Spec Storage: Committed vs Runtime

Task data is split between **committed specs** (YAML files) and **runtime state** (SQLite):

| Data | Storage | Rationale |
|------|---------|-----------|
| Requirements, AC, user story | YAML (committed) | Audit trail, reviewable |
| Execution state, results | SQLite (gitignored) | Changes frequently, no merge conflicts |

### Spec YAML Schema (Committed)

```yaml
# .tandemonium/specs/TAND-004.yaml (committed to git)
---
id: "TAND-004"
title: "Invoice PDF Generation"
created_at: "2025-01-10T14:30:00Z"

# Strategic context (links to planning docs)
strategic_context:
  cuj_id: "CUJ-1"           # Reference to journeys.md
  cuj_name: "First Invoice"
  feature_id: "pdf-render"  # kebab-case, matches journeys.md
  mvp_included: true

# User story (generated by PM Agent, editable by user)
user_story:
  text: "As a freelancer, I want to generate professional PDF invoices so that I can bill clients without manual formatting."
  hash: "a1b2c3d4"          # SHA256 prefix, for drift detection

# Task content
summary: |
  Generate professional PDF invoices from invoice data.

requirements:
  - "Generate PDF from Invoice model"
  - "Include: logo, client info, line items, totals"
  - "File size under 500KB"
  - "Generation time under 2 seconds"

acceptance_criteria:
  - id: "ac-1"
    description: "generate_invoice_pdf(invoice) -> bytes function exists"
  - id: "ac-2"
    description: "PDF includes all required sections"
  - id: "ac-3"
    description: "Tests cover: basic invoice, many line items, missing logo"
  - id: "ac-4"
    description: "All tests pass"

files_to_modify:
  - action: "create"
    path: "src/invoices/pdf_generator.py"
    description: "Main PDF generation logic"
  - action: "create"
    path: "src/templates/invoice_pdf.html"
    description: "HTML template for PDF rendering"
  - action: "modify"
    path: "src/invoices/__init__.py"
    description: "Export new function"
  - action: "create"
    path: "tests/test_pdf_generator.py"
    description: "Unit tests"

# Metadata
complexity: "medium"          # low | medium | high
estimated_minutes: 25
priority: 1                   # 1 = highest
```

### Runtime State (SQLite)

Volatile fields stored in `state.db`, not in YAML:

```sql
-- Task runtime state (changes frequently)
CREATE TABLE task_state (
  task_id TEXT PRIMARY KEY,
  status TEXT NOT NULL,           -- draft|refining|ready|assigned|working|blocked|paused|review|done|rejected|failed
  updated_at TEXT NOT NULL,
  assigned_agent TEXT,
  branch_name TEXT,
  worktree_path TEXT,
  started_at TEXT,
  completed_at TEXT,
  story_hash_at_start TEXT,       -- For drift detection
  error_message TEXT              -- If status = failed
);

-- Acceptance criteria verification (updated during review)
CREATE TABLE ac_verification (
  task_id TEXT,
  ac_id TEXT,
  verified INTEGER DEFAULT 0,
  verified_at TEXT,
  verified_by TEXT,               -- "user" | "agent" | "test"
  PRIMARY KEY (task_id, ac_id)
);

-- Execution results
CREATE TABLE task_results (
  task_id TEXT PRIMARY KEY,
  files_changed TEXT,             -- JSON array
  tests_passed INTEGER,
  drift_detected INTEGER,
  drift_details TEXT,             -- JSON array
  merge_commit_sha TEXT
);
```

**Human-readable export:** `tand show TAND-004` joins YAML + SQLite for full view.

### Git Commit Behavior

**Tandemonium creates git commits automatically** at specific points. This is required for the "canonical specs" guarantee.

| Event | Auto-Commit? | Commit Message |
|-------|--------------|----------------|
| Planning stage completed | ✓ Yes | `docs(tandemonium): add vision.md` |
| Task spec reaches `ready` | ✓ Yes | `chore(tandemonium): add spec TAND-004` |
| Quick task created | ✓ Yes | `chore(tandemonium): add quick task TAND-007` |
| Task approved (merge) | ✗ No* | User merges worktree manually or via `[a]pprove` |
| Config changes | ✗ No | User commits config.toml manually |

*Task approval triggers `git merge` but does NOT auto-commit the merge. The merge commit is created by git as part of the merge operation.

**Auto-commit rules:**
```go
func autoCommitSpec(taskID string, specPath string) error {
    // Only commit if repo is clean (no uncommitted changes)
    if !isRepoClean() {
        // Stage only the spec file, not everything
        if err := gitAdd(specPath); err != nil {
            return err
        }
    }

    msg := fmt.Sprintf("chore(tandemonium): add spec %s", taskID)
    return gitCommit(msg, specPath)  // Commit only this file
}
```

**Dirty repo handling:**
- If repo has uncommitted changes, Tandemonium stages only its files (not user's work)
- Uses `git add <specific-file>` + `git commit` with explicit paths
- Never runs `git add .` or commits user's unrelated work

**Config option to disable:**
```toml
[git]
auto_commit_specs = true    # default: true
auto_commit_plans = true    # default: true
```

When disabled, Tandemonium writes files but leaves committing to the user. Recovery guarantees are weaker in this mode (specs exist but may not be in git history).

---

## Coding Agent

### Supported Agents (MVP)

- **Claude Code CLI only** — perfect one integration first

### Spec Delivery

- **Prompt injection + file backup**: Spec prepended to initial prompt AND written to `.tandemonium/task-spec.md` in worktree
- **Strategic context**: Vision + MVP architecture decisions included

### Auto-Accept Mode

- **`-y` / `--autoyes` flag**: Auto-accept safe prompts (like Claude Squad)
- **Config option**: `[coding_agent] auto_accept = true`
- **Safety**: Only auto-accepts known-safe prompts, never destructive actions

### Health Monitoring

- **Health check interval**: Configurable polling (default 30s)
- **Auto-restart on failure**: Optional restart of crashed sessions
- **Session states**: Working, Complete, Failed, Paused, Starting

### Completion Detection

**Hybrid approach with fallbacks:**

| Method | How | Reliability |
|--------|-----|-------------|
| **Magic string** | Agent outputs `TASK_COMPLETE` | High (if prompted correctly) |
| **Behavioral inference** | Detect idle prompt, no activity | Medium |
| **Inactivity timeout** | No output for N minutes | Fallback |
| **Manual override** | User presses `[m]ark complete` | Always available |

**Detection patterns:**
```go
var completionPatterns = []string{
    `TASK_COMPLETE`,
    `(?i)task.*completed?.*successfully`,
    `(?i)all.*tests.*pass(ed|ing)?`,
    `(?i)implementation.*complete`,
}

var idlePromptPatterns = []string{
    `\$\s*$`,           // Shell prompt
    `>\s*$`,            // Generic prompt
    `claude.*>\s*$`,    // Claude Code prompt
}
```

**Timeout behavior:**
```toml
[coding_agent]
inactivity_timeout = "15m"      # After 15 min no output
inactivity_action = "prompt"    # "prompt" | "complete" | "ignore"
```

When timeout triggers with `action = "prompt"`:
```
┌─ INACTIVITY DETECTED ─────────────────────────────────────────┐
│ agent-1 has been idle for 15 minutes.                         │
│                                                               │
│ Last output: "All tests passing. Ready for review."           │
│                                                               │
│ [m]ark complete    [c]ontinue waiting    [a]ttach to check   │
└───────────────────────────────────────────────────────────────┘
```

### Blocker Detection

**Detection patterns:**
```go
var blockerPatterns = []string{
    // Direct questions (specific phrasing, not just any "?")
    `(?i)should I (use|choose|prefer|go with)`,
    `(?i)which (approach|option|method|library)`,
    `(?i)do you want me to`,
    `(?i)would you (like|prefer)`,
    `(?i)before I (proceed|continue|start)`,
    `(?i)can you (clarify|confirm|specify)`,
    `(?i)what (should|would) you (like|prefer)`,

    // Waiting indicators (explicit)
    `(?i)waiting for.*input`,
    `(?i)please (provide|specify|confirm)`,
    `(?i)need.*input.*to continue`,

    // Error states requiring intervention
    `(?i)error:.*permission denied`,
    `(?i)fatal:.*not a git repository`,
    `(?i)command not found`,
    `(?i)cannot find module`,
    `(?i)authentication (failed|required)`,
}

// NOTE: We intentionally DO NOT match `\?\s*$` (any line ending with "?")
// because this causes false positives on:
//   - Log output: "Processing item 5 of 10?"
//   - Comments: "// TODO: should this be async?"
//   - Test output: "Running test_auth?"
// Instead, we match specific question phrasing above.

// Auto-respond patterns: ORDERED slice (not map) to ensure deterministic matching
// Patterns are evaluated in order; first match wins.
var autoRespondPatterns = []AutoRespondRule{
    {Pattern: regexp.MustCompile(`(?i)do you want to proceed\??\s*\[Y/n\]`), Response: "Y", Priority: 1},
    {Pattern: regexp.MustCompile(`(?i)continue\??\s*\[y/N\]`), Response: "y", Priority: 2},
    {Pattern: regexp.MustCompile(`(?i)overwrite\??\s*\[y/N\]`), Response: "n", Priority: 3},  // Safety: don't auto-overwrite
}

type AutoRespondRule struct {
    Pattern  *regexp.Regexp
    Response string
    Priority int  // Lower = higher priority, for documentation
}
```

**Blocker UI:**
```
┌─ BLOCKER DETECTED ─ agent-2 ──────────────────────────────────┐
│                                                               │
│ "Should I use Stripe or Paddle for payment processing?       │
│  Stripe has better docs but Paddle handles EU VAT."          │
│                                                               │
│ Context: Task TAND-006 (Payment integration)                  │
│ CUJ: CUJ-2 (Get Paid)                                        │
│                                                               │
│ Quick responses:                                              │
│ [1] "Use Stripe"                                             │
│ [2] "Use Paddle"                                             │
│ [t] Type custom response                                      │
│ [s] Skip (let agent decide)                                  │
│ [a] Attach to session                                        │
└───────────────────────────────────────────────────────────────┘
```

**Safety rules for auto-respond:**
- Only auto-respond to known-safe prompts (Y/n confirmations)
- Never auto-respond to destructive actions (delete, overwrite, force)
- Log all auto-responses for audit
- Configurable: `[coding_agent] auto_respond = true`

### Detection Algorithm

The detection pipeline runs on each output chunk from the agent:

```go
// Pipeline: completion checked before blocker (completion takes precedence)
//
// NOTE: We track TWO timestamps for idle detection:
//   - lastAnyOutputAt:       Updated on every output chunk
//   - lastNonPromptOutputAt: Updated only on non-prompt output
//
// This prevents the bug where idle detection always sees ~0 duration
// because we just received output (which updated lastActivity).

func detectState(output string, timestamps *OutputTimestamps) DetectionResult {
    now := time.Now()

    // 1. Check completion first (higher precedence)
    if match := matchesAny(output, completionPatterns); match != nil {
        return DetectionResult{Type: Complete, Match: match, Confidence: High}
    }

    // 2. Check idle prompt (agent at prompt, no recent non-prompt activity)
    if matchesAny(output, idlePromptPatterns) != nil {
        // Only check idle if we've seen non-prompt output (prevents false positive at session start)
        if timestamps.ShouldCheckIdle() {
            idleDuration := now.Sub(timestamps.LastNonPromptOutputAt)
            if idleDuration > config.InactivityThreshold {
                return DetectionResult{Type: PossibleComplete, Confidence: Medium}
            }
        }
    } else {
        // Non-prompt output - update the timestamp and flag
        timestamps.LastNonPromptOutputAt = now
        timestamps.HasNonPromptOutput = true
    }

    // 3. Check blocker patterns (lower precedence)
    if match := matchesAny(output, blockerPatterns); match != nil {
        return DetectionResult{Type: Blocked, Match: match, Confidence: Medium}
    }

    // 4. Check auto-respond patterns (ordered slice, not map)
    for _, ar := range autoRespondPatterns {
        if matched := ar.Pattern.FindString(output); matched != "" {
            return DetectionResult{Type: AutoRespond, Response: ar.Response}
        }
    }

    return DetectionResult{Type: Working}
}

type OutputTimestamps struct {
    LastAnyOutputAt       time.Time  // Any output received
    LastNonPromptOutputAt time.Time  // Non-prompt output (for idle detection)
    HasNonPromptOutput    bool       // True after first non-prompt output observed
}

// CRITICAL: Initialize timestamps on session start to prevent false positives
func NewOutputTimestamps() *OutputTimestamps {
    now := time.Now()
    return &OutputTimestamps{
        LastAnyOutputAt:       now,
        LastNonPromptOutputAt: now,
        HasNonPromptOutput:    false,  // Don't evaluate idle until we've seen non-prompt output
    }
}

// Idle detection only triggers after we've observed at least one non-prompt output
func (t *OutputTimestamps) ShouldCheckIdle() bool {
    return t.HasNonPromptOutput
}
```

**Detection precedence order:**
1. Explicit completion signals (`TASK_COMPLETE`)
2. Behavioral completion (idle at prompt + no recent activity)
3. Blocker questions (agent asking for input)
4. Auto-respondable prompts (Y/n confirmations)
5. Default: still working

**Debouncing with Timer-Based Confirmation:**

The debouncer uses a timer to confirm state transitions, handling both continuous output and "final output then silence" scenarios:

```go
type Debouncer struct {
    lastState          DetectionResult
    stateStart         time.Time
    minStateDuration   time.Duration  // default: 2s
    confirmationTimer  *time.Timer
    onConfirm          func(DetectionResult)

    // Single-confirm guard: prevent duplicate onConfirm calls
    lastConfirmedGen   uint64        // Generation of last confirmed state
    currentGen         uint64        // Increments on each state change
    mu                 sync.Mutex
}

func (d *Debouncer) observe(newState DetectionResult) {
    d.mu.Lock()
    defer d.mu.Unlock()

    now := time.Now()

    if newState.Type != d.lastState.Type {
        // State changed - reset timer and increment generation
        d.lastState = newState
        d.stateStart = now
        d.currentGen++
        gen := d.currentGen

        // Cancel any pending confirmation
        if d.confirmationTimer != nil {
            d.confirmationTimer.Stop()
        }

        // Start timer for confirmation (handles "output then silence")
        d.confirmationTimer = time.AfterFunc(d.minStateDuration, func() {
            d.confirmWithGuard(gen, newState)
        })
        return
    }

    // Same state detected again after duration - confirm immediately
    if time.Since(d.stateStart) >= d.minStateDuration {
        d.confirmWithGuard(d.currentGen, d.lastState)
    }
}

// confirmWithGuard prevents duplicate confirmations using generation tracking
func (d *Debouncer) confirmWithGuard(gen uint64, state DetectionResult) {
    d.mu.Lock()
    if gen <= d.lastConfirmedGen {
        d.mu.Unlock()
        return  // Already confirmed this or newer state
    }
    d.lastConfirmedGen = gen
    if d.confirmationTimer != nil {
        d.confirmationTimer.Stop()
    }
    d.mu.Unlock()

    d.onConfirm(state)
}
```

**Key fix:** Timer-based confirmation ensures that if an agent outputs `TASK_COMPLETE` and then stops producing output, the transition still fires after `minStateDuration` (2s default). Without this timer, the debouncer would wait forever for a second chunk.

**Why debouncing matters:**
- Prevents false positives from partial output (e.g., "?" in middle of log line)
- Timer ensures transitions fire even with "output then silence" pattern
- Configurable: `[coding_agent] detection_debounce = "2s"`

### Safe Response Injection

When auto-responding or user provides response to blocker:

```go
func injectResponse(session *TmuxSession, response string) error {
    // 1. Validate response (no shell injection)
    if containsShellMeta(response) {
        return ErrUnsafeResponse
    }

    // 2. Log for audit
    session.LogEvent(ResponseInjected, response)

    // 3. Send via tmux send-keys (safer than direct write)
    return exec.Command("tmux", "send-keys", "-t", session.Name, response, "Enter").Run()
}

func containsShellMeta(s string) bool {
    // Block: ; | & $ ` \ newlines
    return strings.ContainsAny(s, ";|&$`\\\n\r")
}
```

---

## Strategic Guardrails

Tandemonium's unique competitive advantage: connecting WHY you're building to WHAT agents build.

### The Problem This Solves

Other multi-agent tools (Claude Squad, Clark, Cursor) optimize for "run more agents faster."

Tandemonium optimizes for "run the RIGHT agents on the RIGHT tasks."

Without strategic guardrails:
- Agents build features nobody asked for
- Work doesn't connect to user value
- MVP scope creeps invisibly
- You ship fast but ship wrong

### Three-Layer Protection

```
┌─────────────────────────────────────────────────────────────────┐
│ Layer 1: PLANNING                                                │
│ Vision → Users → Journeys → Features → MVP                      │
│ Establishes: What matters, what doesn't, why                    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Layer 2: CUJ TRACKING                                            │
│ Every task maps to a Critical User Journey                       │
│ Progress visualized: "CUJ-1: 2/3 features complete"             │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ Layer 3: DRIFT DETECTION                                         │
│ Real-time alerts when work goes outside boundaries               │
│ Review-time verification of strategic alignment                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Drift Detection

### What It Detects

- **MVP boundary violations**: Task touches features marked "excluded from MVP"
- **CUJ misalignment**: Task doesn't advance any defined journey
- **Scope creep signals**: Agent adding unrequested features
- **Unplanned dependencies**: Agent pulling in libraries outside spec

### How It Works

**During Execution:**
1. Monitor file changes against expected `files_to_modify` in spec
2. Flag new files not in spec (potential scope creep)
3. Detect imports/dependencies not in original plan

**During Review:**
1. Compare task spec against `mvp.md` boundaries
2. Check if files modified match expected CUJ
3. Surface drift prominently in review UI

### Drift Severity Levels

| Level | Meaning | Action |
|-------|---------|--------|
| ℹ️ INFO | Minor deviation, within spirit of task | Note in review |
| ⚠️ WARN | Outside MVP scope, may be intentional | Require acknowledgment |
| 🚫 BLOCK | Violates explicit exclusion | Require justification |

### Review Integration

```
STRATEGIC ALIGNMENT
───────────────────
✓ Advances CUJ-1: First Invoice
✓ Within MVP scope

⚠ DRIFT DETECTED
  Added: src/utils/currency_formatter.py
  Reason: Currency formatting excluded from MVP (see mvp.md line 42)

  [a]cknowledge (add to MVP)    [r]evert file    [?]explain
```

---

## UI/UX

### Main Menu

```
┌─ TANDEMONIUM ───────────────────────────────────────────────────────────────┐
│ invoice-app    Plan: ✓ Complete    MVP: 3/8 features    2 agents working   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│                         [P] Plan                                            │
│                         [E] Execute                                         │
│                         [S] Status                                          │
│                         [Q] Quit                                            │
│                                                                             │
│  Quick Stats                                                                │
│  ───────────────────────────────────────────────────────────────────────    │
│  Vision: "Zero-effort invoicing for freelancers"                            │
│  MVP Scope: 8 features, 3 complete, 2 in progress                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Fleet View (Execute Mode)

```
┌─ EXECUTE MODE ──────────────────────────────────────────────────────────────┐
│ invoice-app    3/8 MVP features    2 agents working                        │
├────────────────────────────────┬────────────────────────────────────────────┤
│ JOURNEYS                       │ AGENTS                                     │
│                                │                                            │
│ CUJ-1: First Invoice  ████░░░░ │  ● agent-1  WORKING   TAND-004     12m    │
│   └─ 2/3 features              │    └─ Invoice PDF generation              │
│                                │                                            │
│ CUJ-2: Get Paid       ██░░░░░░ │  ◐ agent-2  BLOCKED   TAND-006      8m    │
│   └─ 1/3 features              │    └─ "Stripe or Paddle?"                 │
│                                │                                            │
│ CUJ-3: Track Time     ░░░░░░░░ │  ○ agent-3  IDLE                          │
│   └─ 0/2 features              │                                            │
│                                │                                            │
├────────────────────────────────┴────────────────────────────────────────────┤
│ TASK QUEUE                                                                  │
│ READY: TAND-005 Client management    TAND-008 Time entry                   │
│ REFINING: TAND-007 Payment reminders                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│ [n]ew  [N]quick  [d]erive  [a]ssign  [f]ocus  [u]nblock  [R]eview  [?]help │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Focus View (Single Agent)

```
┌─ agent-1 ─ WORKING ─ TAND-004 ──────────────────────────────────────────────┐
│ CUJ: First Invoice    Feature: PDF rendering    Runtime: 12m               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ 14:22:01  Looking for existing PDF generation code...                       │
│ 14:22:15  Will use weasyprint for PDF generation.                          │
│ 14:22:30  Creating src/invoices/pdf_generator.py                           │
│ 14:25:10  Running tests... 3 passed, 1 failed                              │
│ 14:25:30  Fixing logo path resolution...                                    │
│ ▌                                                                           │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ ACCEPTANCE CRITERIA                             PROGRESS                    │
│ ☑ Generates valid PDF                          ████████░░ 80%              │
│ ☐ Includes company logo                                                     │
│ ☐ All tests passing                                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│ [Enter]attach  [m]essage  [p]ause  [k]ill  [Space]diff  [b]ack             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `j/k` or `↑/↓` | Navigate up/down |
| `Enter` | Select / Attach to tmux |
| `n` | New task (full PM flow) |
| `N` | New task with prompt (quick mode, skip PM) |
| `d` | Derive tasks from plan |
| `s` | **Sync**: commit + push + switch to next agent (like Claude Squad) |
| `c` | **Checkout**: commit WIP + pause session + switch to worktree |
| `p` | Pause/resume session |
| `r` | Reject / restart failed session |
| `k` | Kill session |
| `R` | Enter review queue |
| `Space` | Quick diff preview |
| `/` | Search tasks |
| `Ctrl+K` | Command palette |
| `?` | Help overlay |
| `f` | Toggle focus/fullscreen mode |
| `Tab` | Cycle focus (list ↔ detail ↔ terminal) |
| `,` | Settings |
| `q` | Quit TUI (agents keep running) |
| `Ctrl+Q` | Detach from current view |

### Quit Behavior (Matches Conventions)

| Key | Action |
|-----|--------|
| `q` | Quit TUI, agents keep running in tmux |
| `ctrl-q` | Detach from current agent view (within TUI) |
| `tand stop` | CLI command to kill all agents |

---

## Review Workflow

### Review View

```
┌─ REVIEW ─ TAND-003: Invoice email delivery ─────────────────────────────────┐
│ agent-1    Runtime: 18m    CUJ-1: First Invoice                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ USER STORY                                                                  │
│ As a freelancer, I want to send invoices via email so that clients         │
│ receive professional billing without manual attachment work.                │
│                                                                             │
│ SUMMARY                                                                     │
│ Implemented invoice email delivery with PDF attachment.                     │
│                                                                             │
│ FILES CHANGED                                                               │
│   A src/invoices/email_sender.py              +124 -0                       │
│   A src/templates/invoice_email.html          +45 -0                        │
│   A tests/test_email_sender.py                +67 -0                        │
│                                                                             │
│ TESTS: ✓ 8/8 passing                                                        │
│                                                                             │
│ ACCEPTANCE CRITERIA                                                         │
│ ✓ Sends email with PDF attachment                                           │
│ ✓ Retries on temporary failures                                             │
│ ✓ Email matches brand aesthetic                                             │
│                                                                             │
│ STRATEGIC ALIGNMENT                                                         │
│ ✓ Advances CUJ-1: First Invoice                                             │
│ ✓ Within MVP scope                                                          │
│ ✓ No code drift detected                                                    │
│ ✓ User story unchanged                                                      │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ [d]iff view    [a]pprove    [f]eedback    [r]eject    [e]dit story         │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Story drift warning (when story was modified after work began):**
```
┌─ REVIEW ─ TAND-005: Payment reminders ──────────────────────────────────────┐
│ agent-2    Runtime: 25m    CUJ-2: Get Paid                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ ⚠ STORY DRIFT DETECTED                                                      │
│ The user story was modified after the agent started working.                │
│                                                                             │
│ Original (when work began):                                                 │
│   "As a freelancer, I want payment reminders so that I get paid faster."   │
│                                                                             │
│ Current:                                                                    │
│   "As a freelancer, I want gentle payment reminders at 7, 14, 30 days      │
│    so that I maintain good client relationships while getting paid."       │
│                                                                             │
│ The agent worked against the ORIGINAL story. Review carefully.              │
│                                                                             │
│ [a]ccept anyway    [r]eject + re-queue with new story    [v]iew diff       │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Batch Review Queue

- List of completed tasks pending review
- "Approve all" for trusted tasks
- Individual review for scrutiny
- Strategic alignment summary per task

### Approval Semantics

**What happens when you press `[a]pprove`:**

```
approve → merge worktree → cleanup → mark done
   │            │             │           │
   ▼            ▼             ▼           ▼
 Task       git merge      remove      status =
validated   to target     worktree      "done"
            branch        + branch
```

**Step-by-step:**

1. **Validate** - Ensure acceptance criteria are met (or acknowledged as partial)
2. **Merge** - `git merge --no-ff feature/TAND-XXX` into target branch (default: current branch)
3. **Cleanup** - Remove worktree and feature branch
4. **Mark done** - Task status → `done`, record merge commit SHA

**Configuration options:**
```toml
[review]
# Target branch for merges (default: current branch when task started)
target_branch = ""           # "" = use branch from task creation time

# Merge strategy
merge_strategy = "merge"     # "merge" | "squash" | "rebase"
no_ff = true                 # Always create merge commit

# Cleanup behavior
auto_cleanup = true          # Remove worktree after merge
delete_branch = true         # Delete feature branch after merge

# Push behavior (requires gh CLI)
auto_push = false            # Push after merge
create_pr = false            # Create PR instead of direct merge
```

**Rejection flow:**
```
reject → feedback → reassign
   │         │          │
   ▼         ▼          ▼
 Task    Store      status =
flagged  feedback   "rejected"
         for next   → "ready"
         attempt    (queued)
```

**What `[r]eject` does:**
1. Prompts for feedback (required)
2. Stores feedback in task spec
3. Status → `rejected` → `ready` (queued for reassignment)
4. Worktree preserved for next agent to continue
5. Next agent receives original spec + rejection feedback

---

## Data Model

### Storage Location

```
project/
├── .tandemonium/
│   ├── config.toml          # Project settings (committed)
│   ├── plan/                # Strategic planning docs (committed)
│   │   ├── vision.md
│   │   ├── users.md
│   │   ├── journeys.md
│   │   ├── features.md
│   │   └── mvp.md
│   ├── specs/               # Task specs (committed)
│   │   └── TAND-042.yaml
│   ├── state.db             # SQLite state database (gitignored)
│   ├── sessions/            # tmux session logs (gitignored)
│   └── worktrees/           # Git worktree directories (gitignored)
└── .gitignore               # Auto-updated to ignore runtime data
```

### Git Tracking

**Committed:**
- `.tandemonium/config.toml` - project settings
- `.tandemonium/plan/` - strategic planning documents
- `.tandemonium/specs/` - completed specs as documentation

**Gitignored:**
- `state.db`, `sessions/`, `worktrees/`

### Task Model

```go
type Task struct {
    ID            string
    RawInput      string
    Status        TaskStatus
    Priority      int
    CreatedAt     time.Time
    UpdatedAt     time.Time

    // Strategic context (all nullable for no-plan mode)
    CUJID         *string       // "CUJ-1" (nil if untracked)
    FeatureID     *string       // "pdf-render" (nil if untracked)
    MVPStatus     MVPStatus     // included | excluded | unknown

    // User story
    UserStory     *UserStory

    // Refinement
    RefinedSpec   *string    // Path to TAND-XXX.yaml

    // Execution
    AssignedAgent *string
    BranchName    *string
    StartedAt     *time.Time
    CompletedAt   *time.Time

    // Results
    FilesChanged  []string
    TestsPassed   *bool
    DriftDetected *bool
    StoryDrift    *bool      // True if story changed after work began
}

type UserStory struct {
    Text        string
    Hash        string     // SHA256 prefix for change detection
    CreatedAt   time.Time
    ModifiedAt  *time.Time // Non-nil if user edited
    HashAtStart *string    // Hash when task moved to "working" (for drift detection)
}

// Story drift detection
func (t *Task) CheckStoryDrift() bool {
    if t.UserStory == nil || t.UserStory.HashAtStart == nil {
        return false
    }
    return t.UserStory.Hash != *t.UserStory.HashAtStart
}

// Called when task transitions to "working"
func (t *Task) SnapshotStoryHash() {
    if t.UserStory != nil {
        t.UserStory.HashAtStart = &t.UserStory.Hash
    }
}

// Called when user edits the story
func (t *Task) UpdateStory(newText string) {
    now := time.Now()
    hash := sha256Short(newText)

    if t.UserStory == nil {
        t.UserStory = &UserStory{}
    }
    t.UserStory.Text = newText
    t.UserStory.Hash = hash
    t.UserStory.ModifiedAt = &now

    // Flag drift if work already started
    if t.Status == TaskWorking || t.Status == TaskReview {
        drift := t.CheckStoryDrift()
        t.StoryDrift = &drift
    }
}

type TaskStatus string
const (
    TaskDraft    TaskStatus = "draft"
    TaskRefining TaskStatus = "refining"
    TaskReady    TaskStatus = "ready"
    TaskAssigned TaskStatus = "assigned"
    TaskWorking  TaskStatus = "working"
    TaskBlocked  TaskStatus = "blocked"
    TaskPaused   TaskStatus = "paused"
    TaskReview   TaskStatus = "review"
    TaskDone     TaskStatus = "done"
    TaskRejected TaskStatus = "rejected"
    TaskFailed   TaskStatus = "failed"    // Error state requiring intervention
)

// MVPStatus: tri-state for planning mode compatibility
type MVPStatus string
const (
    MVPIncluded MVPStatus = "included"  // Feature is in MVP scope
    MVPExcluded MVPStatus = "excluded"  // Feature explicitly excluded from MVP
    MVPUnknown  MVPStatus = "unknown"   // No plan or feature not mapped
)

type SessionState string
const (
    SessionStarting SessionState = "starting"
    SessionWorking  SessionState = "working"
    SessionIdle     SessionState = "idle"
    SessionBlocked  SessionState = "blocked"
    SessionPaused   SessionState = "paused"
    SessionComplete SessionState = "complete"
    SessionFailed   SessionState = "failed"
)
```

### CUJ Progress Model

```go
type CUJProgress struct {
    ID                string
    Name              string
    TotalFeatures     int
    CompletedFeatures int
    Tasks             []string  // Task IDs in this CUJ
}
```

---

## Git Integration

### Why Worktrees Matter

**The isolation problem:** Multiple AI agents editing the same files = merge conflicts, race conditions, lost work.

**Tandemonium's solution:** Each agent works in its own git worktree. True filesystem isolation, not just branch isolation.

```
main repo/
├── src/                    # Your working directory
└── .tandemonium/
    └── worktrees/
        ├── TAND-001/       # Agent 1's isolated copy
        │   └── src/
        ├── TAND-002/       # Agent 2's isolated copy
        │   └── src/
        └── TAND-003/       # Agent 3's isolated copy
            └── src/
```

**Benefits:**
- Agents can't step on each other's work
- Review diffs are clean (just one task's changes)
- Easy to discard failed work (delete worktree)
- Human can work in main while agents work in worktrees

### Worktree Lifecycle

```
CREATE → WORK → CHECKOUT → REVIEW → MERGE → CLEANUP
   │       │        │         │        │        │
   ▼       ▼        ▼         ▼        ▼        ▼
worktree  agent   pause &   human    merge   remove
 add      runs    switch   reviews   to main  worktree
```

1. **Create**: `git worktree add .tandemonium/worktrees/<task-id> -b feature/<task-id>`
2. **Work**: Agent operates in isolated worktree
3. **Checkout** (`c`): Commit WIP, pause agent, switch to worktree for inspection
4. **Review**: User reviews diff against main
5. **Sync** (`s`): Commit and push (requires `gh` CLI)
6. **Merge**: `git merge` into main (or user's target branch)
7. **Cleanup**: `git worktree remove` + delete branch

### Safety Rails

- **Dirty repo warning**: Warn if uncommitted changes in main
- **Preflight checks**: Disk space, branch conflicts, path escape prevention
- **Default branch detection**: Don't assume `main`, detect from git config
- **Worktree health**: Detect orphaned worktrees, offer cleanup

---

## Failure Recovery

### Design Principle

**Crash-only design**: Assume the TUI can die at any moment. All state must be recoverable.

### Recovery Scenarios

#### 1. TUI Crashes Mid-Session

**What happens:**
- tmux sessions continue running (agents keep working)
- SQLite WAL mode ensures database consistency
- No work is lost

**Recovery:**
```bash
tand                    # Auto-detects existing sessions
# Or explicitly:
tand recover            # Scan for orphaned sessions, rebuild state
```

**Startup sequence:**
1. Check for existing `.tandemonium/state.db`
2. Scan for tmux sessions matching `tand-*` pattern
3. Reconcile: sessions in tmux but not in DB → prompt to adopt or kill
4. Resume normal operation

#### 2. Machine Crashes / Power Loss

**What happens:**
- tmux sessions die (no tmux-resurrect by default)
- SQLite WAL may have uncommitted transactions
- Worktrees may have uncommitted changes

**Recovery:**
```bash
tand recover --full     # Full recovery mode
```

**Recovery steps:**
1. SQLite WAL checkpoint (recover any pending transactions)
2. Scan worktrees for uncommitted changes
3. For each worktree with changes:
   - Show diff summary
   - Offer: `[c]ommit WIP`, `[d]iscard`, `[k]eep for manual review`
4. Mark interrupted tasks as `paused` (not `failed`)
5. Offer to restart paused tasks

#### 3. SQLite Database Corruption

**Prevention:**
- WAL mode with `synchronous=NORMAL`
- Automatic backups before schema migrations
- Daily backup to `.tandemonium/backups/state-YYYY-MM-DD.db`

**Detection:**
```bash
tand doctor             # Run integrity checks
```

**Recovery:**
```bash
tand recover --from-backup    # Restore from latest backup
tand recover --rebuild        # Rebuild DB from tmux sessions + specs
```

**Rebuild from specs:**
- Task specs in `.tandemonium/specs/*.yaml` are source of truth
- Can reconstruct task list from spec files
- Session logs can reconstruct execution history

#### 4. Orphaned tmux Sessions

**Scenario:** Sessions exist but TUI doesn't know about them.

**Detection (on startup):**
```
┌─ ORPHANED SESSIONS DETECTED ──────────────────────────────────┐
│                                                               │
│ Found 2 tmux sessions not in database:                        │
│                                                               │
│   tand-TAND-007  (created 2h ago, still running)             │
│   tand-TAND-008  (created 1h ago, idle)                      │
│                                                               │
│ [a]dopt all    [r]eview each    [k]ill all                   │
└───────────────────────────────────────────────────────────────┘
```

#### 5. Orphaned Worktrees

**Scenario:** Git worktrees exist but tasks are done/deleted.

**Detection:**
```bash
tand cleanup            # Interactive cleanup
tand cleanup --dry-run  # Show what would be cleaned
```

**Cleanup prompt:**
```
┌─ ORPHANED WORKTREES ──────────────────────────────────────────┐
│                                                               │
│ Found 3 worktrees with no active task:                        │
│                                                               │
│   .tandemonium/worktrees/TAND-003  (task: done, 3 days ago)  │
│   .tandemonium/worktrees/TAND-005  (task: deleted)           │
│   .tandemonium/worktrees/TAND-009  (no matching task)        │
│                                                               │
│ Space used: 245 MB                                            │
│                                                               │
│ [c]lean all    [r]eview each    [s]kip                       │
└───────────────────────────────────────────────────────────────┘
```

### State Integrity Checks

Run automatically on startup, manually with `tand doctor`:

```
$ tand doctor

Checking database integrity... ✓
Checking tmux sessions... ✓
Checking worktrees... ⚠ 1 orphaned
Checking specs... ✓
Checking git state... ✓

Issues found: 1
Run `tand cleanup` to resolve.
```

### Backup Strategy

```toml
[recovery]
auto_backup = true
backup_interval = "24h"
backup_retention = 7           # Keep 7 daily backups
backup_location = ".tandemonium/backups/"
```

**What's backed up:**
- `state.db` (full SQLite database)
- NOT: worktrees (too large, git handles this)
- NOT: session logs (append-only, rarely corrupted)

---

## Configuration

### Config Layers

1. **User config**: `~/.config/tandemonium/config.toml`
2. **Project config**: `.tandemonium/config.toml`
3. **Environment variables**: Override any setting
4. **CLI flags**: Override for single session

### Key Settings

```toml
[general]
max_agents = 4
log_lines = 1000
branch_strategy = "feature"  # or "trunk"

[planning]
skip = false                 # Skip planning entirely
stages = ["vision", "users", "journeys", "features", "mvp"]

[test]
command = ""                 # Empty = auto-detect
timeout = "5m"

[pm_agent]
model = "claude-sonnet-4-20250514"
web_access = true

[coding_agent]
type = "claude-code"
auto_accept = false          # -y flag equivalent
health_check_interval = 30   # seconds, 0 = disabled
restart_on_failure = false   # auto-restart crashed sessions

[ui]
theme = "dark"
show_timestamps = true

[git]
worktree_prefix = "feature"
auto_commit = false

[shortcuts]
quit = "q"
focus = "f"
```

### CLI Surface

**Binary name:** `tandemonium` (installed). `tand` is a convenience alias provided by installers or user shell aliasing; it is not guaranteed unless explicitly installed.

```bash
# Both work identically:
tandemonium                  # Full name
tand                         # Alias (recommended for daily use)
```

### CLI Commands

```bash
tand                         # Launch TUI (main menu)
tand init                    # Initialize .tandemonium/ in current directory
tand plan                    # Jump to planning mode
tand execute                 # Jump to execute mode
tand status                  # Quick status check (no TUI)
tand stop                    # Kill all agents
tand recover                 # Recover from crash (see Failure Recovery)
tand doctor                  # Run integrity checks
tand cleanup                 # Clean orphaned worktrees/sessions
tand export                  # Export state to JSON
tand import <file>           # Import state from JSON
```

**`tand init` behavior:**
- Creates `.tandemonium/` directory structure
- Initializes empty `state.db` (SQLite)
- Creates `config.toml` with defaults
- Adds `.tandemonium/state.db`, `sessions/`, `worktrees/` to `.gitignore`
- Optionally starts planning flow (prompted)

```bash
$ tand init
Initialized Tandemonium in /path/to/project/.tandemonium/

Would you like to start planning? [Y/n]
```

**First-run behavior:** Running `tand` in an uninitialized directory auto-runs `tand init`.

```bash
# Task creation shortcuts
tand "add rate limiting"     # Create task + start PM refinement immediately
tand -q "add rate limiting"  # Quick mode: skip PM, assign directly to agent
tand -y                      # Auto-accept mode (for all agent prompts)
tand --sessions 2            # Limit concurrent agents
tand --prompt <template>     # Use prompt template (see below)
tand --skip-planning         # Skip planning flow on first run
```

### Quick Mode (`-q` / `N` key)

Quick mode skips PM refinement entirely:

```
Normal flow:   prompt → PM refinement → spec → agent
Quick mode:    prompt → agent (with raw prompt as spec)
```

**When to use:**
- Small, well-defined tasks ("fix typo in README")
- Experienced users who write detailed prompts
- Time-sensitive fixes

**Behavior:**
- `tand -q "fix login bug"` — creates task, assigns to next available agent
- `N` key in TUI — same, prompts for task description
- Task status goes directly from `draft` → `assigned` (skipping `refining` → `ready`)

**Quick Mode Story Handling:**

Quick mode tasks have **no user story** (since PM refinement is skipped):

```go
// Quick mode task creation
func createQuickTask(rawInput string) *Task {
    return &Task{
        RawInput:  rawInput,
        Status:    TaskAssigned,  // Skip refining → ready
        UserStory: nil,           // No story in quick mode
        // ...
    }
}
```

**UI implications:**
- Review UI shows "No user story (quick task)" instead of story section
- Story drift detection is skipped (nothing to drift from)
- `[e]dit story` option in review adds a story retroactively (optional)

**Why not auto-generate a story?** Quick mode is for speed. Auto-generating a story would require an API call, defeating the purpose. Users who want a story should use normal mode.

**Quick Mode Artifact Generation:**

Quick mode still generates a **minimal YAML spec** for consistency with recovery and audit:

```yaml
# .tandemonium/specs/TAND-007.yaml (quick mode)
---
id: "TAND-007"
title: "Fix login timeout bug"      # First line of raw input
created_at: "2025-01-10T15:00:00Z"
quick_mode: true                    # Marker for quick task

strategic_context:
  cuj_id: null                      # Untracked
  cuj_name: null
  feature_id: null
  mvp_status: "unknown"

user_story: null                    # No story in quick mode

summary: |
  Fix login timeout bug

  (Quick task - no PM refinement performed)

requirements:
  - "Raw input: fix login timeout bug"

acceptance_criteria: []             # Empty - user's judgment

files_to_modify: []                 # Unknown - agent will discover

complexity: "unknown"
estimated_minutes: null
priority: 2                         # Default priority
```

**Quick mode commit timing:**
- Spec YAML is created and committed immediately when task is created
- This ensures recovery can rebuild from specs even for quick tasks
- Commit message: `chore(tandemonium): add quick task TAND-007`

**Recovery guarantees:**
- Quick tasks ARE included in "rebuild from specs" recovery
- Missing fields (AC, files_to_modify) are treated as "unknown/any"
- Quick mode marker (`quick_mode: true`) helps UI display appropriately

### Prompt Templates

Pre-defined task templates for common scenarios:

```bash
tand --prompt bugfix "fix login timeout"      # Bug fix template
tand --prompt feature "add dark mode"         # Feature template
tand --prompt refactor "extract auth service" # Refactor template
tand --prompt test "add coverage for api/"    # Test template
```

Templates inject structure into PM refinement:

```toml
# .tandemonium/templates/bugfix.toml
[template]
name = "bugfix"
description = "Bug fix task"

[pm_hints]
focus = ["reproduction steps", "root cause", "regression test"]
skip_questions = ["visual design", "user flow"]
```

---

## MVP Scope

### Timeline: 12 Weeks

**Note:** Original estimate was 10 weeks. Extended to 12 based on complexity assessment of tmux integration + stream parsing + blocker detection.

### In Scope (Weeks 1-12)

**Planning (Weeks 1-2):**
- `tand init` with planning prompt
- Vision stage (guided conversation)
- MVP stage (scoping conversation)
- Plan documents saved to `.tandemonium/plan/`
- Optional: Users, Journeys, Features stages

**Task Management (Weeks 3-4):**
- Manual task creation
- Task derivation from plan
- PM-mode refinement with strategic context
- Task queue with CUJ grouping
- Spec YAML schema + validation

**Execution (Weeks 5-8):** ← Extended from 3 to 4 weeks
- Single agent execution (Claude Code CLI)
- Multi-agent (up to 4)
- tmux session management with pipe-pane
- Blocker detection patterns and unblocking UI
- Completion detection with fallbacks
- Health monitoring with auto-restart

**Review (Week 9):**
- Code review with diff view
- Strategic alignment check
- Drift detection
- Batch review queue

**Polish (Weeks 10-12):**
- All keyboard shortcuts (`-y`, `s`, `c`, `Space`, `N`, `-q`)
- Command palette
- Settings UI
- Failure recovery (`tand recover`, `tand doctor`, `tand cleanup`)
- Documentation

### Out of Scope (Post-MVP)

- Daemon mode (background execution)
- Event sourcing (full audit replay)
- Desktop notifications
- Multi-agent path locking
- MCP integration
- RepoMap / tree-sitter
- PR creation automation
- Multiple coding agent types
- Team features

---

## Competitive Comparison

### Market Landscape

| Tool | Focus | Strength | Weakness |
|------|-------|----------|----------|
| **Claude Squad** | Run agents fast | UX polish, tmux integration | No strategy layer |
| **Clark** | Agent health | Monitoring, session states | No worktrees, limited agents |
| **Cursor 2.0** | IDE-native | 8 agents, background mode | Proprietary, IDE-only |
| **Amp** | Multi-editor | Subagents, broad support | Enterprise focus |
| **Aider** | Single agent | Voice, 100+ languages | No orchestration |
| **Tmux Orchestrator** | Self-scheduling | PM→Engineer hierarchy | Experimental |

### Positioning

**Claude Squad et al:** "Run more agents faster"
**Tandemonium:** "Run the RIGHT agents on the RIGHT tasks"

### Feature Matrix

| Feature | Claude Squad | Clark | Cursor | Tandemonium |
|---------|--------------|-------|--------|-------------|
| **STRATEGY LAYER** |
| Strategic Planning | ❌ | ❌ | ❌ | ✅ **Unique** |
| CUJ Tracking | ❌ | ❌ | ❌ | ✅ **Unique** |
| Drift Detection | ❌ | ❌ | ❌ | ✅ **Unique** |
| PM Refinement | ❌ | ❌ | ❌ | ✅ **Unique** |
| Canonical Specs | ❌ | ❌ | ❌ | ✅ **Unique** |
| **EXECUTION** |
| Multi-agent | ✅ ∞ | ✅ 4 | ✅ 8 | ✅ 4 |
| Worktree isolation | ✅ | ❌ | ❌ | ✅ |
| Session persistence | ✅ tmux | ❌ | ✅ bg | ✅ tmux |
| Auto-accept (`-y`) | ✅ | ❌ | ✅ | ✅ |
| Health monitoring | ❌ | ✅ | ❌ | ✅ |
| **UX** |
| Sync shortcut (`s`) | ✅ | ❌ | ❌ | ✅ |
| Checkout shortcut (`c`) | ✅ | ❌ | ❌ | ✅ |
| Quick task (`N`) | ✅ | ❌ | ❌ | ✅ |
| Prompt templates | ✅ | ❌ | ❌ | ✅ |
| Diff preview | ✅ Tab | ✅ | ✅ | ✅ Space |
| **REVIEW** |
| Batch review queue | ❌ | ❌ | ❌ | ✅ **Unique** |
| Strategic alignment | ❌ | ❌ | ❌ | ✅ **Unique** |
| **META** |
| Open source | ✅ AGPL | ✅ MIT | ❌ | ✅ MIT |
| Terminal-native | ✅ | ✅ | ❌ | ✅ |
| Language | Go | Rust | ? | Go |

---

## Success Criteria

1. **Planning Value:** Users who complete planning report clearer direction
2. **Refinement Quality:** PM-refined tasks have 50%+ fewer back-and-forth cycles
3. **Blocker Visibility:** Surface blocked questions within 5 seconds
4. **Review Speed:** Review and approve completed task in <2 minutes
5. **Strategic Alignment:** 90%+ of completed tasks align with defined CUJs
6. **Persistence:** Survive TUI restarts, resume sessions, never lose state

---

## Non-Goals

- Not a terminal emulator (use tmux attach)
- Not a code editor (use your IDE)
- Not a CI/CD system (integrates with, doesn't replace)
- Not a project management tool (tasks are execution items)
- Not multi-user (single TUI for MVP)
- Not a full product strategy tool (guided planning, not comprehensive PM)

---

## Open Questions (Resolved)

| Question | Decision |
|----------|----------|
| Cost tracking for CLI agents | Skip - show "unknown", track PM only |
| Multi-repo support | No - one project per instance |
| Daemon architecture | No daemon for MVP - TUI only |
| Event sourcing | No for MVP - simple SQLite state |
| PM scaffolding | No - PM creates spec only |
| File conflict handling | Task-level hints + merge-time resolution |
| Completion detection | Hybrid: magic string + behavioral inference |
| Planning required? | Optional - can skip entirely or skip stages |
| Tech stack | Go + Bubble Tea + tmux (not Rust) |

---

## References

### Competitive Tools
- [Claude Squad](https://github.com/smtg-ai/claude-squad) - tmux-based multi-agent orchestration (Go, AGPL-3.0)
- [Clark](https://github.com/brianirish/clark) - Health monitoring and session management (Rust, MIT)
- [Tmux Orchestrator](https://github.com/nnmm/tmux-orchestrator) - Self-scheduling agent hierarchy
- [TmuxAI](https://github.com/ruvnet/tmuxai) - Context-aware terminal assistant
- [Cursor](https://cursor.com) - IDE-native multi-agent (proprietary)
- [Amp](https://ampcode.com) - Sourcegraph's multi-editor agent platform
- [Aider](https://github.com/paul-gauthier/aider) - Single-agent pair programming (Apache-2.0)

### Patterns & Libraries
- [MCP Agent Mail](https://github.com/Dicklesworthstone/mcp_agent_mail) - File reservation patterns
- [Ralph-Wiggum](https://github.com/anthropics/claude-code/tree/main/plugins/ralph-wiggum) - Completion detection patterns
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Go TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Go TUI styling

---

*Tandemonium: Orchestrated chaos. From vision to launch.*
