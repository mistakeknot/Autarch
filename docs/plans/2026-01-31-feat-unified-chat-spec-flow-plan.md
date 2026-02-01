---
title: "feat: Unified Chat-Driven Spec Flow"
type: feat
date: 2026-01-31
bead: Autarch-96s
deepened: 2026-01-31
reviewed: 2026-01-31
---

# Unified Chat-Driven Spec Flow

## Overview

Converge Kickoff Interview (chat-based, 3 phases, evidence grounding) and Arbiter Sprint (8 phases, consistency checks, confidence scoring) into a single chat-driven surface. The user never leaves the chat panel; the Arbiter engine runs underneath.

## Problem Statement

Two overlapping systems exist for spec creation:

| | Kickoff Interview | Arbiter Sprint |
|---|---|---|
| **UX** | Chat-based, conversational | Propose-accept, keyboard-driven |
| **Phases** | 3 (Vision, Problem, Users) | 8 (Vision → Acceptance Criteria) |
| **Strengths** | Evidence grounding, quality scores, open question resolution | Consistency checks, confidence scoring, thinking shapes, research |
| **Weakness** | Covers only 3/8 phases | No chat, no evidence, no codebase scanning |

The handoff between them is lossy: `buildSignoffAnswers()` in `kickoff.go` flattens rich `PhaseArtifacts` (evidence items, quality scores, resolved questions) to `map[string]string`. The Arbiter starts blind.

## Proposed Solution

**One flow, one surface.** The chat panel drives all 8 phases. The Arbiter engine runs underneath — everything is mediated through chat messages and the doc panel.

### Architecture

```
┌─────────────────────────────────────────────────────┐
│  Unified Shell (pkg/tui/ShellLayout)                │
│  ┌──────────┬──────────────────┬───────────────────┐│
│  │ Sidebar  │    Doc Panel     │   Chat Panel      ││
│  │          │                  │                   ││
│  │ ● Vision │  [Draft content] │  Agent: Here's    ││
│  │ ○ Problem│  [Evidence ▸]    │  my draft for...  ││
│  │ ○ Users  │                  │                   ││
│  │ ○ Feat…  │                  │  You: Looks good  ││
│  │ ○ Req…   │                  │  but narrow the   ││
│  │ ○ Scope  │                  │  scope to...      ││
│  │ ○ CUJs   │                  │                   ││
│  │ ○ AC     │                  │  [composer]       ││
│  └──────────┴──────────────────┴───────────────────┘│
└─────────────────────────────────────────────────────┘
```

**Data flow:**

```
Chat input → Orchestrator.ProcessChatMessage()
                  ├─ Generator (thinking shapes)
                  ├─ Consistency engine
                  ├─ Confidence calculator
                  └─ Research provider
                        ↓
               SectionDraft update (via Bubble Tea Cmd)
                        ↓
                Doc Panel renders draft + metadata
                Chat Panel shows agent explanation
```

---

## Technical Approach

### Phase 1: Lossless Artifact Handoff

**Goal:** Carry full Kickoff artifacts into Arbiter without loss.

**Key decision:** Reuse existing `PhaseArtifacts` type from `internal/tui/messages.go` (contains `VisionArtifact`, `ProblemArtifact`, `UsersArtifact` with evidence, quality, resolved questions). Do NOT create a new `ScanContext` type.

**Import cycle prevention:** `arbiter` cannot import `tui`. Create adapter package `internal/gurgeh/arbiter/scan/` with minimal types:

```go
// internal/gurgeh/arbiter/scan/artifacts.go
package scan

type Artifacts struct {
    Vision  *PhaseData
    Problem *PhaseData
    Users   *PhaseData
}

type PhaseData struct {
    Summary           string
    Evidence          []EvidenceItem
    ResolvedQuestions []ResolvedQuestion
    Quality           QualityScores
}

type EvidenceItem struct {
    FilePath   string
    Quote      string  // sanitize before prompt injection (wrap in <evidence> delimiters)
    Confidence float64
}
```

The TUI layer converts `tui.PhaseArtifacts` → `scan.Artifacts` at the boundary.

**QualityScores → ConfidenceScore mapping:** These are different taxonomies. Explicit mapping:
- `QualityScores.Grounding` → `ConfidenceScore.Research` (evidence backing)
- `QualityScores.Clarity` → `ConfidenceScore.Specificity` (precision of language)
- `QualityScores.Completeness` → `ConfidenceScore.Completeness` (direct)
- `QualityScores.Consistency` → `ConfidenceScore.Consistency` (direct)

**Files to modify:**

- [ ] `internal/gurgeh/arbiter/scan/artifacts.go` — New adapter package with types above
- [ ] `internal/gurgeh/arbiter/types.go` — Add `ScanArtifacts *scan.Artifacts` field to `SprintState`
- [ ] `internal/gurgeh/arbiter/orchestrator.go` — Accept `*scan.Artifacts` in `Start()`, seed first 3 sections
- [ ] `internal/gurgeh/arbiter/generator.go` — Inject evidence + resolved questions into draft prompt context; wrap evidence quotes in `<evidence>` XML delimiters for prompt injection safety
- [ ] `internal/gurgeh/arbiter/confidence/calculator.go` — Map `QualityScores` dimensions per mapping above
- [ ] `internal/tui/views/kickoff.go` — Replace `buildSignoffAnswers()` with conversion to `scan.Artifacts`

---

### Phase 2: Orchestrator Chat Extension

**Goal:** Extend Orchestrator to accept chat messages directly. No intermediate controller/coordinator layer — the view owns the Orchestrator directly, matching how `kickoff.go` works.

**Files to modify:**

- [x] `internal/gurgeh/arbiter/orchestrator.go` — Add:
  ```go
  // ProcessChatMessage handles a user message in the context of the current phase.
  // The caller MUST cancel ctx when navigating away to prevent goroutine leaks.
  func (o *Orchestrator) ProcessChatMessage(ctx context.Context, msg string) <-chan string

  // AcceptDraft accepts the current phase draft, runs consistency check, advances.
  func (o *Orchestrator) AcceptDraft() error

  // ReviseDraft requests revision with user feedback.
  func (o *Orchestrator) ReviseDraft(feedback string) error
  ```
- [x] `internal/gurgeh/arbiter/orchestrator.go` — Add `sync.Mutex` to protect state (needed for future concurrent callers; Bubble Tea is single-threaded but the Orchestrator should be safe by design)
- [x] `internal/gurgeh/arbiter/orchestrator.go` — `ProcessChatMessage` producer must `select` on `ctx.Done()` to stop when caller navigates away
- [x] `internal/gurgeh/arbiter/orchestrator.go` — Basic retry with backoff (3 attempts) inside `ProcessChatMessage`
- [x] `internal/gurgeh/arbiter/orchestrator_test.go` — Unit tests for message→operation mapping

**Interaction model:**

| User action | Orchestrator operation |
|---|---|
| Types message | `ProcessChatMessage` → agent refines draft → streams response |
| Presses `a` | `AcceptDraft` → consistency check → advance or show conflicts |
| Presses `e` | Focus composer with "Edit:" prefix → `ReviseDraft` on send |
| Ctrl+Left/Right | Navigate to prev/next phase (completed only) |

**Streaming pattern in the view:**

```go
func waitForResponse(ch <-chan string) tea.Cmd {
    return func() tea.Msg {
        line, ok := <-ch
        if !ok {
            return streamDoneMsg{}
        }
        return streamLineMsg(line)
    }
}
```

On receiving `streamLineMsg`, append to chat and issue another `waitForResponse`. On `streamDoneMsg`, stop. On navigation away, cancel the context.

---

### Phase 3: Unified Sprint View

**Goal:** Single Bubble Tea view replacing both `internal/gurgeh/tui/sprint.go` and the arbiter portions of `internal/tui/views/kickoff.go`.

**Files to create:**

#### `internal/tui/views/sprint_view.go`

```go
type SprintView struct {
    orch       *arbiter.Orchestrator  // direct ownership
    chatPanel  *pkgtui.ChatPanel
    docPanel   *DocPanel              // draft content + collapsible evidence toggle
    sidebar    *PhaseSidebar
    shell      *pkgtui.ShellLayout
    cancelChat context.CancelFunc     // cancel streaming on navigation
}
```

**Doc panel:** Draft content always visible. Single "Details" toggle for evidence/quality/consistency. Don't build 6 simultaneous renderers — add sections when users ask for them.

**Sidebar states:** 4 icons (not 5):

| Icon | Meaning |
|---|---|
| ● | Current phase |
| ✅ | Accepted |
| ⚠️ | Has issues (warnings or blockers) |
| ○ | Pending |

**Dimension calculation (CRITICAL — per `docs/solutions/ui-bugs/tui-dimension-mismatch-splitlayout-20260126.md`):**

```go
case tea.WindowSizeMsg:
    sidebarWidth := 12
    separatorWidth := 1
    availableWidth := msg.Width - sidebarWidth - (2 * separatorWidth)
    docWidth := availableWidth * 55 / 100
    chatWidth := availableWidth - docWidth
    contentHeight := msg.Height - 2  // reserve for breadcrumb + status
```

Always subtract parent chrome from `WindowSizeMsg` before passing to children. Reserve vertical space for breadcrumb/status before allocating to content.

**View decomposition:** Extract `DocPanel` and `PhaseSidebar` into separate files (`doc_panel.go`, `phase_sidebar.go`) within `internal/tui/views/` to keep `sprint_view.go` under 500 lines.

**Files to modify:**

- [x] `internal/tui/views/sprint_view.go` — New file (the main view)
- [x] `internal/tui/views/doc_panel.go` — New file (draft + details toggle)
- [x] `internal/tui/views/phase_sidebar.go` — New file (phase list with icons)
- [x] `internal/tui/unified_app.go` — Route to `SprintView` after project creation (replace separate kickoff→arbiter handoff)
- [x] `internal/tui/messages.go` — Add `SprintDraftUpdatedMsg`, `SprintPhaseAdvancedMsg`, `SprintConflictMsg`

---

### Phase 4: Async Codebase Scan Integration

**Goal:** Vision/Problem/Users phases trigger codebase scanning within the sprint flow, without blocking the TUI.

**Files to modify:**

- [ ] `internal/gurgeh/arbiter/orchestrator.go` — For phases Vision/Problem/Users, auto-trigger codebase scan as async `tea.Cmd`; results arrive as `scanCompleteMsg`
- [ ] `internal/tui/views/sprint_view.go` — Show scan progress in chat ("Scanning codebase for vision signals..."); on `scanCompleteMsg`, pass results to Generator
- [ ] `internal/gurgeh/arbiter/generator.go` — Accept scan results as additional context for draft generation

**Scan phases:**

| Sprint Phase | Scan | What to look for |
|---|---|---|
| Vision | README, CLAUDE.md, package manifests | Project purpose, goals, non-goals |
| Problem | Issues, TODOs, error patterns | Pain points, gaps |
| Users | Config files, CLI flags, API routes | Who interacts with this |
| Phases 4-8 | — | Agent drafts from prior phases |

**Async pattern:**
```go
func (v *SprintView) startScan(phase arbiter.Phase) tea.Cmd {
    return func() tea.Msg {
        results, err := v.scanner.Scan(phase)
        if err != nil {
            return scanErrorMsg{phase, err}
        }
        return scanCompleteMsg{phase, results}
    }
}
```

**Path traversal prevention:** Validate all scan file paths are within project root before storing.

**Security:** Sprint state files written with `0600` permissions (not `0644`).

---

## Acceptance Criteria

### Functional

- [ ] User can go from project creation through all 8 spec phases without leaving the chat panel
- [ ] Codebase scan runs automatically (async) for Vision/Problem/Users phases
- [ ] Evidence, quality scores, and resolved questions survive from scan into all 8 phases
- [ ] Consistency checks run on each phase acceptance; blockers prevent advancement
- [ ] Confidence score updates after each phase with 5-dimension breakdown
- [ ] Thinking shape preambles injected per phase (existing `pkg/thinking` integration)
- [ ] User can navigate to completed phases via sidebar and review (not edit unless `e`)
- [ ] Agent failure triggers retry (3x) with status in chat

### Non-Functional

- [ ] No data loss: all scan artifacts preserved through full sprint
- [ ] Sprint state persists to disk on each phase acceptance with `0600` permissions
- [ ] Existing `go test ./internal/gurgeh/arbiter/...` passes unchanged
- [ ] Scan operations never block the TUI event loop
- [ ] Evidence quotes wrapped in XML delimiters before prompt injection
- [ ] `sprint_view.go` stays under 500 lines (decompose into doc_panel, phase_sidebar)
- [ ] No goroutine leaks: streaming cancelled on navigation via `context.WithCancel`

### Tests to Add

- [ ] Dimension-calculation test: SprintView children receive correct widths after subtracting sidebar + separators
- [ ] Integration test: full 8-phase sprint with mock agent, verifying artifacts survive all transitions
- [ ] Adapter conversion test: `tui.PhaseArtifacts` → `scan.Artifacts` round-trip

---

## Implementation Sequence

| Step | What | Files | Depends on |
|---|---|---|---|
| 1 | Adapter package + artifact types | `arbiter/scan/artifacts.go`, tests | — |
| 2 | Wire artifacts into SprintState + Orchestrator.Start() | `arbiter/types.go`, `arbiter/orchestrator.go` | 1 |
| 3 | Generator uses evidence context | `arbiter/generator.go` | 2 |
| 4 | Confidence uses quality scores | `arbiter/confidence/calculator.go` | 2 |
| 5 | Orchestrator.ProcessChatMessage() + mutex + retry | `arbiter/orchestrator.go`, tests | 2 |
| 6 | SprintView + DocPanel + PhaseSidebar | `tui/views/sprint_view.go`, `doc_panel.go`, `phase_sidebar.go` | 5 |
| 7 | Unified app routing | `tui/unified_app.go`, `tui/messages.go` | 6 |
| 8 | Async scan integration | `arbiter/orchestrator.go`, `tui/views/sprint_view.go` | 3, 5 |

Steps 1-4 are engine-only (no TUI). Steps 5-7 are the new surface. Step 8 is scan completion.

Steps 3 and 4 can run in parallel (both depend only on Step 2).

**Ship as 3 PRs:**
1. **PR 1 (Steps 1-4):** Thread artifacts through engine. Tests pass. No TUI changes.
2. **PR 2 (Steps 5-7):** SprintView wired up. Replace old flow directly — no feature flag (this is a dev tool, keep old code in git history).
3. **PR 3 (Step 8):** Async scan integration within the sprint flow.

---

## Key Decisions

- **Chat is the only input surface.** `a` accept / `e` edit are accelerators, not the only way.
- **Extend Orchestrator, don't wrap it.** View owns Orchestrator directly. No controller layer.
- **Reuse existing types.** `PhaseArtifacts` via adapter, not new `ScanContext`.
- **Async everything.** Scans run as Bubble Tea Cmds. Nothing blocks `Update()`.
- **Scan phases are opt-in.** Greenfield projects skip scanning, rely on chat input.
- **No cascading regeneration.** Phase revision shows warnings; user decides whether to regenerate downstream.
- **Replace, don't feature-flag.** Ship the new flow, delete the old one. Git has history.
- **Decompose the view.** Mandatory extraction of DocPanel and PhaseSidebar to keep files manageable.

## Deferred (ship separately when needed)

- Agent write API (REST endpoints for programmatic sprint interaction)
- Pollard research wiring (stub hook point in Generator for future injection)
- Manual input fallback / open question skip / blocker resolution suggestions
- Terminal escape filtering in ChatPanel
- Additional doc panel sections beyond draft + evidence toggle

## Known Technical Debt

- Phases 4-8 have no structured artifact type (flat `SectionDraft.Content` string). May need structure if consistency engine requires it later.
- Research findings eviction policy unspecified — define when Pollard wiring is added.

## References

- `internal/tui/views/kickoff.go` — Current chat-based interview (1530 lines)
- `internal/gurgeh/arbiter/orchestrator.go` — Sprint engine (608 lines)
- `internal/gurgeh/arbiter/types.go` — SprintState, Phase, SectionDraft
- `internal/gurgeh/arbiter/generator.go` — Draft generation with thinking shapes
- `internal/gurgeh/tui/sprint.go` — Current arbiter TUI (to be replaced)
- `pkg/tui/chatpanel.go` — Shared chat panel component
- `pkg/tui/shell_layout.go` — 3-pane Cursor-style layout
- `docs/solutions/patterns/arbiter-spec-sprint-architecture.md` — Import cycle solution
- `docs/solutions/ui-bugs/tui-dimension-mismatch-splitlayout-20260126.md` — Dimension calculation
- `docs/solutions/ui-bugs/tui-breadcrumb-hidden-by-oversized-child-view-20260127.md` — Chrome reservation
